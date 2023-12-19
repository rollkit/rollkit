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
	node := setupTestNode(ctx, t, "full")
	fullNode, ok := node.(*FullNode)
	require.True(t, ok)
	store := fullNode.Store
	height := store.Height()
	b1, signingKey := types.GetRandomBlockWithKey(height+1, 0)
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
