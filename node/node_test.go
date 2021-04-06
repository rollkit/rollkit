package node

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	abci "github.com/lazyledger/lazyledger-core/abci/types"
	"github.com/lazyledger/lazyledger-core/libs/log"
	"github.com/lazyledger/lazyledger-core/proxy"
	"github.com/lazyledger/lazyledger-core/types"
	"github.com/libp2p/go-libp2p-core/peer"
	crypto "github.com/libp2p/go-libp2p-crypto"

	"github.com/lazyledger/optimint/config"
	"github.com/lazyledger/optimint/mocks"
	"github.com/lazyledger/optimint/p2p"
)

// simply check that node is starting and stopping without panicking
func TestStartup(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	app := &mocks.Application{}
	key, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	node, err := NewNode(context.Background(), config.NodeConfig{}, key, proxy.NewLocalClientCreator(app), &types.GenesisDoc{}, log.TestingLogger())
	require.NoError(err)
	require.NotNil(node)

	assert.False(node.IsRunning())

	err = node.Start()
	assert.NoError(err)
	defer node.Stop()
	assert.True(node.IsRunning())
}

func TestMempool(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	app := &mocks.Application{}
	app.On("CheckTx", mock.Anything).Return(abci.ResponseCheckTx{})
	key, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	anotherKey, _, _ := crypto.GenerateEd25519Key(rand.Reader)

	node, err := NewNode(context.Background(), config.NodeConfig{}, key, proxy.NewLocalClientCreator(app), &types.GenesisDoc{}, log.TestingLogger())
	require.NoError(err)
	require.NotNil(node)

	err = node.Start()
	require.NoError(err)

	pid, err := peer.IDFromPrivateKey(anotherKey)
	require.NoError(err)
	node.incommingTxCh <- &p2p.Tx{Data: []byte("foobar"), From: pid}

	time.Sleep(1 * time.Second)

	assert.Equal(node.Mempool.TxsBytes(), int64(len("foobar")))
}
