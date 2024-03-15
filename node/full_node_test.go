package node

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"strconv"
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
	node := initAndStartNodeWithCleanup(ctx, t, Full)
	require.IsType(t, new(FullNode), node)
}

func TestMempoolDirectly(t *testing.T) {
	ctx := context.Background()

	fn := initAndStartNodeWithCleanup(ctx, t, Full)
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
	node, signingKey := setupTestNode(ctx, t, Full)
	fullNode, ok := node.(*FullNode)
	require.True(t, ok)

	store := fullNode.Store
	height := store.Height()

	config := types.BlockConfig{
		Height:  height + 1,
		NTxs:    0,
		PrivKey: signingKey,
	}
	b1, _ := types.GenerateRandomBlockCustom(&config)
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
	node, signingKey := setupTestNode(ctx, t, Full)
	fullNode, ok := node.(*FullNode)
	require.True(t, ok)
	store := fullNode.Store

	manager := fullNode.blockManager
	height := store.Height()

	config := types.BlockConfig{
		Height:  height + 1,
		NTxs:    0,
		PrivKey: signingKey,
	}

	b1, _ := types.GenerateRandomBlockCustom(&config)

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
//
// Test scenario:
// - mock DA to refuse all submissions (returning error)
// - run aggregator to produce some blocks
// - stop aggregator node
// - all blocks should be considered as pending DA submission (because of mock DA behaviour)
// - change mock to accept all submissions
// - start aggregator node again (using the same store, to simulate restart)
// - verify that blocks from first run was submitted to DA
// - additionally - ensure that information was persisted in store (TODO: this should be tested separately)
func TestPendingBlocks(t *testing.T) {
	ctx := context.Background()

	mockDA := new(mocks.DA)
	mockDA.On("MaxBlobSize", mock.Anything).Return(uint64(10240), nil)
	mockDA.On("Submit", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.New("DA not available"))

	dac := da.NewDAClient(mockDA, 1234, -1, goDA.Namespace(MockDANamespace), nil)
	dbPath, err := os.MkdirTemp("", "testdb")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(dbPath)
	}()

	node, _ := createAggregatorWithPersistence(ctx, dbPath, dac, t)
	err = node.Start()
	assert.NoError(t, err)

	const (
		firstRunBlocks  = 10
		secondRunBlocks = 5
	)

	err = waitForAtLeastNBlocks(node, firstRunBlocks, Store)
	assert.NoError(t, err)

	err = node.Stop()
	assert.NoError(t, err)

	// create & start new node
	node, _ = createAggregatorWithPersistence(ctx, dbPath, dac, t)

	// reset DA mock to ensure that Submit was called
	mockDA.On("Submit", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Unset()

	// mock submit function to just return some hash and collect all blobs in order
	// node will be stopped after producing at least firstRunBlocks blocks
	// restarted node should submit to DA blocks from first and second run (more than firstRunBlocks)
	allBlobs := make([][]byte, 0, firstRunBlocks+secondRunBlocks)
	mockDA.On("Submit", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(
		func(ctx context.Context, blobs [][]byte, gasPrice float64, namespace []byte) ([][]byte, error) {
			hashes := make([][]byte, len(blobs))
			for i, blob := range blobs {
				sha := sha256.Sum256(blob)
				hashes[i] = sha[:]
			}
			allBlobs = append(allBlobs, blobs...)
			return hashes, nil
		})

	err = node.Start()
	assert.NoError(t, err)

	// let node produce few more blocks
	err = waitForAtLeastNBlocks(node, firstRunBlocks+secondRunBlocks, Store)
	assert.NoError(t, err)

	// assert that LastSubmittedHeight was updated in store
	raw, err := node.(*FullNode).Store.GetMetadata(context.Background(), block.LastSubmittedHeightKey)
	require.NoError(t, err)
	lsh, err := strconv.ParseUint(string(raw), 10, 64)
	require.NoError(t, err)
	assert.Greater(t, lsh, uint64(firstRunBlocks))

	err = node.Stop()
	assert.NoError(t, err)
	mock.AssertExpectationsForObjects(t, mockDA)

	// ensure that all blocks were submitted in order
	for i := 0; i < len(allBlobs); i++ {
		b := &types.Block{}
		err := b.UnmarshalBinary(allBlobs[i])
		require.NoError(t, err)
		require.Equal(t, uint64(i+1), b.Height()) // '+1' because blocks start at genesis with height 1
	}

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
			DAAddress:   MockDAAddress,
			DANamespace: MockDANamespace,
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
