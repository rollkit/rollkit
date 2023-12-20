package node

import (
	"context"
	"crypto/rand"
	"fmt"
	"testing"
	"time"

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
	node := initializeAndStartFullNode(ctx, t)
	defer cleanUpNode(node, t)
}

func TestMempoolDirectly(t *testing.T) {
	ctx := context.Background()

	node := initializeAndStartFullNode(ctx, t)
	defer cleanUpNode(node, t)
	assert := assert.New(t)

	peerID := getPeerID(assert)
	verifyTransactions(node, peerID, assert)
	verifyMempoolSize(node, assert)
}

// Tests that the node is able to sync multiple blocks even if blocks arrive out of order
func TestTrySyncNextBlockMultiple(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	node, _ := setupTestNode(ctx, t, "full")
	fullNode, ok := node.(*FullNode)
	require.True(t, ok)
	store := fullNode.Store
	height := store.Height()
	b1, signingKey := types.GetRandomBlockWithKey(height+1, 0, nil)
	b2 := types.GetRandomNextBlock(b1, signingKey, []byte{1, 2, 3, 4}, 0)
	b2.SignedHeader.AppHash = []byte{1, 2, 3, 4}

	// Update state with hashes generated from block
	state, err := store.GetState()
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

	err = node.Start()
	require.NoError(t, err)
	defer cleanUpNode(node, t)

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
	state, err := store.GetState()
	require.NoError(t, err)
	state.AppHash = b1.SignedHeader.AppHash
	state.LastResultsHash = b1.SignedHeader.LastResultsHash
	manager.SetLastState(state)

	// Set up mock DA
	dalc := getMockDA()
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

	err = node.Start()
	require.NoError(t, err)
	defer cleanUpNode(node, t)

	// Submit invalid blocks to the mock DA
	// Invalid blocks should be ignored by the node
	submitResp := fullNode.dalc.SubmitBlocks(ctx, []*types.Block{&junkProposerBlock, &junkCommitBlock, b1})
	require.Equal(t, submitResp.Code, da.StatusSuccess)

	// Only the valid block gets synced
	require.NoError(t, waitUntilBlockHashSeen(node, b1.Hash().String()))
	require.True(t, b1.Hash().String() == junkCommitBlock.Hash().String())
	require.False(t, manager.IsBlockHashSeen(junkProposerBlock.Hash().String()))
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
	key, _, _ := crypto.GenerateEd25519Key(rand.Reader)
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
		expectedSize := int64(4 * len("tx*"))
		actualSize := node.Mempool.SizeBytes()
		if expectedSize == actualSize {
			return nil
		}
		return fmt.Errorf("expected size %v, got size %v", expectedSize, actualSize)
	}))
}
