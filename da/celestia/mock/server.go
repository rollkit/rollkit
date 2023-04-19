package mock

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	mux2 "github.com/gorilla/mux"

	"github.com/celestiaorg/go-cnc"

	"github.com/rollkit/rollkit/da"
	mockda "github.com/rollkit/rollkit/da/mock"
	"github.com/rollkit/rollkit/log"
	"github.com/rollkit/rollkit/store"
	"github.com/rollkit/rollkit/types"
)

// Server mocks celestia-node HTTP API.
type Server struct {
	mock              *mockda.DataAvailabilityLayerClient
	blockTime         time.Duration
	server            *http.Server
	logger            log.Logger
	headerNamespaceID string
	dataNamespaceID   string
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

	headerNamespaceID := types.NamespaceID{0, 1, 2, 3, 4, 5, 6, 7}
	dataNamespaceID := types.NamespaceID{7, 6, 5, 4, 3, 2, 1, 0}

	s.headerNamespaceID = hex.EncodeToString(headerNamespaceID[:])
	s.dataNamespaceID = hex.EncodeToString(dataNamespaceID[:])

	err = s.mock.Init(headerNamespaceID, dataNamespaceID, []byte(s.blockTime.String()), kvStore, s.logger)
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
	mux.HandleFunc("/submit_pfb", s.submit).Methods(http.MethodPost)
	mux.HandleFunc("/namespaced_shares/{namespace}/height/{height}", s.shares).Methods(http.MethodGet)
	mux.HandleFunc("/namespaced_data/{namespace}/height/{height}", s.data).Methods(http.MethodGet)

	return mux
}

func (s *Server) submit(w http.ResponseWriter, r *http.Request) {
	req := cnc.SubmitPFBRequest{}
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		s.writeError(w, err)
		return
	}

	data, err := hex.DecodeString(req.Data)
	if err != nil {
		s.writeError(w, err)
		return
	}

	var res da.ResultSubmitBlock
	blockHeader := types.SignedHeader{}
	blockData := types.Data{}

	if strings.Compare(s.headerNamespaceID, req.NamespaceID) == 0 {
		err = blockHeader.UnmarshalBinary(data)
		if err != nil {
			s.writeError(w, err)
			return
		}
		res = s.mock.SubmitBlockHeader(r.Context(), &blockHeader)
	} else if strings.Compare(s.dataNamespaceID, req.NamespaceID) == 0 {
		err = blockData.UnmarshalBinary(data)
		if err != nil {
			s.writeError(w, err)
			return
		}
		res = s.mock.SubmitBlockData(r.Context(), &blockData)
	} else {
		s.writeError(w, errors.New("unknown namespace to handle request"))
		return
	}

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

	var nShares []NamespacedShare
	var DAHeight uint64
	namespaceId := mux2.Vars(r)["namespace"]
	if strings.Compare(s.headerNamespaceID, namespaceId) == 0 {
		res := s.mock.RetrieveBlockHeaders(r.Context(), height)
		if res.Code != da.StatusSuccess {
			s.writeError(w, errors.New(res.Message))
			return
		}

		DAHeight = res.DAHeight
		for _, header := range res.Headers {
			blob, err := header.MarshalBinary()
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
	} else if strings.Compare(s.dataNamespaceID, namespaceId) == 0 {
		res := s.mock.RetrieveBlockData(r.Context(), height)
		if res.Code != da.StatusSuccess {
			s.writeError(w, errors.New(res.Message))
			return
		}

		DAHeight = res.DAHeight
		for _, data := range res.Data {
			blob, err := data.MarshalBinary()
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
	} else {
		s.writeError(w, errors.New("unknown namespace to handle request"))
		return
	}

	shares := make([]Share, len(nShares))
	for i := range nShares {
		shares[i] = nShares[i].Share
	}

	resp, err := json.Marshal(namespacedSharesResponse{
		Shares: shares,
		Height: DAHeight,
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

	var DAHeight uint64
	var data [][]byte
	namespaceId := mux2.Vars(r)["namespace"]
	if strings.Compare(s.headerNamespaceID, namespaceId) == 0 {
		res := s.mock.RetrieveBlockHeaders(r.Context(), height)
		if res.Code != da.StatusSuccess {
			s.writeError(w, errors.New(res.Message))
			return
		}

		DAHeight = res.DAHeight
		data = make([][]byte, len(res.Headers))
		for i := range res.Headers {
			data[i], err = res.Headers[i].MarshalBinary()
			if err != nil {
				s.writeError(w, err)
				return
			}
		}
	} else if strings.Compare(s.dataNamespaceID, namespaceId) == 0 {
		res := s.mock.RetrieveBlockData(r.Context(), height)
		if res.Code != da.StatusSuccess {
			s.writeError(w, errors.New(res.Message))
			return
		}

		DAHeight = res.DAHeight
		data = make([][]byte, len(res.Data))
		for i := range res.Data {
			data[i], err = res.Data[i].MarshalBinary()
			if err != nil {
				s.writeError(w, err)
				return
			}
		}
	} else {
		s.writeError(w, errors.New("unknown namespace to handle request"))
		return
	}

	resp, err := json.Marshal(namespacedDataResponse{
		Data:   data,
		Height: DAHeight,
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
