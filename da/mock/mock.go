package mock

import (
	"context"
	"encoding/hex"
	"errors"
	"math/rand"
	"sync/atomic"
	"time"

	ds "github.com/ipfs/go-datastore"

	"github.com/rollkit/rollkit/da"
	"github.com/rollkit/rollkit/log"
	"github.com/rollkit/rollkit/store"
	"github.com/rollkit/rollkit/types"
)

// DataAvailabilityLayerClient is intended only for usage in tests.
// It does actually ensures DA - it stores data in-memory.
type DataAvailabilityLayerClient struct {
	logger            log.Logger
	dalcKV            ds.Datastore
	daHeight          uint64
	dataCounter       uint64
	config            config
	headerNamespaceID types.NamespaceID
	dataNamespaceID   types.NamespaceID
}

const defaultBlockTime = 3 * time.Second

type config struct {
	BlockTime time.Duration
}

var _ da.DataAvailabilityLayerClient = &DataAvailabilityLayerClient{}
var _ da.BlockRetriever = &DataAvailabilityLayerClient{}

// Init is called once to allow DA client to read configuration and initialize resources.
func (m *DataAvailabilityLayerClient) Init(headerNamespaceID, dataNamespaceID types.NamespaceID, config []byte, dalcKV ds.Datastore, logger log.Logger) error {
	if headerNamespaceID == dataNamespaceID {
		return errors.New("header and data namespaces must be different")
	}
	m.headerNamespaceID = headerNamespaceID
	m.dataNamespaceID = dataNamespaceID
	m.logger = logger
	m.dalcKV = dalcKV
	m.daHeight = 1
	m.dataCounter = 1000000 // so that it doesn't overwrite the header heights
	if len(config) > 0 {
		var err error
		m.config.BlockTime, err = time.ParseDuration(string(config))
		if err != nil {
			return err
		}
	} else {
		m.config.BlockTime = defaultBlockTime
	}
	return nil
}

// Start implements DataAvailabilityLayerClient interface.
func (m *DataAvailabilityLayerClient) Start() error {
	m.logger.Debug("Mock Data Availability Layer Client starting")
	go func() {
		for {
			time.Sleep(m.config.BlockTime)
			m.updateDAHeight()
		}
	}()
	return nil
}

// Stop implements DataAvailabilityLayerClient interface.
func (m *DataAvailabilityLayerClient) Stop() error {
	m.logger.Debug("Mock Data Availability Layer Client stopped")
	return nil
}

// SubmitBlockHeader submits the passed in block header to the DA layer.
// This should create a transaction which (potentially)
// triggers a state transition in the DA layer.
func (m *DataAvailabilityLayerClient) SubmitBlockHeader(ctx context.Context, header *types.SignedHeader) da.ResultSubmitBlock {
	daHeight := atomic.LoadUint64(&m.daHeight)
	hash := header.Hash()

	m.logger.Debug("Submitting block header to DA layer!", "height", header.Height(), "dataLayerHeight", daHeight)

	blob, err := header.MarshalBinary()
	if err != nil {
		return da.ResultSubmitBlock{BaseResult: da.BaseResult{Code: da.StatusError, Message: err.Error()}}
	}

	err = m.dalcKV.Put(ctx, getKey(daHeight, uint64(header.Height())), hash[:])
	if err != nil {
		return da.ResultSubmitBlock{BaseResult: da.BaseResult{Code: da.StatusError, Message: err.Error()}}
	}

	err = m.dalcKV.Put(ctx, ds.NewKey(hex.EncodeToString(hash[:])+"/header"), blob)
	if err != nil {
		return da.ResultSubmitBlock{BaseResult: da.BaseResult{Code: da.StatusError, Message: err.Error()}}
	}

	return da.ResultSubmitBlock{
		BaseResult: da.BaseResult{
			Code:     da.StatusSuccess,
			Message:  "OK",
			DAHeight: daHeight,
		},
	}
}

// SubmitBlockData submits the passed in block data to the DA layer.
// This should create a transaction which (potentially)
// triggers a state transition in the DA layer.
func (m *DataAvailabilityLayerClient) SubmitBlockData(ctx context.Context, data *types.Data) da.ResultSubmitBlock {
	daHeight := atomic.LoadUint64(&m.daHeight)
	m.logger.Debug("Submitting block data to DA layer!", "dataLayerHeight", daHeight)

	hash, err := data.Hash()
	if err != nil {
		return da.ResultSubmitBlock{BaseResult: da.BaseResult{Code: da.StatusError, Message: err.Error()}}
	}

	blob, err := data.MarshalBinary()
	if err != nil {
		return da.ResultSubmitBlock{BaseResult: da.BaseResult{Code: da.StatusError, Message: err.Error()}}
	}

	// re-insert the hash to make sure the updated daHeight includes the header hash using which the data will be searched
	counter := atomic.AddUint64(&m.dataCounter, 1)
	err = m.dalcKV.Put(ctx, getKey(daHeight, counter), hash[:])
	if err != nil {
		return da.ResultSubmitBlock{BaseResult: da.BaseResult{Code: da.StatusError, Message: err.Error()}}
	}

	err = m.dalcKV.Put(ctx, ds.NewKey(hex.EncodeToString(hash[:])+"/data"), blob)
	if err != nil {
		return da.ResultSubmitBlock{BaseResult: da.BaseResult{Code: da.StatusError, Message: err.Error()}}
	}

	return da.ResultSubmitBlock{
		BaseResult: da.BaseResult{
			Code:     da.StatusSuccess,
			Message:  "OK",
			DAHeight: daHeight,
		},
	}
}

