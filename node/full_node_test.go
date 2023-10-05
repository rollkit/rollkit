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
	"github.com/cometbft/cometbft/libs/log"
	proxy "github.com/cometbft/cometbft/proxy"
	"github.com/cometbft/cometbft/types"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"

	testutils "github.com/celestiaorg/utils/test"

	"github.com/rollkit/rollkit/config"
	"github.com/rollkit/rollkit/mempool"
	"github.com/rollkit/rollkit/test/mocks"
)

// TestStartup checks if the node starts and stops without any errors
func TestStartup(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	ctx := context.Background()
	node := initializeAndStartNode(ctx, require, assert)
	defer cleanUpNode(node, assert)
	assert.True(node.IsRunning())
}

func TestMempoolDirectly(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	ctx := context.Background()
	node := initializeAndStartNode(ctx, require, assert)
	defer cleanUpNode(node, assert)

	peerID := getPeerID(assert)
	verifyTransactions(node, peerID, assert)
	verifyMempoolSize(node, assert)
}

// cleanUpNode stops the node and checks if it is running
func cleanUpNode(node *FullNode, assert *assert.Assertions) {
	defer node.cancel()
	assert.NoError(node.Stop())
	assert.False(node.IsRunning())
}

// setupMockApplication initializes a mock application
func setupMockApplication() *mocks.Application {
	app := &mocks.Application{}
	app.On(InitChain, mock.Anything).Return(abci.ResponseInitChain{})
	app.On(CheckTx, mock.Anything).Return(abci.ResponseCheckTx{})
	return app
}

// generateSingleKey generates a single private key
func generateSingleKey() crypto.PrivKey {
	key, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	return key
}

// newTestNode creates a new test node
func newTestNode(ctx context.Context, key crypto.PrivKey, signingKey crypto.PrivKey, app *mocks.Application) (*FullNode, error) {
	return newFullNode(ctx, config.NodeConfig{DALayer: "mock"}, key, signingKey, proxy.NewLocalClientCreator(app), &types.GenesisDoc{ChainID: "test"}, log.TestingLogger())
}

// setupTestNode sets up a test node
func setupTestNode(ctx context.Context, require *require.Assertions) *FullNode {
	app := setupMockApplication()
	key, signingKey := generateSingleKey(), generateSingleKey()
	node, err := newTestNode(ctx, key, signingKey, app)
	require.NoError(err)
	require.NotNil(node)
	return node
}

// initializeAndStartNode initializes and starts a test node
func initializeAndStartNode(ctx context.Context, require *require.Assertions, assert *assert.Assertions) *FullNode {
	node := setupTestNode(ctx, require)
	assert.False(node.IsRunning())
	err := node.Start()
	assert.NoError(err)
	return node
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
		err := node.Mempool.CheckTx([]byte(tx), func(r *abci.Response) {}, mempool.TxInfo{
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
