package block

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"

	coresequencer "github.com/rollkit/rollkit/core/sequencer"
)

// BatchRetrieveLoop is responsible for retrieving batches from the sequencer.
func (m *Manager) BatchRetrieveLoop(ctx context.Context) {
	m.logger.Info("Starting BatchRetrieveLoop")
	batchTimer := time.NewTimer(0)
	defer batchTimer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-batchTimer.C:
			start := time.Now()
			m.logger.Debug("Attempting to retrieve next batch",
				"chainID", m.genesis.ChainID,
				"lastBatchData", m.lastBatchData)

			req := coresequencer.GetNextBatchRequest{
				RollupId:      []byte(m.genesis.ChainID),
				LastBatchData: m.lastBatchData,
			}

			res, err := m.sequencer.GetNextBatch(ctx, req)
			if err != nil {
				m.logger.Error("error while retrieving batch", "error", err)
				// Always reset timer on error
				batchTimer.Reset(m.config.Node.BlockTime.Duration)
				continue
			}

			if res != nil && res.Batch != nil {
				m.logger.Debug("Retrieved batch",
					"txCount", len(res.Batch.Transactions),
					"timestamp", res.Timestamp)

				m.bq.Add(BatchData{Batch: res.Batch, Time: res.Timestamp, Data: res.BatchData})
				if len(res.Batch.Transactions) != 0 {
					h := convertBatchDataToBytes(res.BatchData)
					if err := m.store.SetMetadata(ctx, LastBatchDataKey, h); err != nil {
						m.logger.Error("error while setting last batch hash", "error", err)
					}
					m.lastBatchData = res.BatchData
				}

			} else {
				m.logger.Debug("No batch available")
			}

			// Always reset timer
			elapsed := time.Since(start)
			remainingSleep := m.config.Node.BlockTime.Duration - elapsed
			if remainingSleep < 0 {
				remainingSleep = 0
			}
			batchTimer.Reset(remainingSleep)
		}
	}
}

func (m *Manager) getTxsFromBatch() (*BatchData, error) {
	batch := m.bq.Next()
	if batch == nil {
		// batch is nil when there is nothing to process
		return nil, ErrNoBatch
	}
	return &BatchData{Batch: batch.Batch, Time: batch.Time, Data: batch.Data}, nil
}

// AggregationLoop is responsible for aggregating transactions into rollup-blocks.
func (m *Manager) AggregationLoop(ctx context.Context) {
	initialHeight := m.genesis.InitialHeight //nolint:gosec
	height := m.store.Height()
	var delay time.Duration

	// TODO(tzdybal): double-check when https://github.com/celestiaorg/rollmint/issues/699 is resolved
	if height < initialHeight {
		delay = time.Until(m.genesis.GenesisDAStartHeight.Add(m.config.Node.BlockTime.Duration))
	} else {
		lastBlockTime := m.getLastBlockTime()
		delay = time.Until(lastBlockTime.Add(m.config.Node.BlockTime.Duration))
	}

	if delay > 0 {
		m.logger.Info("Waiting to produce block", "delay", delay)
		time.Sleep(delay)
	}

	// blockTimer is used to signal when to build a block based on the
	// rollup block time. A timer is used so that the time to build a block
	// can be taken into account.
	blockTimer := time.NewTimer(0)
	defer blockTimer.Stop()

	// Lazy Aggregator mode.
	// In Lazy Aggregator mode, blocks are built only when there are
	// transactions or every LazyBlockTime.
	if m.config.Node.LazyAggregator {
		m.lazyAggregationLoop(ctx, blockTimer)
		return
	}

	m.normalAggregationLoop(ctx, blockTimer)
}

