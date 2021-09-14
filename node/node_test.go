package node

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/proxy"
	"github.com/tendermint/tendermint/types"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/celestiaorg/optimint/config"
	"github.com/celestiaorg/optimint/mocks"
	"github.com/celestiaorg/optimint/p2p"
)

// simply check that node is starting and stopping without panicking
func TestStartup(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	app := &mocks.Application{}
	key, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	node, err := NewNode(context.Background(), config.NodeConfig{DALayer: "mock"}, key, proxy.NewLocalClientCreator(app), &types.GenesisDoc{ChainID: "test"}, log.TestingLogger())
	require.NoError(err)
	require.NotNil(node)

	assert.False(node.IsRunning())

	err = node.Start()
	assert.NoError(err)
	defer func() {
		err := node.Stop()
		assert.NoError(err)
	}()
	assert.True(node.IsRunning())
}

func TestMempoolDirectly(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	app := &mocks.Application{}
	app.On("CheckTx", mock.Anything).Return(abci.ResponseCheckTx{})
	key, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	anotherKey, _, _ := crypto.GenerateEd25519Key(rand.Reader)

	node, err := NewNode(context.Background(), config.NodeConfig{DALayer: "mock"}, key, proxy.NewLocalClientCreator(app), &types.GenesisDoc{ChainID: "test"}, log.TestingLogger())
	require.NoError(err)
	require.NotNil(node)

	err = node.Start()
	require.NoError(err)

	pid, err := peer.IDFromPrivateKey(anotherKey)
	require.NoError(err)
	node.incomingTxCh <- &p2p.GossipMessage{Data: []byte("tx1"), From: pid}
	node.incomingTxCh <- &p2p.GossipMessage{Data: []byte("tx2"), From: pid}
	time.Sleep(100 * time.Millisecond)
	node.incomingTxCh <- &p2p.GossipMessage{Data: []byte("tx3"), From: pid}
	node.incomingTxCh <- &p2p.GossipMessage{Data: []byte("tx4"), From: pid}

	time.Sleep(1 * time.Second)

	assert.Equal(int64(4*len("tx*")), node.Mempool.TxsBytes())
}
