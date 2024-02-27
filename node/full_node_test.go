package node

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	cmconfig "github.com/cometbft/cometbft/config"
	"github.com/cometbft/cometbft/proxy"
	goDA "github.com/rollkit/go-da"
	"github.com/rollkit/rollkit/config"
	test "github.com/rollkit/rollkit/test/log"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"

	testutils "github.com/celestiaorg/utils/test"

	"github.com/rollkit/rollkit/block"
	"github.com/rollkit/rollkit/da"
	"github.com/rollkit/rollkit/mempool"
	"github.com/rollkit/rollkit/test/mocks"
	"github.com/rollkit/rollkit/types"
)

// simply check that node is starting and stopping without panicking
func TestStartup(t *testing.T) {
	ctx := context.Background()
	node := initAndStartNodeWithCleanup(ctx, t, "full")
	require.IsType(t, new(FullNode), node)
}

func TestMempoolDirectly(t *testing.T) {
	ctx := context.Background()

	fn := initAndStartNodeWithCleanup(ctx, t, "full")
	require.IsType(t, new(FullNode), fn)

	node := fn.(*FullNode)
	assert := assert.New(t)
	peerID := getPeerID(assert)
	verifyTransactions(node, peerID, assert)
	verifyMempoolSize(node, assert)
}

// Tests that the node is able to sync multiple blocks even if blocks arrive out of order
func TestTrySyncNextBlockMultiple(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	node, signingKey := setupTestNode(ctx, t, "full")
	fullNode, ok := node.(*FullNode)
	require.True(t, ok)
	store := fullNode.Store
	height := store.Height()
	b1, _ := types.GetRandomBlockWithKey(height+1, 0, signingKey)
	b2 := types.GetRandomNextBlock(b1, signingKey, []byte{1, 2, 3, 4}, 0)
	b2.SignedHeader.AppHash = []byte{1, 2, 3, 4}

	// Update state with hashes generated from block
	state, err := store.GetState(ctx)
	require.NoError(t, err)
	state.AppHash = b1.SignedHeader.AppHash
	state.LastResultsHash = b1.SignedHeader.LastResultsHash
	manager := fullNode.blockManager
	manager.SetLastState(state)

	// Add second block to blockInCh
	// This should not trigger a sync since b1 hasn't been seen yet
	blockInCh := manager.GetBlockInCh()
	blockInCh <- block.NewBlockEvent{
		Block:    b2,
		DAHeight: state.DAHeight,
	}

	startNodeWithCleanup(t, node)
	require.NoError(t, waitUntilBlockHashSeen(node, b2.Hash().String()))

	newHeight := store.Height()
	require.Equal(t, height, newHeight)

	// Adding first block to blockInCh should sync both blocks
	blockInCh <- block.NewBlockEvent{
		Block:    b1,
		DAHeight: state.DAHeight,
	}

	require.NoError(t, waitForAtLeastNBlocks(node, 2, Store))
}

// Tests that the node ignores invalid blocks posted to the DA layer
func TestInvalidBlocksIgnored(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	node, signingKey := setupTestNode(ctx, t, "full")
	fullNode, ok := node.(*FullNode)
	require.True(t, ok)
	store := fullNode.Store

	manager := fullNode.blockManager
	height := store.Height()
	b1, _ := types.GetRandomBlockWithKey(height+1, 0, signingKey)

	// Update state with hashes generated from block
	state, err := store.GetState(ctx)
	require.NoError(t, err)
	state.AppHash = b1.SignedHeader.AppHash
	state.LastResultsHash = b1.SignedHeader.LastResultsHash
	manager.SetLastState(state)

	// Set up mock DA
	dalc := getMockDA(t)
	fullNode.dalc = dalc
	manager.SetDALC(dalc)

	require.NoError(t, b1.ValidateBasic())

	// Create a block with an invalid proposer address
	junkProposerBlock := *b1
	junkProposerBlock.SignedHeader.ProposerAddress = types.GetRandomBytes(32)

	// Recompute signature over the block with the invalid proposer address
	commit, err := types.GetCommit(junkProposerBlock.SignedHeader.Header, signingKey)
	require.NoError(t, err)
	junkProposerBlock.SignedHeader.Commit = *commit
	require.ErrorIs(t, junkProposerBlock.ValidateBasic(), types.ErrProposerAddressMismatch)

	// Create a block with an invalid commit
	junkCommitBlock := *b1
	junkCommitBlock.SignedHeader.Commit.Signatures = []types.Signature{types.GetRandomBytes(32)}
	require.ErrorIs(t, junkCommitBlock.ValidateBasic(), types.ErrSignatureVerificationFailed)

	// Validate b1 to make sure it's still valid
	require.NoError(t, b1.ValidateBasic())

	startNodeWithCleanup(t, node)

	maxBlobSize, err := fullNode.dalc.DA.MaxBlobSize(ctx)
	require.NoError(t, err)

	// Submit invalid blocks to the mock DA
	// Invalid blocks should be ignored by the node
	submitResp := fullNode.dalc.SubmitBlocks(ctx, []*types.Block{&junkProposerBlock, &junkCommitBlock, b1}, maxBlobSize, -1)
	require.Equal(t, submitResp.Code, da.StatusSuccess)

	// Only the valid block gets synced
	require.NoError(t, waitUntilBlockHashSeen(node, b1.Hash().String()))
	require.True(t, b1.Hash().String() == junkCommitBlock.Hash().String())
	require.False(t, manager.IsBlockHashSeen(junkProposerBlock.Hash().String()))
}