func (m *Manager) lazyAggregationLoop(ctx context.Context, blockTimer *time.Timer) {
	// start is used to track the start time of the block production period
	start := time.Now()
	// lazyTimer is used to signal when a block should be built in
	// lazy mode to signal that the chain is still live during long
	// periods of inactivity.
	lazyTimer := time.NewTimer(0)
	defer lazyTimer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		// the m.bq.notifyCh channel is signalled when batch becomes available in the batch queue
		case _, ok := <-m.bq.NotifyCh():
			if ok && !m.buildingBlock {
				// set the buildingBlock flag to prevent multiple calls to reset the time
				m.buildingBlock = true
				// Reset the block timer based on the block time.
				blockTimer.Reset(m.getRemainingSleep(start))
			}
			continue
		case <-lazyTimer.C:
		case <-blockTimer.C:
		}
		// Define the start time for the block production period
		start = time.Now()
		if err := m.publishBlock(ctx); err != nil && ctx.Err() == nil {
			m.logger.Error("error while publishing block", "error", err)
		}
		// unset the buildingBlocks flag
		m.buildingBlock = false
		// Reset the lazyTimer to produce a block even if there
		// are no transactions as a way to signal that the chain
		// is still live.
		lazyTimer.Reset(m.getRemainingSleep(start))
	}
}

func (m *Manager) normalAggregationLoop(ctx context.Context, blockTimer *time.Timer) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-blockTimer.C:
			// Define the start time for the block production period
			start := time.Now()
			if err := m.publishBlock(ctx); err != nil && ctx.Err() == nil {
				m.logger.Error("error while publishing block", "error", err)
			}
			// Reset the blockTimer to signal the next block production
			// period based on the block time.
			blockTimer.Reset(m.getRemainingSleep(start))
		}
	}
}

// getRemainingSleep calculates the remaining sleep time based on config and a start time.
func (m *Manager) getRemainingSleep(start time.Time) time.Duration {
	elapsed := time.Since(start)
	interval := m.config.Node.BlockTime.Duration

	if m.config.Node.LazyAggregator {
		if m.buildingBlock && elapsed >= interval {
			// Special case to give time for transactions to accumulate if we
			// are coming out of a period of inactivity.
			return (interval * time.Duration(defaultLazySleepPercent) / 100)
		} else if !m.buildingBlock {
			interval = m.config.Node.LazyBlockTime.Duration
		}
	}

	if elapsed < interval {
		return interval - elapsed
	}

	return 0
}

func convertBatchDataToBytes(batchData [][]byte) []byte {
	// If batchData is nil or empty, return an empty byte slice
	if len(batchData) == 0 {
		return []byte{}
	}

	// For a single item, we still need to length-prefix it for consistency
	// First, calculate the total size needed
	// Format: 4 bytes (length) + data for each entry
	totalSize := 0
	for _, data := range batchData {
		totalSize += 4 + len(data) // 4 bytes for length prefix + data length
	}

	// Allocate buffer with calculated capacity
	result := make([]byte, 0, totalSize)

	// Add length-prefixed data
	for _, data := range batchData {
		// Encode length as 4-byte big-endian integer
		lengthBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(lengthBytes, uint32(len(data)))

		// Append length prefix
		result = append(result, lengthBytes...)

		// Append actual data
		result = append(result, data...)
	}

	return result
}

// bytesToBatchData converts a length-prefixed byte array back to a slice of byte slices
func bytesToBatchData(data []byte) ([][]byte, error) {
	if len(data) == 0 {
		return [][]byte{}, nil
	}

	var result [][]byte
	offset := 0

	for offset < len(data) {
		// Check if we have at least 4 bytes for the length prefix
		if offset+4 > len(data) {
			return nil, fmt.Errorf("corrupted data: insufficient bytes for length prefix at offset %d", offset)
		}

		// Read the length prefix
		length := binary.BigEndian.Uint32(data[offset : offset+4])
		offset += 4

		// Check if we have enough bytes for the data
		if offset+int(length) > len(data) {
			return nil, fmt.Errorf("corrupted data: insufficient bytes for entry of length %d at offset %d", length, offset)
		}

		// Extract the data entry
		entry := make([]byte, length)
		copy(entry, data[offset:offset+int(length)])
		result = append(result, entry)

		// Move to the next entry
		offset += int(length)
	}

	return result, nil
}
