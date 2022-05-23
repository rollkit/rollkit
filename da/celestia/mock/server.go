package mock

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/celestiaorg/optimint/da"
	"github.com/celestiaorg/optimint/store"
	"github.com/celestiaorg/optimint/types"
	mux2 "github.com/gorilla/mux"
	"net"
	"net/http"
	"strconv"
	"time"

	mockda "github.com/celestiaorg/optimint/da/mock"
	"github.com/celestiaorg/optimint/libs/cnrc"
	"github.com/celestiaorg/optimint/log"
)

type Server struct {
	mock      *mockda.MockDataAvailabilityLayerClient
	blockTime time.Duration
	logger    log.Logger
}

func NewServer(blockTime time.Duration, logger log.Logger) *Server {
	return &Server{
		mock:      new(mockda.MockDataAvailabilityLayerClient),
		blockTime: blockTime,
		logger:    logger,
	}
}

func (s *Server) Start(listener net.Listener) error {
	s.mock.Init([]byte(s.blockTime.String()), store.NewDefaultInMemoryKVStore(), s.logger)
	err := s.mock.Start()
	if err != nil {
		return err
	}
	go func() {
		err := http.Serve(listener, s.getHandler())
		s.logger.Error("listener", "error", err)
	}()
	return nil
}

func (s *Server) getHandler() http.Handler {
	mux := mux2.NewRouter()
	mux.HandleFunc("/submit_pfd", s.submit).Methods(http.MethodPost)
	mux.HandleFunc("/namespaced_shares/{namespace}/height/{height}", s.shares).Methods(http.MethodGet)

	return mux
}

func (s *Server) submit(w http.ResponseWriter, r *http.Request) {
	req := cnrc.SubmitPFDRequest{}
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

	res := s.mock.SubmitBlock(&block)

	resp, err := json.Marshal(cnrc.TxResponse{
		Height: int64(res.DAHeight),
		Code:   uint32(res.Code),
		RawLog: res.Message,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}

	s.writeResponse(w, resp)
}

func (s *Server) shares(w http.ResponseWriter, r *http.Request) {
	vars := mux2.Vars(r)

	height, err := strconv.ParseUint(vars["height"], 10, 64)
	if err != nil {
		s.writeError(w, err)
		return
	}

	res := s.mock.RetrieveBlocks(height)
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
		delimited, err := MarshalDelimited(blob)
		if err != nil {
			s.writeError(w, err)
		}
		nShares = AppendToShares(nShares, []byte{1, 2, 3, 4, 5, 6, 7, 8}, delimited)
	}
	shares := make([]Share, len(nShares))
	for i := range nShares {
		shares[i] = nShares[i].Share
	}

	resp, err := json.Marshal(NamespacedSharesResponse{
		Shares: shares,
		Height: res.DAHeight,
	})
	if err != nil {
		s.writeError(w, err)
		return
	}

	s.writeResponse(w, resp)
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

const (
	ShareSize     = 256
	NamespaceSize = 8
	MsgShareSize  = ShareSize - NamespaceSize
)

// splitMessage breaks the data in a message into the minimum number of
// namespaced shares
func splitMessage(rawData []byte, nid []byte) []NamespacedShare {
	shares := make([]NamespacedShare, 0)
	firstRawShare := append(append(
		make([]byte, 0, ShareSize),
		nid...),
		rawData[:MsgShareSize]...,
	)
	shares = append(shares, NamespacedShare{firstRawShare, nid})
	rawData = rawData[MsgShareSize:]
	for len(rawData) > 0 {
		shareSizeOrLen := min(MsgShareSize, len(rawData))
		rawShare := append(append(
			make([]byte, 0, ShareSize),
			nid...),
			rawData[:shareSizeOrLen]...,
		)
		paddedShare := zeroPadIfNecessary(rawShare, ShareSize)
		share := NamespacedShare{paddedShare, nid}
		shares = append(shares, share)
		rawData = rawData[shareSizeOrLen:]
	}
	return shares
}

// Share contains the raw share data without the corresponding namespace.
type Share []byte

// NamespacedShare extends a Share with the corresponding namespace.
type NamespacedShare struct {
	Share
	ID []byte
}

func min(a, b int) int {
	if a <= b {
		return a
	}
	return b
}

func zeroPadIfNecessary(share []byte, width int) []byte {
	oldLen := len(share)
	if oldLen < width {
		missingBytes := width - oldLen
		padByte := []byte{0}
		padding := bytes.Repeat(padByte, missingBytes)
		share = append(share, padding...)
		return share
	}
	return share
}

// MarshalDelimited marshals the raw data (excluding the namespace) of this
// message and prefixes it with the length of that encoding.
func MarshalDelimited(data []byte) ([]byte, error) {
	lenBuf := make([]byte, binary.MaxVarintLen64)
	length := uint64(len(data))
	n := binary.PutUvarint(lenBuf, length)
	return append(lenBuf[:n], data...), nil
}

// appendToShares appends raw data as shares.
// Used for messages.
func AppendToShares(shares []NamespacedShare, nid []byte, rawData []byte) []NamespacedShare {
	if len(rawData) <= MsgShareSize {
		rawShare := append(append(
			make([]byte, 0, len(nid)+len(rawData)),
			nid...),
			rawData...,
		)
		paddedShare := zeroPadIfNecessary(rawShare, ShareSize)
		share := NamespacedShare{paddedShare, nid}
		shares = append(shares, share)
	} else { // len(rawData) > MsgShareSize
		shares = append(shares, splitMessage(rawData, nid)...)
	}
	return shares
}

// NamespacedSharesResponse represents the response to a
// SharesByNamespace request.
type NamespacedSharesResponse struct {
	Shares []Share `json:"shares"`
	Height uint64  `json:"height"`
}
