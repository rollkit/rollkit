package mock

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	mux2 "github.com/gorilla/mux"

	"github.com/celestiaorg/go-cnc"

	"github.com/rollkit/celestia-openrpc/types/core"
	"github.com/rollkit/celestia-openrpc/types/header"
	"github.com/rollkit/celestia-openrpc/types/sdk"
	"github.com/rollkit/celestia-openrpc/types/share"
	"github.com/rollkit/rollkit/da"
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
	server    *http.Server
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
func (s *Server) Start(listener net.Listener) error {
	kvStore, err := store.NewDefaultInMemoryKVStore()
	if err != nil {
		return err
	}
	err = s.mock.Init([8]byte{}, []byte(s.blockTime.String()), kvStore, s.logger)
	if err != nil {
		return err
	}
	err = s.mock.Start()
	if err != nil {
		return err
	}
	go func() {
		s.server = new(http.Server)
		s.server.Handler = s.getHandler()
		err := s.server.Serve(listener)
		s.logger.Debug("http server exited with", "error", err)
	}()
	return nil
}

// Stop shuts down the Server.
func (s *Server) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	_ = s.server.Shutdown(ctx)
}

func (s *Server) getHandler() http.Handler {
	mux := mux2.NewRouter()
	mux.HandleFunc("/", s.rpc).Methods(http.MethodPost)
	mux.HandleFunc("/submit_pfb", s.submit).Methods(http.MethodPost)
	mux.HandleFunc("/namespaced_shares/{namespace}/height/{height}", s.shares).Methods(http.MethodGet)
	mux.HandleFunc("/namespaced_data/{namespace}/height/{height}", s.data).Methods(http.MethodGet)

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
		var params []uint64
		json.Unmarshal(req.Params, &params)
		dah := s.mock.GetHeaderByHeight(params[0])
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
	case "share.SharesAvailable":
		var params []core.DataAvailabilityHeader
		json.Unmarshal(req.Params, &params)
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
	case "share.GetSharesByNamespace":
		var params []core.DataAvailabilityHeader
		json.Unmarshal(req.Params, &params)
		height := s.mock.GetHeightByHeader(&params[0])
		block := s.mock.RetrieveBlocks(r.Context(), height)
		row := share.NamespacedRow{}
		for _, block := range block.Blocks {
			share, err := block.MarshalBinary()
			if err != nil {
				s.writeError(w, err)
				return
			}
			row.Shares = append(row.Shares, share)
		}
		resp := &response{
			Jsonrpc: "2.0",
			Result: share.NamespacedShares{row},
			ID:      req.ID,
			Error:   nil,
		}
		bytes, err := json.Marshal(resp)
		if err != nil {
			s.writeError(w, err)
			return
		}
		s.writeResponse(w, bytes)
	case "state.SubmitPayForBlob":
		var params [][]byte
		json.Unmarshal(req.Params, &params)
		if len(params) != 4 {
			s.writeError(w, errors.New("expected 4 params"))
			return
		}
		block := types.Block{}
		blockData := params[1]
		if err != nil {
			s.writeError(w, err)
			return
		}
		err = block.UnmarshalBinary(blockData)
		if err != nil {
			s.writeError(w, err)
			return
		}

		res := s.mock.SubmitBlock(r.Context(), &block)
		resp := &response{
			Jsonrpc: "2.0",
			Result: &sdk.TxResponse{
				Height: int64(res.DAHeight),
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
	}
}

func (s *Server) submit(w http.ResponseWriter, r *http.Request) {
	req := cnc.SubmitPFBRequest{}
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		s.writeError(w, err)
		return
	}

	block := types.Block{}
	blockData, err := hex.DecodeString(req.Data)
	if err != nil {
		s.writeError(w, err)
		return
	}
	err = block.UnmarshalBinary(blockData)
	if err != nil {
		s.writeError(w, err)
		return
	}

	res := s.mock.SubmitBlock(r.Context(), &block)
	code := 0
	if res.Code != da.StatusSuccess {
		code = 3
	}

	resp, err := json.Marshal(cnc.TxResponse{
		Height: int64(res.DAHeight),
		Code:   uint32(code),
		RawLog: res.Message,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}

	s.writeResponse(w, resp)
}

func (s *Server) shares(w http.ResponseWriter, r *http.Request) {
	height, err := parseHeight(r)
	if err != nil {
		s.writeError(w, err)
		return
	}

	res := s.mock.RetrieveBlocks(r.Context(), height)
	if res.Code != da.StatusSuccess {
		s.writeError(w, errors.New(res.Message))
		return
	}

	var nShares []NamespacedShare
	for _, block := range res.Blocks {
		blob, err := block.MarshalBinary()
		if err != nil {
			s.writeError(w, err)
			return
		}
		delimited, err := marshalDelimited(blob)
		if err != nil {
			s.writeError(w, err)
		}
		nShares = appendToShares(nShares, []byte{1, 2, 3, 4, 5, 6, 7, 8}, delimited)
	}
	shares := make([]Share, len(nShares))
	for i := range nShares {
		shares[i] = nShares[i].Share
	}

	resp, err := json.Marshal(namespacedSharesResponse{
		Shares: shares,
		Height: res.DAHeight,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}

	s.writeResponse(w, resp)
}

func (s *Server) data(w http.ResponseWriter, r *http.Request) {
	height, err := parseHeight(r)
	if err != nil {
		s.writeError(w, err)
		return
	}

	res := s.mock.RetrieveBlocks(r.Context(), height)
	if res.Code != da.StatusSuccess {
		s.writeError(w, errors.New(res.Message))
		return
	}

	data := make([][]byte, len(res.Blocks))
	for i := range res.Blocks {
		data[i], err = res.Blocks[i].MarshalBinary()
		if err != nil {
			s.writeError(w, err)
			return
		}
	}

	resp, err := json.Marshal(namespacedDataResponse{
		Data:   data,
		Height: res.DAHeight,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}

	s.writeResponse(w, resp)
}

func parseHeight(r *http.Request) (uint64, error) {
	vars := mux2.Vars(r)

	height, err := strconv.ParseUint(vars["height"], 10, 64)
	if err != nil {
		return 0, err
	}
	return height, nil
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
