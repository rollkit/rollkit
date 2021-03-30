package rpcclient

import (
	"context"
	"crypto/rand"
	"testing"

	abci "github.com/lazyledger/lazyledger-core/abci/types"
	"github.com/lazyledger/lazyledger-core/libs/log"
	"github.com/lazyledger/lazyledger-core/proxy"
	"github.com/lazyledger/lazyledger-core/types"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/lazyledger/optimint/config"
	"github.com/lazyledger/optimint/mocks"
	"github.com/lazyledger/optimint/node"
)

var expectedInfo = abci.ResponseInfo{
	Version:         "v0.0.1",
	AppVersion:      1,
	LastBlockHeight: 0,
}

func TestInfo(t *testing.T) {
	assert := assert.New(t)

	mockApp, rpc := getRPC(t)
	mockApp.On("Info", mock.Anything).Return(expectedInfo)

	info, err := rpc.ABCIInfo(context.Background())
	assert.NoError(err)
	assert.Equal(expectedInfo, info.Response)
}

func TestMempool(t *testing.T) {

}

func getRPC(t *testing.T) (*mocks.Application, *Local) {
	t.Helper()
	require := require.New(t)
	app := &mocks.Application{}
	key, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	node, err := node.NewNode(context.Background(), config.NodeConfig{}, key, proxy.NewLocalClientCreator(app), &types.GenesisDoc{}, log.TestingLogger())
	require.NoError(err)
	require.NotNil(node)

	rpc := NewLocal(node)
	require.NotNil(rpc)

	return app, rpc
}
