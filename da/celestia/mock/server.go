package mock

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	mux2 "github.com/gorilla/mux"

	"github.com/rollkit/celestia-openrpc/types/blob"
	"github.com/rollkit/celestia-openrpc/types/header"
	mockda "github.com/rollkit/rollkit/da/mock"
	"github.com/rollkit/rollkit/log"
	"github.com/rollkit/rollkit/store"
	"github.com/rollkit/rollkit/types"
)

type ErrorCode int

type respError struct {
	Code    ErrorCode       `json:"code"`
	Message string          `json:"message"`
	Meta    json.RawMessage `json:"meta,omitempty"`
}

func (e *respError) Error() string {
	if e.Code >= -32768 && e.Code <= -32000 {
		return fmt.Sprintf("RPC error (%d): %s", e.Code, e.Message)
	}
	return e.Message
}

type request struct {
	Jsonrpc string            `json:"jsonrpc"`
	ID      interface{}       `json:"id,omitempty"`
	Method  string            `json:"method"`
	Params  json.RawMessage   `json:"params"`
	Meta    map[string]string `json:"meta,omitempty"`
}

type response struct {
	Jsonrpc string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	ID      interface{} `json:"id"`
	Error   *respError  `json:"error,omitempty"`
}

// Server mocks celestia-node HTTP API.
type Server struct {
	mock      *mockda.DataAvailabilityLayerClient
	blockTime time.Duration
	server    *httptest.Server
	logger    log.Logger
}

// NewServer creates new instance of Server.
func NewServer(blockTime time.Duration, logger log.Logger) *Server {
	return &Server{
		mock:      new(mockda.DataAvailabilityLayerClient),
		blockTime: blockTime,
		logger:    logger,
	}
}

// Start starts HTTP server with given listener.
func (s *Server) Start() (string, error) {
	kvStore, err := store.NewDefaultInMemoryKVStore()
	if err != nil {
		return "", err
	}
	err = s.mock.Init([8]byte{}, []byte(s.blockTime.String()), kvStore, s.logger)
	if err != nil {
		return "", err
	}
	err = s.mock.Start()
	if err != nil {
		return "", err
	}
	s.server = httptest.NewServer(s.getHandler())
	s.logger.Debug("http server exited with", "error", err)
	return s.server.URL, nil
}

// Stop shuts down the Server.
func (s *Server) Stop() {
	s.server.Close()
}

func (s *Server) getHandler() http.Handler {
	mux := mux2.NewRouter()
	mux.HandleFunc("/", s.rpc).Methods(http.MethodPost)

	return mux
}

func (s *Server) rpc(w http.ResponseWriter, r *http.Request) {
	var req request
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		s.writeError(w, err)
		return
	}
	switch req.Method {
	case "header.GetByHeight":
		var params []interface{}
		err := json.Unmarshal(req.Params, &params)
		if err != nil {
			s.writeError(w, err)
			return
		}
		if len(params) != 1 {
			s.writeError(w, errors.New("expected 1 param: height (uint64)"))
			return
		}
		height := uint64(params[0].(float64))
		dah := s.mock.GetHeaderByHeight(height)
		resp := &response{
			Jsonrpc: "2.0",
			Result: header.ExtendedHeader{
				DAH: dah,
			},
			ID:    req.ID,
			Error: nil,
		}
		bytes, err := json.Marshal(resp)
		if err != nil {
			s.writeError(w, err)
			return
		}
		s.writeResponse(w, bytes)
	case "blob.GetAll":
		var params []interface{}
		err := json.Unmarshal(req.Params, &params)
		if err != nil {
			s.writeError(w, err)
			return
		}
		if len(params) != 2 {
			s.writeError(w, errors.New("expected 2 params: height (uint64), namespace (base64 string)"))
			return
		}
		height := params[0].(float64)
		nsBase64 := params[1].([]interface{})[0].(string)
		ns, err := base64.StdEncoding.DecodeString(nsBase64)
		if err != nil {
			s.writeError(w, err)
			return
		}
		block := s.mock.RetrieveBlocks(r.Context(), uint64(height))
		var blobs []blob.Blob
		for _, block := range block.Blocks {
			data, err := block.MarshalBinary()
			if err != nil {
				s.writeError(w, err)
				return
			}
			blob, err := blob.NewBlobV0(ns, data)
			if err != nil {
				s.writeError(w, err)
				return
			}
			blobs = append(blobs, *blob)
		}
		resp := &response{
			Jsonrpc: "2.0",
			Result:  blobs,
			ID:      req.ID,
			Error:   nil,
		}
		bytes, err := json.Marshal(resp)
		if err != nil {
			s.writeError(w, err)
			return
		}
		s.writeResponse(w, bytes)
	case "share.SharesAvailable":
		resp := &response{
			Jsonrpc: "2.0",
			ID:      req.ID,
			Error:   nil,
		}
		bytes, err := json.Marshal(resp)
		if err != nil {
			s.writeError(w, err)
			return
		}
		s.writeResponse(w, bytes)
	case "blob.Submit":
		var params []interface{}
		err := json.Unmarshal(req.Params, &params)
		if err != nil {
			s.writeError(w, err)
			return
		}
		if len(params) != 2 {
			s.writeError(w, errors.New("expected 2 params: data (base64 string) and options (map[string]interface{})"))
			return
		}
		// ignores the second parameter - options
		blocks := make([]*types.Block, len(params[0].([]interface{})))
		for i, data := range params[0].([]interface{}) {
			blockBase64 := data.(map[string]interface{})["data"].(string)
			blockData, err := base64.StdEncoding.DecodeString(blockBase64)
			if err != nil {
				s.writeError(w, err)
				return
			}
			blocks[i] = new(types.Block)
			err = blocks[i].UnmarshalBinary(blockData)
			if err != nil {
				s.writeError(w, err)
				return
			}
		}

		res := s.mock.SubmitBlocks(r.Context(), blocks)
		resp := &response{
			Jsonrpc: "2.0",
			Result:  res.DAHeight,
			ID:      req.ID,
			Error:   nil,
		}
		bytes, err := json.Marshal(resp)
		if err != nil {
			s.writeError(w, err)
			return
		}
		s.writeResponse(w, bytes)
	}
}

func (s *Server) writeResponse(w http.ResponseWriter, payload []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, err := w.Write(payload)
	if err != nil {
		s.logger.Error("failed to write response", "error", err)
	}
}

func (s *Server) writeError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	resp, jerr := json.Marshal(err.Error())
	if jerr != nil {
		s.logger.Error("failed to serialize error message", "error", jerr)
	}
	_, werr := w.Write(resp)
	if werr != nil {
		s.logger.Error("failed to write response", "error", werr)
	}
}
