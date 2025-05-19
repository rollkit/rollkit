# ADR 021: Lazy Aggregation with DA Layer Consistency

## Changelog

- 2024-01-24: Initial draft
- 2024-01-24: Revised to use existing empty batch mechanism
- 2024-01-25: Updated with implementation details from aggregation.go

## Context

Rollkit's lazy aggregation mechanism currently produces blocks at set intervals when no transactions are present, and immediately when transactions are available. However, this approach creates inconsistency with the DA layer (Celestia) as empty blocks are not posted to the DA layer. This breaks the expected 1:1 mapping between DA layer blocks and execution layer blocks in EVM environments.

## Decision

Leverage the existing empty batch mechanism and `dataHashForEmptyTxs` to maintain block height consistency.

## Detailed Design

### Implementation Details

1. **Modified Batch Retrieval**:

    The batch retrieval mechanism has been modified to handle empty batches differently. Instead of discarding empty batches, we now return them with the ErrNoBatch error, allowing the caller to create empty blocks with proper timestamps. This ensures that block timing remains consistent even during periods of inactivity.

    ```go
    func (m *Manager) retrieveBatch(ctx context.Context) (*BatchData, error) {
        res, err := m.sequencer.GetNextBatch(ctx, req)
        if err != nil {
            return nil, err
        }

        if res != nil && res.Batch != nil {
            m.logger.Debug("Retrieved batch",
                "txCount", len(res.Batch.Transactions),
                "timestamp", res.Timestamp)

            var errRetrieveBatch error
            // Even if there are no transactions, return the batch with timestamp
            // This allows empty blocks to maintain proper timing
            if len(res.Batch.Transactions) == 0 {
                errRetrieveBatch = ErrNoBatch
            }
            // Even if there are no transactions, update lastBatchData so we don't
            // repeatedly emit the same empty batch, and persist it to metadata.
            if err := m.store.SetMetadata(ctx, LastBatchDataKey, convertBatchDataToBytes(res.BatchData)); err != nil {
                m.logger.Error("error while setting last batch hash", "error", err)
            }
            m.lastBatchData = res.BatchData
            return &BatchData{Batch: res.Batch, Time: res.Timestamp, Data: res.BatchData}, errRetrieveBatch
	    }
	    return nil, ErrNoBatch
    }
    ```

2. **Empty Block Creation**:

    The block publishing logic has been enhanced to create empty blocks when a batch with no transactions is received. This uses the special `dataHashForEmptyTxs` value to indicate an empty batch, maintaining the block height consistency with the DA layer while minimizing overhead.

    ```go
    // In publishBlock method
    batchData, err := m.retrieveBatch(ctx)
		if err != nil {
			if errors.Is(err, ErrNoBatch) {
				if batchData == nil {
					m.logger.Info("No batch retrieved from sequencer, skipping block production")
					return nil
				}
				m.logger.Info("Creating empty block", "height", newHeight)
			} else {
				return fmt.Errorf("failed to get transactions from batch: %w", err)
			}
		} else {
			if batchData.Before(lastHeaderTime) {
				return fmt.Errorf("timestamp is not monotonically increasing: %s < %s", batchData.Time, m.getLastBlockTime())
			}
			m.logger.Info("Creating and publishing block", "height", newHeight)
			m.logger.Debug("block info", "num_tx", len(batchData.Batch.Transactions))
		}

		header, data, err = m.createBlock(ctx, newHeight, lastSignature, lastHeaderHash, batchData)
		if err != nil {
			return err
		}

		if err = m.store.SaveBlockData(ctx, header, data, &signature); err != nil {
			return SaveBlockError{err}
		}
    ```

