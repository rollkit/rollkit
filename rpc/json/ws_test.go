package json

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cometbft/cometbft/libs/log"
	cmtypes "github.com/cometbft/cometbft/types"
	"github.com/gorilla/rpc/v2/json2"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebSockets(t *testing.T) {
	_, local := getRPC(t)
	handler, err := GetHTTPHandler(local, log.TestingLogger())
	require.NoError(t, err)

	srv := httptest.NewServer(handler)

	conn, resp, err := websocket.DefaultDialer.Dial(strings.Replace(srv.URL, "http://", "ws://", 1)+"/websocket", nil)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, conn)
	defer func() {
		_ = conn.Close()
	}()

	assert.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)

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
	assert.NoError(t, err)

	err = conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	assert.NoError(t, err)
	typ, msg, err := conn.ReadMessage()
	assert.NoError(t, err)
	assert.Equal(t, websocket.TextMessage, typ)
	assert.NotEmpty(t, msg)

	// wait for new block event
	err = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	assert.NoError(t, err)
	typ, msg, err = conn.ReadMessage()
	assert.NoError(t, err)
	assert.Equal(t, websocket.TextMessage, typ)
	assert.NotEmpty(t, msg)
	var payload cmtypes.EventDataNewBlock
	err = json.Unmarshal(msg, &payload)
	assert.NoError(t, err)
	assert.NotNil(t, payload.ResultBeginBlock)
	assert.NotNil(t, payload.Block)
	assert.GreaterOrEqual(t, payload.Block.Height, int64(1))
	assert.NotNil(t, payload.ResultEndBlock)

	unsubscribeAllReq, err := json2.EncodeClientRequest("unsubscribe_all", &unsubscribeAllArgs{})
	require.NoError(t, err)
	require.NotEmpty(t, unsubscribeAllReq)
	req := httptest.NewRequest(http.MethodGet, "/", bytes.NewReader(unsubscribeAllReq))
	req.RemoteAddr = conn.LocalAddr().String()
	rsp := httptest.NewRecorder()
	handler.ServeHTTP(rsp, req)
	assert.Equal(t, http.StatusOK, rsp.Code)
	jsonResp := response{}
	assert.NoError(t, json.Unmarshal(rsp.Body.Bytes(), &jsonResp))
	assert.Nil(t, jsonResp.Error)
}
