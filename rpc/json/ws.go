package json

import (
	"io"
	"net/http"

	"github.com/gorilla/websocket"
)

func (h *handler) publishEvent(remoteAddr string, data []byte) {
	h.logger.Debug("publish:", "addr", remoteAddr, "data", string(data))
	h.queue <- wsMsg{remoteAddr, data}
}

func (h *handler) close(remoteAddr string) {
	h.logger.Debug("closing WebSocket", "addr", remoteAddr)
	if conn, ok := h.conns[remoteAddr]; ok {
		err := conn.Close()
		if err != nil {
			h.logger.Error("failed to close WebSocket", "addr", remoteAddr, "error", err)
		}
		delete(h.conns, remoteAddr)
	}
}

func (h *handler) publishLoop() {
	for msg := range h.queue {
		conn, ok := h.conns[msg.remoteAddr]
		if !ok {
			h.logger.Error("ignoring message: no such connection", "remoteAddr", msg.remoteAddr)
			continue
		}
		writer, err := conn.NextWriter(websocket.TextMessage)
		if err != nil {
			h.logger.Error("failed to create writer", "error", err)
			continue
		}
		_, err = writer.Write(msg.data)
		if err != nil {
			h.logger.Error("failed to write message", "error", err)
		}
		if err = writer.Close(); err != nil {
			h.logger.Error("failed to close writer", "error", err)
		}
	}
}

func (h *handler) wsHandler(w http.ResponseWriter, r *http.Request) {
	// TODO(tzdybal): configuration options
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}

	wsConn, err := upgrader.Upgrade(w, r, nil)
	remoteAddr := wsConn.RemoteAddr().String()
	if err != nil {
		h.logger.Error("failed to update to WebSocket connection", "error", err, "address", remoteAddr)
	}
	defer func() {
		err := wsConn.Close()
		if err != nil {
			h.logger.Error("failed to close WebSocket connection", "err")
		}
	}()

	h.conns[remoteAddr] = wsConn

	wsConn.SetCloseHandler(func(code int, text string) error {
		h.logger.Debug("WebSocket connection closed", code, text)
		delete(h.conns, remoteAddr)
		return nil
	})

	for {
		mt, r, err := wsConn.NextReader()
		if err != nil {
			h.logger.Debug("failed to read next WebSocket message", "error", err)
		}

		if mt != websocket.TextMessage {
			h.logger.Error("expected text message")
			continue
		}
		req, err := http.NewRequest(http.MethodGet, "", r)
		req.RemoteAddr = remoteAddr
		if err != nil {
			h.logger.Error("failed to create request", "error", err)
			continue
		}

		writer, err := wsConn.NextWriter(websocket.TextMessage)
		if err != nil {
			h.logger.Error("failed to create writer", "error", err)
			continue
		}

		h.serveJSONRPC(newResponseWriter(writer), req)
		err = writer.Close()
		if err != nil {
			h.logger.Error("failed to close WebSocket writer", "error", err)
		}
	}

}

func newResponseWriter(w io.Writer) http.ResponseWriter {
	return &wsResponse{w}
}

// wsResponse is a simple implementation of http.ResponseWriter
type wsResponse struct {
	w io.Writer
}

var _ http.ResponseWriter = wsResponse{}

// Write use underlying writer to write response to WebSocket
func (w wsResponse) Write(bytes []byte) (int, error) {
	return w.w.Write(bytes)
}

func (w wsResponse) Header() http.Header {
	return http.Header{}

}

func (w wsResponse) WriteHeader(statusCode int) {
}
