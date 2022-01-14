package json

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/gorilla/rpc/v2/json2"
	"github.com/libp2p/go-libp2p-core/crypto"
	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/proxy"
	"github.com/tendermint/tendermint/types"

	"github.com/celestiaorg/optimint/config"
	"github.com/celestiaorg/optimint/mocks"
	"github.com/celestiaorg/optimint/node"
	"github.com/celestiaorg/optimint/rpc/client"
)

func TestHandlerMapping(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	_, local := getRPC(t)
	handler, err := GetHttpHandler(local, log.TestingLogger())
	require.NoError(err)

	jsonReq, err := json2.EncodeClientRequest("health", &HealthArgs{})
	require.NoError(err)

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(jsonReq))
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	assert.Equal(http.StatusOK, resp.Code)
}

func TestREST(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	txSearchParams := url.Values{}
	txSearchParams.Set("query", "message.sender='cosmos1njr26e02fjcq3schxstv458a3w5szp678h23dh'")
	txSearchParams.Set("prove", "true")
	txSearchParams.Set("page", "1")
	txSearchParams.Set("per_page", "10")
	txSearchParams.Set("order_by", "asc")

	cases := []struct {
		name         string
		uri          string
		httpCode     int
		jsonrpcCode  int
		bodyContains string
	}{

		{"invalid/malformed request", "/block?so{}wrong!", http.StatusOK, int(json2.E_INVALID_REQ), ``},
		{"invalid/missing param", "/block", http.StatusOK, int(json2.E_INVALID_REQ), `missing param 'height'`},
		{"valid/no params", "/abci_info", http.StatusOK, -1, `"last_block_height":345`},
		// to keep test simple, allow returning application error in following case
		{"valid/int param", "/block?height=321", http.StatusOK, int(json2.E_INTERNAL), `"key not found"`},
		{"invalid/int param", "/block?height=foo", http.StatusOK, int(json2.E_PARSE), "failed to parse param 'height'"},
		{"valid/bool int string params",
			"/tx_search?" + txSearchParams.Encode(),
			http.StatusOK, -1, `"total_count":0`},
		{"invalid/bool int string params",
			"/tx_search?" + strings.Replace(txSearchParams.Encode(), "true", "blue", 1),
			http.StatusOK, int(json2.E_PARSE), "failed to parse param 'prove'"},
		{"valid/hex param", "/check_tx?tx=DEADBEEF", http.StatusOK, -1, `"gas_used":"1000"`},
		{"invalid/hex param", "/check_tx?tx=QWERTY", http.StatusOK, int(json2.E_PARSE), "failed to parse param 'tx'"},
	}

	_, local := getRPC(t)
	handler, err := GetHttpHandler(local, log.TestingLogger())
	require.NoError(err)

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, c.uri, nil)
			resp := httptest.NewRecorder()
			handler.ServeHTTP(resp, req)

			assert.Equal(c.httpCode, resp.Code)
			s := resp.Body.String()
			assert.NotEmpty(s)
			assert.Contains(s, c.bodyContains)
			var jsonResp response
			assert.NoError(json.Unmarshal([]byte(s), &jsonResp))
			if c.jsonrpcCode != -1 {
				require.NotNil(jsonResp.Error)
				assert.EqualValues(c.jsonrpcCode, jsonResp.Error.Code)
			}
			t.Log(s)
		})
	}

}

func TestEmptyRequest(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	_, local := getRPC(t)
	handler, err := GetHttpHandler(local, log.TestingLogger())
	require.NoError(err)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	assert.Equal(http.StatusOK, resp.Code)
}

func TestStringyRequest(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	_, local := getRPC(t)
	handler, err := GetHttpHandler(local, log.TestingLogger())
	require.NoError(err)

	// `starport chain faucet ...` generates broken JSON (ints are "quoted" as strings)
	brokenJSON := `{"jsonrpc":"2.0","id":0,"method":"tx_search","params":{"order_by":"","page":"1","per_page":"1000","prove":true,"query":"message.sender='cosmos1njr26e02fjcq3schxstv458a3w5szp678h23dh' AND transfer.recipient='cosmos1e0ajth0s847kqcu2ssnhut32fsrptf94fqnfzx'"}}`

	respJson := `{"jsonrpc":"2.0","result":{"txs":[],"total_count":0},"id":0}` + "\n"

	req := httptest.NewRequest(http.MethodGet, "/", strings.NewReader(brokenJSON))
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	assert.Equal(http.StatusOK, resp.Code)
	assert.Equal(respJson, resp.Body.String())
}

