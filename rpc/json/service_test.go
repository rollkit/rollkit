package json

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/cometbft/cometbft/crypto/ed25519"
	"github.com/cometbft/cometbft/p2p"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	"github.com/stretchr/testify/mock"

	"github.com/rollkit/rollkit/test/mocks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cometbft/cometbft/libs/log"
	"github.com/gorilla/rpc/v2/json2"
)

func TestHandlerMapping(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	_, local := getRPC(t, "TestHandlerMapping")
	handler, err := GetHTTPHandler(local, log.TestingLogger())
	require.NoError(err)

	jsonReq, err := json2.EncodeClientRequest("health", &healthArgs{})
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

		{"valid/malformed request", "/block?so{}wrong!", http.StatusOK, -1, `"result":{"block_id":`},
		// to keep test simple, allow returning application error in following case
		{"invalid/missing required param", "/tx", http.StatusOK, int(json2.E_INVALID_REQ), `missing param 'hash'`},
		{"valid/missing optional param", "/block", http.StatusOK, -1, `"result":{"block_id":`},
		{"valid/included block tag param", "/block?height=included", http.StatusOK, int(json2.E_INTERNAL), "failed to load hash from index"},
		{"valid/no params", "/abci_info", http.StatusOK, -1, `"last_block_height":"345"`},
		{"valid/int param", "/block?height=321", http.StatusOK, int(json2.E_INTERNAL), "failed to load hash from index"},
		{"invalid/int param", "/block?height=foo", http.StatusOK, int(json2.E_PARSE), "failed to parse param 'height'"},
		{"valid/bool int string params",
			"/tx_search?" + txSearchParams.Encode(),
			http.StatusOK, -1, `"total_count":"0"`},
		{"invalid/bool int string params",
			"/tx_search?" + strings.Replace(txSearchParams.Encode(), "true", "blue", 1),
			http.StatusOK, int(json2.E_PARSE), "failed to parse param 'prove'"},
		{"valid/hex param", "/check_tx?tx=DEADBEEF", http.StatusOK, -1, `"gas_used":"1000"`},
		{"invalid/hex param", "/check_tx?tx=QWERTY", http.StatusOK, int(json2.E_PARSE), "failed to parse param 'tx'"},
	}

	_, local := getRPC(t, "TestREST")
	handler, err := GetHTTPHandler(local, log.TestingLogger())
	require.NoError(err)

	// wait for blocks
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	for {
		if errors.Is(err, context.DeadlineExceeded) {
			t.FailNow()
		}
		resp, err := local.Header(ctx, nil)
		if err == nil && resp.Header.Height > 1 {
			break
		}
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, c.uri, nil)
			resp := httptest.NewRecorder()
			handler.ServeHTTP(resp, req)

			assert.Equal(c.httpCode, resp.Code)
			s := resp.Body.String()
			assert.NotEmpty(s)
			fmt.Print(s)
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

	_, local := getRPC(t, "TestEmptyRequest")
	handler, err := GetHTTPHandler(local, log.TestingLogger())
	require.NoError(err)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	assert.Equal(http.StatusOK, resp.Code)
}

func TestStringyRequest(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	_, local := getRPC(t, "TestStringyRequest")
	handler, err := GetHTTPHandler(local, log.TestingLogger())
	require.NoError(err)

	// `starport chain faucet ...` generates broken JSON (ints are "quoted" as strings)
	brokenJSON := `{"jsonrpc":"2.0","id":0,"method":"tx_search","params":{"order_by":"","page":"1","per_page":"1000","prove":true,"query":"message.sender='cosmos1njr26e02fjcq3schxstv458a3w5szp678h23dh' AND transfer.recipient='cosmos1e0ajth0s847kqcu2ssnhut32fsrptf94fqnfzx'"}}`

	respJSON := `{"jsonrpc":"2.0","result":{"txs":[],"total_count":"0"},"id":0}` + "\n"

	req := httptest.NewRequest(http.MethodGet, "/", strings.NewReader(brokenJSON))
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	assert.Equal(http.StatusOK, resp.Code)
	assert.Equal(respJSON, resp.Body.String())
}