// TestPendingBlocks is a test for bug described in https://github.com/rollkit/rollkit/issues/1548
func TestPendingBlocks(t *testing.T) {
	ctx := context.Background()

	mockDA := new(mocks.DA)
	mockDA.On("MaxBlobSize", mock.Anything).Return(uint64(10240), nil)
	mockDA.On("Submit", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("DA not available"))

	dac := &da.DAClient{
		DA:        mockDA,
		Namespace: goDA.Namespace(MockNamespace),
		GasPrice:  1234,
	}
	dbPath, err := os.MkdirTemp("", "testdb")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(dbPath)
	}()

	node, _ := createAggregatorWithPersistence(ctx, dbPath, dac, t)
	err = node.Start()
	assert.NoError(t, err)

	const firstRunBlocks = 10

	err = waitForAtLeastNBlocks(node, firstRunBlocks, Store)
	assert.NoError(t, err)

	err = node.Stop()
	assert.NoError(t, err)

	// create & start new node
	node, _ = createAggregatorWithPersistence(ctx, dbPath, dac, t)

	// reset DA mock to ensure that Submit was called
	mockDA.On("Submit", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Unset()

	// mock submit function to just return some hash and count the number of submitted blobs
	// node will be stopped after producing at least firstRunBlocks blocks
	// restarted node should get blocks from first and second run (more than firstRunBlocks
	// TODO(tzdybal): this seems fragile and probably should be improved (to deserialize blocks and check heights)
	uniqueBlobs := make(map[string]uint64)
	mockDA.On("Submit", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(
		func(ctx context.Context, blobs [][]byte, gasPrice float64, namespace []byte) ([][]byte, error) {
			hashes := make([][]byte, len(blobs))
			for i, blob := range blobs {
				sha := sha256.Sum256(blob)
				hashes[i] = sha[:]
				uniqueBlobs[string(sha[:])]++
			}
			return hashes, nil
		})

	err = node.Start()
	assert.NoError(t, err)

	// let node produce few more blocks
	err = waitForAtLeastNBlocks(node, firstRunBlocks+5, Store)
	assert.NoError(t, err)

	err = node.Stop()
	assert.NoError(t, err)

	assert.Greater(t, len(uniqueBlobs), firstRunBlocks)
	mock.AssertExpectationsForObjects(t, mockDA)
}

func createAggregatorWithPersistence(ctx context.Context, dbPath string, dalc *da.DAClient, t *testing.T) (Node, *mocks.Application) {
	t.Helper()

	key, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	genesis, genesisValidatorKey := types.GetGenesisWithPrivkey()
	signingKey, err := types.PrivKeyToSigningKey(genesisValidatorKey)
	require.NoError(t, err)

	app := getMockApplication()

	node, err := NewNode(
		ctx,
		config.NodeConfig{
			DBPath:      dbPath,
			DAAddress:   MockServerAddr,
			DANamespace: MockNamespace,
			Aggregator:  true,
			BlockManagerConfig: config.BlockManagerConfig{
				BlockTime:   100 * time.Millisecond,
				DABlockTime: 300 * time.Millisecond,
			},
			Light: false,
		},
		key,
		signingKey,
		proxy.NewLocalClientCreator(app),
		genesis,
		DefaultMetricsProvider(cmconfig.DefaultInstrumentationConfig()),
		test.NewFileLoggerCustom(t, test.TempLogFileName(t, "")),
	)
	require.NoError(t, err)
	require.NotNil(t, node)

	fullNode := node.(*FullNode)
	fullNode.dalc = dalc
	fullNode.blockManager.SetDALC(dalc)

	return fullNode, app
}

// setupMockApplication initializes a mock application
func setupMockApplication() *mocks.Application {
	app := &mocks.Application{}
	app.On("InitChain", mock.Anything, mock.Anything).Return(&abci.ResponseInitChain{}, nil)
	app.On("CheckTx", mock.Anything, mock.Anything).Return(&abci.ResponseCheckTx{}, nil)
	app.On("PrepareProposal", mock.Anything, mock.Anything).Return(prepareProposalResponse).Maybe()
	app.On("ProcessProposal", mock.Anything, mock.Anything).Return(&abci.ResponseProcessProposal{Status: abci.ResponseProcessProposal_ACCEPT}, nil)
	app.On("FinalizeBlock", mock.Anything, mock.Anything).Return(&abci.ResponseFinalizeBlock{AppHash: []byte{1, 2, 3, 4}}, nil)
	app.On("Commit", mock.Anything, mock.Anything).Return(&abci.ResponseCommit{}, nil)
	return app
}

// generateSingleKey generates a single private key
func generateSingleKey() crypto.PrivKey {
	key, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		panic(err)
	}
	return key
}

// getPeerID generates a peer ID
func getPeerID(assert *assert.Assertions) peer.ID {
	key := generateSingleKey()

	peerID, err := peer.IDFromPrivateKey(key)
	assert.NoError(err)
	return peerID
}

// verifyTransactions checks if transactions are valid
func verifyTransactions(node *FullNode, peerID peer.ID, assert *assert.Assertions) {
	transactions := []string{"tx1", "tx2", "tx3", "tx4"}
	for _, tx := range transactions {
		err := node.Mempool.CheckTx([]byte(tx), func(r *abci.ResponseCheckTx) {}, mempool.TxInfo{
			SenderID: node.mempoolIDs.GetForPeer(peerID),
		})
		assert.NoError(err)
	}
}

// verifyMempoolSize checks if the mempool size is as expected
func verifyMempoolSize(node *FullNode, assert *assert.Assertions) {
	assert.NoError(testutils.Retry(300, 100*time.Millisecond, func() error {
		expectedSize := uint64(4 * len("tx*"))
		actualSize := uint64(node.Mempool.SizeBytes())
		if expectedSize == actualSize {
			return nil
		}
		return fmt.Errorf("expected size %v, got size %v", expectedSize, actualSize)
	}))
}