3. **Lazy Aggregation Loop**:

    A dedicated lazy aggregation loop has been implemented with dual timer mechanisms. The `lazyTimer` ensures blocks are produced at regular intervals even during network inactivity, while the `blockTimer` handles normal block production when transactions are available. Transaction notifications from the `Reaper` to the `Manager` are now handled via the `txNotifyCh` channel: when the `Reaper` detects new transactions, it calls `Manager.NotifyNewTransactions()`, which performs a non-blocking signal on this channel. See the tests in `block/lazy_aggregation_test.go` for verification of this behavior.

    ```go
    // In Reaper.SubmitTxs
    if r.manager != nil && len(newTxs) > 0 {
        r.logger.Debug("Notifying manager of new transactions")
        r.manager.NotifyNewTransactions() // Signals txNotifyCh
    }

    // In Manager.NotifyNewTransactions
    func (m *Manager) NotifyNewTransactions() {
        select {
        case m.txNotifyCh <- struct{}{}:
            // Successfully sent notification
        default:
            // Channel buffer is full, notification already pending
        }
    }
    // Modified lazyAggregationLoop
    func (m *Manager) lazyAggregationLoop(ctx context.Context, blockTimer *time.Timer) {
        // lazyTimer triggers block publication even during inactivity
        lazyTimer := time.NewTimer(0)
        defer lazyTimer.Stop()

        for {
            select {
            case <-ctx.Done():
                return

            case <-lazyTimer.C:
                m.logger.Debug("Lazy timer triggered block production")
                m.produceBlock(ctx, "lazy_timer", lazyTimer, blockTimer)

            case <-blockTimer.C:
                if m.txsAvailable {
                    m.produceBlock(ctx, "block_timer", lazyTimer, blockTimer)
                    m.txsAvailable = false
                } else {
                    // Ensure we keep ticking even when there are no txs
                    blockTimer.Reset(m.config.Node.BlockTime.Duration)
                }
            case <-m.txNotifyCh:
                m.txsAvailable = true
            }
        }
    }
    ```

4. **Block Production**:

    The block production function centralizes the logic for publishing blocks and resetting timers. It records the start time, attempts to publish a block, and then intelligently resets both timers based on the elapsed time. This ensures that block production remains on schedule even if the block creation process takes significant time.

    ```go
    func (m *Manager) produceBlock(ctx context.Context, trigger string, lazyTimer, blockTimer *time.Timer) {
        // Record the start time
        start := time.Now()

        // Attempt to publish the block
        if err := m.publishBlock(ctx); err != nil && ctx.Err() == nil {
            m.logger.Error("error while publishing block", "trigger", trigger, "error", err)
        } else {
            m.logger.Debug("Successfully published block", "trigger", trigger)
        }

        // Reset both timers for the next aggregation window
        lazyTimer.Reset(getRemainingSleep(start, m.config.Node.LazyBlockInterval.Duration))
        blockTimer.Reset(getRemainingSleep(start, m.config.Node.BlockTime.Duration))
    }
    ```

### Key Changes

1. Return batch with timestamp even when empty, allowing proper block timing
2. Use existing `dataHashForEmptyTxs` for empty block indication
3. Leverage current sync mechanisms that already handle empty blocks
4. Implement a dedicated lazy aggregation loop with two timers:
   - `blockTimer`: Triggers block production at regular intervals when transactions are available
   - `lazyTimer`: Ensures blocks are produced even during periods of inactivity
5. Maintain transaction availability tracking via the `txsAvailable` flag and notification channel

### Efficiency Considerations

- Minimal DA layer overhead for empty blocks
- Reuses existing empty block detection mechanism
- Maintains proper block timing using batch timestamps
- Intelligent timer management to account for block production time
- Non-blocking transaction notification channel to prevent backpressure

## Status

Implemented

## Consequences

### Positive

- Maintains consistent block heights between DA and execution layers
- Leverages existing empty block mechanisms
- Simpler implementation than sentinel-based approach
- Preserves proper block timing
- Provides deterministic block production even during network inactivity
- Reduces latency for transaction inclusion during active periods

### Negative

- Small DA layer overhead for empty blocks
- Additional complexity in timer management

### Neutral

- Requires careful handling of batch timestamps
- Maintains backward compatibility with existing Rollkit deployments

## References

- [Block Manager Implementation](../../block/manager.go)
- [Block Aggregation Implementation](../../block/aggregation.go)
- [Lazy Aggregation Tests](../../block/lazy_aggregation_test.go)
