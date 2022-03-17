package mock

import (
	"encoding/binary"
	"errors"
	"sync/atomic"

	"github.com/celestiaorg/optimint/da"
	"github.com/celestiaorg/optimint/log"
	"github.com/celestiaorg/optimint/store"
	"github.com/celestiaorg/optimint/types"
)

// MockDataAvailabilityLayerClient is intended only for usage in tests.
// It does actually ensures DA - it stores data in-memory.
type MockDataAvailabilityLayerClient struct {
	logger   log.Logger
	dalcKV   store.KVStore
	daHeight uint64
}

var _ da.DataAvailabilityLayerClient = &MockDataAvailabilityLayerClient{}
var _ da.BlockRetriever = &MockDataAvailabilityLayerClient{}

// Init is called once to allow DA client to read configuration and initialize resources.
func (m *MockDataAvailabilityLayerClient) Init(config []byte, dalcKV store.KVStore, logger log.Logger) error {
	m.logger = logger
	m.dalcKV = dalcKV
	return nil
}

// Start implements DataAvailabilityLayerClient interface.
func (m *MockDataAvailabilityLayerClient) Start() error {
	m.logger.Debug("Mock Data Availability Layer Client starting")
	return nil
}

// Stop implements DataAvailabilityLayerClient interface.
func (m *MockDataAvailabilityLayerClient) Stop() error {
	m.logger.Debug("Mock Data Availability Layer Client stopped")
	return nil
}

// SubmitBlock submits the passed in block to the DA layer.
// This should create a transaction which (potentially)
// triggers a state transition in the DA layer.
func (m *MockDataAvailabilityLayerClient) SubmitBlock(block *types.Block) da.ResultSubmitBlock {
	height := atomic.AddUint64(&m.daHeight, 1)
	m.logger.Debug("Submitting block to DA layer!", "height", block.Header.Height, "dataLayerHeight", height)

	hash := block.Header.Hash()
	blob, err := block.MarshalBinary()
	if err != nil {
		return da.ResultSubmitBlock{DAResult: da.DAResult{Code: da.StatusError, Message: err.Error()}}
	}

	// TODO(tzdybal): more than one optimint block per "mock" DA height
	err = m.dalcKV.Set(getKey(height), hash[:])
	if err != nil {
		return da.ResultSubmitBlock{DAResult: da.DAResult{Code: da.StatusError, Message: err.Error()}}
	}
	err = m.dalcKV.Set(hash[:], blob)
	if err != nil {
		return da.ResultSubmitBlock{DAResult: da.DAResult{Code: da.StatusError, Message: err.Error()}}
	}

	return da.ResultSubmitBlock{
		DAResult: da.DAResult{
			Code:            da.StatusSuccess,
			Message:         "OK",
			DataLayerHeight: height,
		},
	}
}

// CheckBlockAvailability queries DA layer to check data availability of block corresponding to given header.
func (m *MockDataAvailabilityLayerClient) CheckBlockAvailability(dataLayerHeight uint64) da.ResultCheckBlock {
	hash, err := m.dalcKV.Get(getKey(dataLayerHeight))
	if errors.Is(err, store.ErrKeyNotFound) {
		return da.ResultCheckBlock{DAResult: da.DAResult{Code: da.StatusSuccess}, DataAvailable: false}
	}
	if err != nil {
		return da.ResultCheckBlock{DAResult: da.DAResult{Code: da.StatusError, Message: err.Error()}, DataAvailable: false}
	}
	_, err = m.dalcKV.Get(hash[:])
	if errors.Is(err, store.ErrKeyNotFound) {
		return da.ResultCheckBlock{DAResult: da.DAResult{Code: da.StatusSuccess}, DataAvailable: false}
	}
	if err != nil {
		return da.ResultCheckBlock{DAResult: da.DAResult{Code: da.StatusError, Message: err.Error()}, DataAvailable: false}
	}
	return da.ResultCheckBlock{DAResult: da.DAResult{Code: da.StatusSuccess}, DataAvailable: true}
}

// RetrieveBlocks returns block at given height from data availability layer.
func (m *MockDataAvailabilityLayerClient) RetrieveBlocks(dataLayerHeight uint64) da.ResultRetrieveBlock {
	hash, err := m.dalcKV.Get(getKey(dataLayerHeight))
	if err != nil {
		return da.ResultRetrieveBlock{DAResult: da.DAResult{Code: da.StatusError, Message: err.Error()}}
	}
	blob, err := m.dalcKV.Get(hash)
	if err != nil {
		return da.ResultRetrieveBlock{DAResult: da.DAResult{Code: da.StatusError, Message: err.Error()}}
	}

	block := &types.Block{}
	err = block.UnmarshalBinary(blob)
	if err != nil {
		return da.ResultRetrieveBlock{DAResult: da.DAResult{Code: da.StatusError, Message: err.Error()}}
	}

	return da.ResultRetrieveBlock{DAResult: da.DAResult{Code: da.StatusSuccess}, Blocks: []*types.Block{block}}
}

func getKey(height uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, height)
	return b
}
