package mock

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	ds "github.com/ipfs/go-datastore"

	"github.com/rollkit/celestia-openrpc/types/core"
	"github.com/rollkit/rollkit/da"
	"github.com/rollkit/rollkit/log"
	"github.com/rollkit/rollkit/store"
	"github.com/rollkit/rollkit/types"
)

// DataAvailabilityLayerClient is intended only for usage in tests.
// It does actually ensures DA - it stores data in-memory.
type DataAvailabilityLayerClient struct {
	logger log.Logger
	dalcKV ds.Datastore

	daHeaders     map[uint64]*core.DataAvailabilityHeader
	daHeadersLock sync.RWMutex

	daHeight uint64
	config   config
}

const defaultBlockTime = 3 * time.Second

type config struct {
	BlockTime time.Duration
}

var _ da.DataAvailabilityLayerClient = &DataAvailabilityLayerClient{}
var _ da.BlockRetriever = &DataAvailabilityLayerClient{}

// Init is called once to allow DA client to read configuration and initialize resources.
func (m *DataAvailabilityLayerClient) Init(_ types.NamespaceID, config []byte, dalcKV ds.Datastore, logger log.Logger) error {
	m.logger = logger
	m.dalcKV = dalcKV
	m.daHeight = 1
	m.daHeaders = make(map[uint64]*core.DataAvailabilityHeader)

	eds, err := RandEDS(4)
	if err != nil {
		return err
	}
	dah := core.NewDataAvailabilityHeader(eds)
	m.daHeadersLock.Lock()
	m.daHeaders[m.daHeight] = &dah
	m.daHeadersLock.Unlock()

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

// GetHeaderByHeight returns the header at the given height.
func (m *DataAvailabilityLayerClient) GetHeaderByHeight(height uint64) *core.DataAvailabilityHeader {
	m.daHeadersLock.RLock()
	dah := m.daHeaders[height]
	m.daHeadersLock.RUnlock()
	return dah
}

func isEqual(headerA, headerB *core.DataAvailabilityHeader) bool {
	if len(headerA.RowRoots) != len(headerB.RowRoots) {
		return false
	}
	if len(headerA.ColumnRoots) != len(headerB.ColumnRoots) {
		return false
	}
	for i, row := range headerA.RowRoots {
		if !bytes.Equal(row, headerB.RowRoots[i]) {
			return false
		}
	}
	for i, col := range headerA.ColumnRoots {
		if !bytes.Equal(col, headerB.ColumnRoots[i]) {
			return false
		}
	}
	return true
}

// GetHeightByHeader returns the height for the given header.
func (m *DataAvailabilityLayerClient) GetHeightByHeader(dah *core.DataAvailabilityHeader) uint64 {
	daHeight := atomic.LoadUint64(&m.daHeight)
	for height := uint64(0); height < daHeight; height++ {
		m.daHeadersLock.RLock()
		header, ok := m.daHeaders[height]
		m.daHeadersLock.RUnlock()
		if !ok {
			continue
		}
		if isEqual(header, dah) {
			return height
		}
	}
	return 0
}

// SubmitBlocks submits the passed in blocks to the DA layer.
// This should create a transaction which (potentially)
// triggers a state transition in the DA layer.
func (m *DataAvailabilityLayerClient) SubmitBlocks(ctx context.Context, blocks []*types.Block) da.ResultSubmitBlocks {
	daHeight := atomic.LoadUint64(&m.daHeight)

	for _, block := range blocks {
		m.logger.Debug("Submitting blocks to DA layer!", "height", block.SignedHeader.Header.Height(), "dataLayerHeight", daHeight)

		hash := block.SignedHeader.Header.Hash()
		blob, err := block.MarshalBinary()
		if err != nil {
			return da.ResultSubmitBlocks{BaseResult: da.BaseResult{Code: da.StatusError, Message: err.Error()}}
		}

		err = m.dalcKV.Put(ctx, getKey(daHeight, uint64(block.SignedHeader.Header.Height())), hash[:])
		if err != nil {
			return da.ResultSubmitBlocks{BaseResult: da.BaseResult{Code: da.StatusError, Message: err.Error()}}
		}

		err = m.dalcKV.Put(ctx, ds.NewKey(hex.EncodeToString(hash[:])), blob)
		if err != nil {
			return da.ResultSubmitBlocks{BaseResult: da.BaseResult{Code: da.StatusError, Message: err.Error()}}
		}
	}
	return da.ResultSubmitBlocks{
		BaseResult: da.BaseResult{
			Code:     da.StatusSuccess,
			Message:  "OK",
			DAHeight: daHeight,
		},
	}
}

// RetrieveBlocks returns block at given height from data availability layer.
func (m *DataAvailabilityLayerClient) RetrieveBlocks(ctx context.Context, daHeight uint64) da.ResultRetrieveBlocks {
	if daHeight >= atomic.LoadUint64(&m.daHeight) {
		return da.ResultRetrieveBlocks{BaseResult: da.BaseResult{Code: da.StatusError, Message: "block not found"}}
	}

	results, err := store.PrefixEntries(ctx, m.dalcKV, getPrefix(daHeight))
	if err != nil {
		return da.ResultRetrieveBlocks{BaseResult: da.BaseResult{Code: da.StatusError, Message: err.Error()}}
	}

	var blocks []*types.Block
	for result := range results.Next() {
		blob, err := m.dalcKV.Get(ctx, ds.NewKey(hex.EncodeToString(result.Entry.Value)))
		if err != nil {
			return da.ResultRetrieveBlocks{BaseResult: da.BaseResult{Code: da.StatusError, Message: err.Error()}}
		}

		block := &types.Block{}
		err = block.UnmarshalBinary(blob)
		if err != nil {
			return da.ResultRetrieveBlocks{BaseResult: da.BaseResult{Code: da.StatusError, Message: err.Error()}}
		}
		blocks = append(blocks, block)
	}

	return da.ResultRetrieveBlocks{BaseResult: da.BaseResult{Code: da.StatusSuccess}, Blocks: blocks}
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
	eds, err := RandEDS(4)
	if err != nil {
		fmt.Println(err)
		return
	}
	dah := core.NewDataAvailabilityHeader(eds)
	m.daHeadersLock.Lock()
	m.daHeaders[atomic.LoadUint64(&m.daHeight)] = &dah
	defer m.daHeadersLock.Unlock()
}