func TestSubscription(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	var (
		query        = "message.sender='cosmos1njr26e02fjcq3schxstv458a3w5szp678h23dh'"
		query2       = "message.sender CONTAINS 'cosmos1njr26e02fjcq3schxstv458a3w5szp678h23dh'"
		invalidQuery = "message.sender='broken"
	)
	subscribeReq, err := json2.EncodeClientRequest("subscribe", &subscribeArgs{
		Query: &query,
	})
	require.NoError(err)
	require.NotEmpty(subscribeReq)

	subscribeReq2, err := json2.EncodeClientRequest("subscribe", &subscribeArgs{
		Query: &query2,
	})
	require.NoError(err)
	require.NotEmpty(subscribeReq2)

	invalidSubscribeReq, err := json2.EncodeClientRequest("subscribe", &subscribeArgs{
		Query: &invalidQuery,
	})
	require.NoError(err)
	require.NotEmpty(invalidSubscribeReq)

	unsubscribeReq, err := json2.EncodeClientRequest("unsubscribe", &unsubscribeArgs{
		Query: &query,
	})
	require.NoError(err)
	require.NotEmpty(unsubscribeReq)

	unsubscribeAllReq, err := json2.EncodeClientRequest("unsubscribe_all", &unsubscribeAllArgs{})
	require.NoError(err)
	require.NotEmpty(unsubscribeAllReq)

	_, local := getRPC(t, "TestSubscription")
	handler, err := GetHTTPHandler(local, log.TestingLogger())
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

func TestRESTSerialization(t *testing.T) {
	tests := []struct {
		name         string
		endpoint     string
		mockCode     int
		mockResp     interface{}
		mockError    error
		expectedResp string
	}{
		{
			name:      "Health",
			endpoint:  "/health",
			mockCode:  200,
			mockResp:  &coretypes.ResultHealth{},
			mockError: nil,
			expectedResp: `{
  "jsonrpc": "2.0",
  "id": -1,
  "result": {}
}`,
		},
		{
			name:     "Status",
			endpoint: "/status",
			mockCode: 200,
			mockResp: &coretypes.ResultStatus{
				NodeInfo: p2p.DefaultNodeInfo{
					ProtocolVersion: p2p.ProtocolVersion{
						P2P:   8,
						Block: 11,
						App:   0,
					},
					DefaultNodeID: "b93270b358a72a2db30089f3856475bb1f918d6d",
					ListenAddr:    "tcp://0.0.0.0:26656",
					Network:       "cosmoshub-4",
					Version:       "v0.34.8",
					Channels:      mustHexToBytes("40202122233038606100"),
					Moniker:       "aib-hub-node",
					Other: p2p.DefaultNodeInfoOther{
						TxIndex:    "on",
						RPCAddress: "tcp://0.0.0.0:26657",
					},
				},
				SyncInfo: coretypes.SyncInfo{
					LatestBlockHash:     mustHexToBytes("50F03C0EAACA8BCA7F9C14189ACE9C05A9A1BBB5268DB63DC6A3C848D1ECFD27"),
					LatestAppHash:       mustHexToBytes("2316CFF7644219F4F15BEE456435F280E2B38955EEA6D4617CCB6D7ABF781C22"),
					LatestBlockHeight:   5622165,
					LatestBlockTime:     mustTimeParse("2021-03-25T14:00:43.356134226Z"),
					EarliestBlockHash:   mustHexToBytes("1455A0C15AC49BB506992EC85A3CD4D32367E53A087689815E01A524231C3ADF"),
					EarliestAppHash:     mustHexToBytes("E3B0C44298FC1C149AFBF4C8996FB92427AE41E4649B934CA495991B7852B855"),
					EarliestBlockHeight: 5200791,
					EarliestBlockTime:   mustTimeParse("2019-12-11T16:11:34Z"),
					CatchingUp:          false,
				},
				ValidatorInfo: coretypes.ValidatorInfo{
					Address:     mustHexToBytes("38FB765D0092470989360ECA1C89CD06C2C1583C"),
					PubKey:      mustGetPubKey("Z+8kntVegi1sQiWLYwFSVLNWqdAUGEy7lskL78gxLZI="),
					VotingPower: 0,
				},
			},
			mockError: nil,
			expectedResp: `{
  "jsonrpc": "2.0",
  "id": -1,
  "result": {
    "node_info": {
      "protocol_version": {
        "p2p": "8",
        "block": "11",
        "app": "0"
      },
      "id": "b93270b358a72a2db30089f3856475bb1f918d6d",
      "listen_addr": "tcp://0.0.0.0:26656",
      "network": "cosmoshub-4",
      "version": "v0.34.8",
      "channels": "40202122233038606100",
      "moniker": "aib-hub-node",
      "other": {
        "tx_index": "on",
        "rpc_address": "tcp://0.0.0.0:26657"
      }
    },
    "sync_info": {
      "latest_block_hash": "50F03C0EAACA8BCA7F9C14189ACE9C05A9A1BBB5268DB63DC6A3C848D1ECFD27",
      "latest_app_hash": "2316CFF7644219F4F15BEE456435F280E2B38955EEA6D4617CCB6D7ABF781C22",
      "latest_block_height": "5622165",
      "latest_block_time": "2021-03-25T14:00:43.356134226Z",
      "earliest_block_hash": "1455A0C15AC49BB506992EC85A3CD4D32367E53A087689815E01A524231C3ADF",
      "earliest_app_hash": "E3B0C44298FC1C149AFBF4C8996FB92427AE41E4649B934CA495991B7852B855",
      "earliest_block_height": "5200791",
      "earliest_block_time": "2019-12-11T16:11:34Z",
      "catching_up": false
    },
    "validator_info": {
      "address": "38FB765D0092470989360ECA1C89CD06C2C1583C",
      "pub_key": {
        "type": "tendermint/PubKeyEd25519",
        "value": "Z+8kntVegi1sQiWLYwFSVLNWqdAUGEy7lskL78gxLZI="
      },
      "voting_power": "0"
    }
  }
}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mocks.Client{}
			client.On(tt.name, mock.Anything).Return(tt.mockResp, tt.mockError)
			handler, err := GetHTTPHandler(client, log.TestingLogger())
			require.NoError(t, err)
			require.NotNil(t, handler)

			req := httptest.NewRequest(http.MethodPost, tt.endpoint, nil)
			resp := httptest.NewRecorder()
			handler.ServeHTTP(resp, req)

			s := resp.Body.String()
			assert.NotEmpty(t, s)

			assert.Equal(t, tt.mockCode, resp.Code)
			assert.JSONEq(t, tt.expectedResp, s)
		})
	}
}

func TestBlockNumber_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    BlockNumber
		wantErr bool
	}{
		{"Earliest", `"earliest"`, EarliestBlockNumber, false},
		{"Included", `"included"`, IncludedBlockNumber, false},
		{"PositiveInteger", `42`, BlockNumber(42), false},
		{"NegativeInteger", `-10`, BlockNumber(-10), false},
		{"Zero", `0`, BlockNumber(0), false},
		{"QuotedInteger", `"123"`, BlockNumber(123), false},
		{"InvalidString", `"invalid"`, BlockNumber(0), true},
		{"InvalidJSON", `{`, BlockNumber(0), true},
		{"UnsupportedType", `true`, BlockNumber(0), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got BlockNumber
			err := json.Unmarshal([]byte(tt.input), &got)
			if (err != nil) != tt.wantErr {
				t.Errorf("BlockNumber.UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("BlockNumber.UnmarshalJSON() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func mustGetPubKey(b64 string) ed25519.PubKey {
	decodeString, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		panic(err)
	}
	return decodeString
}

func mustHexToBytes(hexStr string) []byte {
	bytes, err := hex.DecodeString(hexStr)
	if err != nil {
		panic(err)
	}
	return bytes
}

func mustTimeParse(str string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, str)
	if err != nil {
		panic(err)
	}
	return t
}
