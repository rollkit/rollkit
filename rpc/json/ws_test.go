package json

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	cmjson "github.com/cometbft/cometbft/libs/json"
	"github.com/go-kit/kit/transport/http/jsonrpc"

	"github.com/cometbft/cometbft/libs/log"
	cmtypes "github.com/cometbft/cometbft/types"
	"github.com/gorilla/rpc/v2/json2"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebSockets(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	_, local := getRPC(t, "TestWebSockets")
	handler, err := GetHTTPHandler(local, log.TestingLogger())
	require.NoError(err)

	srv := httptest.NewServer(handler)
	defer srv.Close()

	conn, resp, err := websocket.DefaultDialer.Dial(strings.Replace(srv.URL, "http://", "ws://", 1)+"/websocket", nil)
	require.NoError(err)
	require.NotNil(resp)
	require.NotNil(conn)
	defer func() {
		_ = conn.Close()
	}()

	assert.Equal(http.StatusSwitchingProtocols, resp.StatusCode)

	err = conn.WriteMessage(websocket.TextMessage, []byte(`
{
"jsonrpc": "2.0",
"method": "subscribe",
"id": 7,
"params": {
"query": "tm.event='NewBlock'"
}
}
`))
	require.NoError(err)

	err = conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	require.NoError(err)
	typ, msg, err := conn.ReadMessage()
	require.NoError(err)
	assert.Equal(websocket.TextMessage, typ)
	assert.NotEmpty(msg)

	// wait for new block event
	err = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	require.NoError(err)
	typ, msg, err = conn.ReadMessage()
	require.NoError(err)
	assert.Equal(websocket.TextMessage, typ)
	assert.NotEmpty(msg)
	var jsrpcResp jsonrpc.Response
	err = json.Unmarshal(msg, &jsrpcResp)
	require.NoError(err)
	var payload cmtypes.EventDataNewBlock
	err = cmjson.Unmarshal(jsrpcResp.Result, &payload)
	require.NoError(err)
	assert.NotNil(payload.ResultFinalizeBlock)
	assert.NotNil(payload.Block)
	assert.GreaterOrEqual(payload.Block.Height, int64(1))

	unsubscribeAllReq, err := json2.EncodeClientRequest("unsubscribe_all", &unsubscribeAllArgs{})
	require.NoError(err)
	require.NotEmpty(unsubscribeAllReq)
	req := httptest.NewRequest(http.MethodGet, "/", bytes.NewReader(unsubscribeAllReq))
	req.RemoteAddr = conn.LocalAddr().String()
	rsp := httptest.NewRecorder()
	handler.ServeHTTP(rsp, req)
	assert.Equal(http.StatusOK, rsp.Code)
	jsonResp := response{}
	require.NoError(json.Unmarshal(rsp.Body.Bytes(), &jsonResp))
	assert.Nil(jsonResp.Error)
}