// CheckBlockHeaderAvailability queries DA layer to check data availability of block header corresponding to given da height.
func (m *DataAvailabilityLayerClient) CheckBlockHeaderAvailability(ctx context.Context, daHeight uint64) da.ResultCheckBlock {
	headersRes := m.RetrieveBlockHeaders(ctx, daHeight)
	return da.ResultCheckBlock{BaseResult: da.BaseResult{Code: headersRes.Code}, DataAvailable: len(headersRes.Headers) > 0}
}

// CheckBlockDataAvailability queries DA layer to check data availability of block data corresponding to given da height.
func (m *DataAvailabilityLayerClient) CheckBlockDataAvailability(ctx context.Context, daHeight uint64) da.ResultCheckBlock {
	datasRes := m.RetrieveBlockData(ctx, daHeight)
	return da.ResultCheckBlock{BaseResult: da.BaseResult{Code: datasRes.Code}, DataAvailable: len(datasRes.Data) > 0}
}

// RetrieveBlockHeaders returns block header at given height from data availability layer.
func (m *DataAvailabilityLayerClient) RetrieveBlockHeaders(ctx context.Context, daHeight uint64) da.ResultRetrieveBlockHeaders {
	if daHeight >= atomic.LoadUint64(&m.daHeight) {
		return da.ResultRetrieveBlockHeaders{BaseResult: da.BaseResult{Code: da.StatusError, Message: "block not found"}}
	}

	results, err := store.PrefixEntries(ctx, m.dalcKV, getPrefix(daHeight))
	if err != nil {
		return da.ResultRetrieveBlockHeaders{BaseResult: da.BaseResult{Code: da.StatusError, Message: err.Error()}}
	}

	var headers []*types.SignedHeader
	for result := range results.Next() {
		blob, err := m.dalcKV.Get(ctx, ds.NewKey(hex.EncodeToString(result.Entry.Value)+"/header"))
		if err != nil {
			// it is okay to not find keys that are /data
			continue
		}

		header := &types.SignedHeader{}
		err = header.UnmarshalBinary(blob)
		if err != nil {
			return da.ResultRetrieveBlockHeaders{BaseResult: da.BaseResult{Code: da.StatusError, Message: err.Error()}}
		}

		headers = append(headers, header)
	}

	return da.ResultRetrieveBlockHeaders{BaseResult: da.BaseResult{Code: da.StatusSuccess}, Headers: headers}
}

// RetrieveBlockData returns block data at given height from data availability layer.
func (m *DataAvailabilityLayerClient) RetrieveBlockData(ctx context.Context, daHeight uint64) da.ResultRetrieveBlockData {
	if daHeight >= atomic.LoadUint64(&m.daHeight) {
		return da.ResultRetrieveBlockData{BaseResult: da.BaseResult{Code: da.StatusError, Message: "block not found"}}
	}

	results, err := store.PrefixEntries(ctx, m.dalcKV, getPrefix(daHeight))
	if err != nil {
		return da.ResultRetrieveBlockData{BaseResult: da.BaseResult{Code: da.StatusError, Message: err.Error()}}
	}

	var dataArr []*types.Data
	for result := range results.Next() {
		blob, err := m.dalcKV.Get(ctx, ds.NewKey(hex.EncodeToString(result.Entry.Value)+"/data"))
		if err != nil {
			// it is okay to not find keys that are /header
			continue
		}

		data := &types.Data{}
		err = data.UnmarshalBinary(blob)
		if err != nil {
			return da.ResultRetrieveBlockData{BaseResult: da.BaseResult{Code: da.StatusError, Message: err.Error()}}
		}

		dataArr = append(dataArr, data)
	}

	return da.ResultRetrieveBlockData{BaseResult: da.BaseResult{Code: da.StatusSuccess}, Data: dataArr}
}

func getPrefix(daHeight uint64) string {
	return store.GenerateKey([]interface{}{daHeight})
}

func getKey(daHeight uint64, height uint64) ds.Key {
	return ds.NewKey(store.GenerateKey([]interface{}{daHeight, height}))
}

func (m *DataAvailabilityLayerClient) updateDAHeight() {
	blockStep := rand.Uint64()%10 + 1 //nolint:gosec
	atomic.AddUint64(&m.daHeight, blockStep)
}
