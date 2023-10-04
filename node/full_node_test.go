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

	app := setupMockApplication()
	key, signingKey := generateKeys()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	node, err := newFullNode(ctx, config.NodeConfig{DALayer: "mock"}, key, signingKey, proxy.NewLocalClientCreator(app), &types.GenesisDoc{ChainID: "test"}, log.TestingLogger())
	require.NoError(err)
	require.NotNil(node)

	assert.False(node.IsRunning())

	err = node.Start()
	assert.NoError(err)
	defer func() {
		assert.NoError(node.Stop())
	}()
	assert.True(node.IsRunning())
}

func TestMempoolDirectly(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	app := setupMockApplication()
	key, signingKey := generateKeys()
	anotherKey := generateSingleKey()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	node, err := setupNode(ctx, key, signingKey, app)
	require.NoError(err)
	require.NotNil(node)

	err = node.Start()
	require.NoError(err)
	defer func() {
		assert.NoError(node.Stop())
	}()

	pid := getPeerID(anotherKey, require)

	checkTransactions(node, pid, require)

	checkMempoolSize(node, require)
}

func setupMockApplication() *mocks.Application {
	app := &mocks.Application{}
	app.On(InitChain, mock.Anything).Return(abci.ResponseInitChain{})
	app.On(CheckTx, mock.Anything).Return(abci.ResponseCheckTx{})
	return app
}

func generateKeys() (crypto.PrivKey, crypto.PrivKey) {
	key, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	signingKey, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	return key, signingKey
}

func generateSingleKey() crypto.PrivKey {
	key, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	return key
}

func setupNode(ctx context.Context, key crypto.PrivKey, signingKey crypto.PrivKey, app *mocks.Application) (*FullNode, error) {
	return newFullNode(ctx, config.NodeConfig{DALayer: "mock"}, key, signingKey, proxy.NewLocalClientCreator(app), &types.GenesisDoc{ChainID: "test"}, log.TestingLogger())
}

func getPeerID(key crypto.PrivKey, require *require.Assertions) peer.ID {
	pid, err := peer.IDFromPrivateKey(key)
	require.NoError(err)
	return pid
}

func checkTransactions(node *FullNode, pid peer.ID, require *require.Assertions) {
	transactions := []string{"tx1", "tx2", "tx3", "tx4"}
	for _, tx := range transactions {
		err := node.Mempool.CheckTx([]byte(tx), func(r *abci.Response) {}, mempool.TxInfo{
			SenderID: node.mempoolIDs.GetForPeer(pid),
		})
		require.NoError(err)
	}
}

func checkMempoolSize(node *FullNode, require *require.Assertions) {
	require.NoError(testutils.Retry(300, 100*time.Millisecond, func() error {
		expectedSize := int64(4 * len("tx*"))
		actualSize := node.Mempool.SizeBytes()
		if expectedSize == actualSize {
			return nil
		}
		return fmt.Errorf("expected size %v, got size %v", expectedSize, actualSize)
	}))
}
