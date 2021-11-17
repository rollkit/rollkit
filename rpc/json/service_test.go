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
	"strings"
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

func TestStringyRequest(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	_, local := getRPC(t)
	handler, err := GetHttpHandler(local)
	require.NoError(err)

	// `starport chain faucet ...` generates broken JSON (ints are "quoted" as strings)
	brokenJSON := `{"jsonrpc":"2.0","id":0,"method":"tx_search","params":{"order_by":"","page":"1","per_page":"1000","prove":true,"query":"message.sender='cosmos1njr26e02fjcq3schxstv458a3w5szp678h23dh' AND transfer.recipient='cosmos1e0ajth0s847kqcu2ssnhut32fsrptf94fqnfzx'"}}`

	req := httptest.NewRequest(http.MethodGet, "/", strings.NewReader(brokenJSON))
	resp := httptest.NewRecorder()
	assert.PanicsWithValue("TxSearch - not implemented!", func() {handler.ServeHTTP(resp, req)})

	assert.Equal(200, resp.Code)
	assert.Empty(resp.Body.String())

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
