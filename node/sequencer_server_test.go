package node

import (
	"context"
	"crypto/rand"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/rollkit/rollkit/config"
	"github.com/rollkit/rollkit/mocks"
	"github.com/rollkit/rollkit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	abcicli "github.com/tendermint/tendermint/abci/client"
	abci "github.com/tendermint/tendermint/abci/types"
	tmconf "github.com/tendermint/tendermint/config"
	"github.com/tendermint/tendermint/libs/log"
	tmtypes "github.com/tendermint/tendermint/types"
)

func TestSequencerServer(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	app := &mocks.Application{}
	app.On("InitChain", mock.Anything).Return(abci.ResponseInitChain{})
	app.On("CheckTx", mock.Anything).Return(abci.ResponseCheckTx{})
	app.On("BeginBlock", mock.Anything).Return(abci.ResponseBeginBlock{})
	app.On("DeliverTx", mock.Anything).Return(abci.ResponseDeliverTx{})
	app.On("EndBlock", mock.Anything).Return(abci.ResponseEndBlock{})
	app.On("Commit", mock.Anything).Return(abci.ResponseCommit{})
	app.On("GetAppHash", mock.Anything).Return(abci.ResponseGetAppHash{})

	key, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	signingKey, _, _ := crypto.GenerateEd25519Key(rand.Reader)

	blockManagerConfig := config.BlockManagerConfig{
		BlockTime:   1 * time.Second,
		NamespaceID: types.NamespaceID{1, 2, 3, 4, 5, 6, 7, 8},
	}
	node, err := newFullNode(context.Background(), config.NodeConfig{
		DALayer:                "mock",
		Aggregator:             true,
		BlockManagerConfig:     blockManagerConfig,
		SequencerListenAddress: "tcp://127.0.0.1:2007",
		ProgressiveSequencer:   true,
	}, key, signingKey, abcicli.NewLocalClient(nil, app), &tmtypes.GenesisDoc{ChainID: "test"}, log.TestingLogger())
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

	require.NoError(err)

	conf := tmconf.DefaultRPCConfig()
	ss := NewSequencerServer(node, conf, node.Logger)
	err = ss.Start()
	assert.NoError(err)

	resp, err := http.PostForm("http://127.0.0.1:2007/tx",
		url.Values{
			"tx": []string{"abcdefg"},
		},
	)
	assert.NoError(err)
	respData := make([]byte, 1024)
	resp.Body.Read(respData)
	node.Logger.Info("Got:")
	node.Logger.Info(string(respData))

	/*err = node.Start()
	assert.NoError(err)
	defer func() {
		err := node.Stop()
		assert.NoError(err)
	}()
	assert.True(node.IsRunning())*/
}
