package json

import (
	"bytes"
	"context"
	"crypto/rand"
	"github.com/celestiaorg/optimint/config"
	"github.com/celestiaorg/optimint/mocks"
	"github.com/celestiaorg/optimint/node"
	"github.com/celestiaorg/optimint/rpc/client"
	gorillajson "github.com/gorilla/rpc/v2/json"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/proxy"
	"github.com/tendermint/tendermint/types"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandlerMapping(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	_, local := getRPC(t)
	handler, err := GetHttpHandler(local)
	require.NoError(err)

	jsonReq, err := gorillajson.EncodeClientRequest("health", &HealthArgs{})
	require.NoError(err)

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(jsonReq))
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	assert.Equal(200, resp.Code)
}

func TestEmptyRequest(t *testing.T) {

	assert := assert.New(t)
	require := require.New(t)

	_, local := getRPC(t)
	handler, err := GetHttpHandler(local)
	require.NoError(err)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	assert.Equal(200, resp.Code)
}

// copied from rpc
func getRPC(t *testing.T) (*mocks.Application, *client.Client) {
	t.Helper()
	require := require.New(t)
	app := &mocks.Application{}
	app.On("InitChain", mock.Anything).Return(abci.ResponseInitChain{})
	key, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	node, err := node.NewNode(context.Background(), config.NodeConfig{DALayer: "mock"}, key, proxy.NewLocalClientCreator(app), &types.GenesisDoc{ChainID: "test"}, log.TestingLogger())
	require.NoError(err)
	require.NotNil(node)

	local := client.NewClient(node)
	require.NotNil(local)

	return app, local
}