func TestSubscription(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	const (
		query        = "message.sender='cosmos1njr26e02fjcq3schxstv458a3w5szp678h23dh'"
		query2       = "message.sender!='cosmos1njr26e02fjcq3schxstv458a3w5szp678h23dh'"
		invalidQuery = "message.sender='broken"
	)
	subscribeReq, err := json2.EncodeClientRequest("subscribe", &SubscribeArgs{
		Query: query,
	})
	require.NoError(err)
	require.NotEmpty(subscribeReq)

	subscribeReq2, err := json2.EncodeClientRequest("subscribe", &SubscribeArgs{
		Query: query2,
	})
	require.NoError(err)
	require.NotEmpty(subscribeReq2)

	invalidSubscribeReq, err := json2.EncodeClientRequest("subscribe", &SubscribeArgs{
		Query: invalidQuery,
	})
	require.NoError(err)
	require.NotEmpty(invalidSubscribeReq)

	unsubscribeReq, err := json2.EncodeClientRequest("unsubscribe", &UnsubscribeArgs{
		Query: query,
	})
	require.NoError(err)
	require.NotEmpty(unsubscribeReq)

	unsubscribeAllReq, err := json2.EncodeClientRequest("unsubscribe_all", &UnsubscribeAllArgs{})
	require.NoError(err)
	require.NotEmpty(unsubscribeAllReq)

	_, local := getRPC(t)
	handler, err := GetHttpHandler(local, log.TestingLogger())
	require.NoError(err)

	var (
		jsonResp response
	)

	// test valid subscription
	req := httptest.NewRequest(http.MethodGet, "/", bytes.NewReader(subscribeReq))
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	assert.Equal(http.StatusOK, resp.Code)
	jsonResp = response{}
	assert.NoError(json.Unmarshal(resp.Body.Bytes(), &jsonResp))
	assert.Nil(jsonResp.Error)

	// test valid subscription with second query
	req = httptest.NewRequest(http.MethodGet, "/", bytes.NewReader(subscribeReq2))
	resp = httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	assert.Equal(http.StatusOK, resp.Code)
	jsonResp = response{}
	assert.NoError(json.Unmarshal(resp.Body.Bytes(), &jsonResp))
	assert.Nil(jsonResp.Error)

	// test subscription with invalid query
	req = httptest.NewRequest(http.MethodGet, "/", bytes.NewReader(invalidSubscribeReq))
	resp = httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	assert.Equal(http.StatusOK, resp.Code)
	jsonResp = response{}
	assert.NoError(json.Unmarshal(resp.Body.Bytes(), &jsonResp))
	require.NotNil(jsonResp.Error)
	assert.Contains(jsonResp.Error.Message, "failed to parse query")

	// test valid, but duplicate subscription
	req = httptest.NewRequest(http.MethodGet, "/", bytes.NewReader(subscribeReq))
	resp = httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	assert.Equal(http.StatusOK, resp.Code)
	jsonResp = response{}
	assert.NoError(json.Unmarshal(resp.Body.Bytes(), &jsonResp))
	require.NotNil(jsonResp.Error)
	assert.Contains(jsonResp.Error.Message, "already subscribed")

	// test unsubscribing
	req = httptest.NewRequest(http.MethodGet, "/", bytes.NewReader(unsubscribeReq))
	resp = httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	assert.Equal(http.StatusOK, resp.Code)
	jsonResp = response{}
	assert.NoError(json.Unmarshal(resp.Body.Bytes(), &jsonResp))
	assert.Nil(jsonResp.Error)

	// test unsubscribing again
	req = httptest.NewRequest(http.MethodGet, "/", bytes.NewReader(unsubscribeReq))
	resp = httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	assert.Equal(http.StatusOK, resp.Code)
	jsonResp = response{}
	assert.NoError(json.Unmarshal(resp.Body.Bytes(), &jsonResp))
	require.NotNil(jsonResp.Error)
	assert.Contains(jsonResp.Error.Message, "subscription not found")

	// test unsubscribe all
	req = httptest.NewRequest(http.MethodGet, "/", bytes.NewReader(unsubscribeAllReq))
	resp = httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	assert.Equal(http.StatusOK, resp.Code)
	jsonResp = response{}
	assert.NoError(json.Unmarshal(resp.Body.Bytes(), &jsonResp))
	assert.Nil(jsonResp.Error)

	// test unsubscribing all again
	req = httptest.NewRequest(http.MethodGet, "/", bytes.NewReader(unsubscribeAllReq))
	resp = httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	assert.Equal(http.StatusOK, resp.Code)
	jsonResp = response{}
	assert.NoError(json.Unmarshal(resp.Body.Bytes(), &jsonResp))
	require.NotNil(jsonResp.Error)
	assert.Contains(jsonResp.Error.Message, "subscription not found")

}

// copied from rpc
func getRPC(t *testing.T) (*mocks.Application, *client.Client) {
	t.Helper()
	require := require.New(t)
	app := &mocks.Application{}
	app.On("InitChain", mock.Anything).Return(abci.ResponseInitChain{})
	app.On("CheckTx", mock.Anything).Return(abci.ResponseCheckTx{
		GasWanted: 1000,
		GasUsed:   1000,
	})
	app.On("Info", mock.Anything).Return(abci.ResponseInfo{
		Data:             "mock",
		Version:          "mock",
		AppVersion:       123,
		LastBlockHeight:  345,
		LastBlockAppHash: nil,
	})
	key, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	node, err := node.NewNode(context.Background(), config.NodeConfig{DALayer: "mock"}, key, proxy.NewLocalClientCreator(app), &types.GenesisDoc{ChainID: "test"}, log.TestingLogger())
	require.NoError(err)
	require.NotNil(node)

	local := client.NewClient(node)
	require.NotNil(local)

	return app, local
}
