package block

import (
	"context"
	"encoding/hex"
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
				"lastBatchHash", hex.EncodeToString(m.lastBatchHash))

			// Create a get next batch request
			req := coresequencer.GetNextBatchRequest{
				RollupId:      []byte(m.genesis.ChainID),
				LastBatchHash: m.lastBatchHash,
				// Set a reasonable max bytes limit for batch retrieval
				MaxBytes: uint64(1024 * 1024), // 1MB as a default max bytes
			}

			// Get the next batch from the sequencer
			res, err := m.sequencer.GetNextBatch(ctx, req)
			if err != nil {
				m.logger.Error("error while retrieving batch", "error", err)
				// Always reset timer on error
				batchTimer.Reset(m.config.Node.BlockTime)
				continue
			}

			if res != nil && res.Batch != nil {
				m.logger.Debug("Retrieved batch",
					"txCount", len(res.Batch.Transactions),
					"timestamp", res.Timestamp)

				if h, err := res.Batch.Hash(); err == nil {
					// Verify the batch if we're not in proposer mode
					// Proposers don't need to verify since they create the batch
					if !m.isProposer {
						verifyReq := coresequencer.VerifyBatchRequest{
							RollupId:  []byte(m.genesis.ChainID),
							BatchHash: h,
						}

						verifyRes, err := m.sequencer.VerifyBatch(ctx, verifyReq)
						if err != nil {
							m.logger.Error("error while verifying batch", "error", err)
							batchTimer.Reset(m.config.Node.BlockTime)
							continue
						}

						if !verifyRes.Status {
							m.logger.Error("batch verification failed", "batchHash", hex.EncodeToString(h))
							batchTimer.Reset(m.config.Node.BlockTime)
							continue
						}

						m.logger.Debug("Batch verified successfully", "batchHash", hex.EncodeToString(h))
					}

					// Add the verified batch to the queue
					m.bq.AddBatch(BatchWithTime{Batch: res.Batch, Time: res.Timestamp})

					if len(res.Batch.Transactions) != 0 {
						if err := m.store.SetMetadata(ctx, LastBatchHashKey, h); err != nil {
							m.logger.Error("error while setting last batch hash", "error", err)
						}
						m.lastBatchHash = h
					}
				} else {
					m.logger.Error("failed to hash batch", "error", err)
				}
			} else {
				m.logger.Debug("No batch available")
			}

			// Always reset timer
			elapsed := time.Since(start)
			remainingSleep := m.config.Node.BlockTime - elapsed
			if remainingSleep < 0 {
				remainingSleep = 0
			}
			batchTimer.Reset(remainingSleep)
		}
	}
}

func (m *Manager) getTxsFromBatch() ([][]byte, *time.Time, error) {
	batch := m.bq.Next()
	if batch == nil {
		// batch is nil when there is nothing to process
		// this is expected when there are no transactions in the base layer
		m.logger.Debug("No batch available in the batch queue")
		return nil, nil, ErrNoBatch
	}

	m.logger.Debug("Got batch from queue",
		"txCount", len(batch.Transactions),
		"timestamp", batch.Time)

	// If batch has no transactions, we still want to create a block with the timestamp
	// This is important for based sequencing to maintain chain liveness even without transactions
	if len(batch.Transactions) == 0 {
		m.logger.Debug("Batch has no transactions, will create empty block")
	}

	return batch.Transactions, &batch.Time, nil
}
