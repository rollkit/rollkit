# ADR 021: Lazy Aggregation with DA Layer Consistency

## Changelog

- 2024-01-24: Initial draft
- 2024-01-24: Revised to use existing empty batch mechanism

## Context

Rollkit's lazy aggregation mechanism currently produces blocks at set intervals when no transactions are present, and immediately when transactions are available. However, this approach creates inconsistency with the DA layer (Celestia) as empty blocks are not posted to the DA layer. This breaks the expected 1:1 mapping between DA layer blocks and execution layer blocks in EVM environments.

## Decision

Leverage the existing empty batch mechanism and `dataHashForEmptyTxs` to maintain block height consistency.

## Detailed Design

### Implementation Details

1. **Modified Batch Retrieval**:

    ```go
func (m *Manager) retrieveBatch(ctx context.Context) (*BatchData, error) {
	res, err := m.sequencer.GetBatch(ctx)
	if err != nil {
		return nil, err
	}

	if res != nil && res.Batch != nil {
		// Even if there are no transactions, return the batch with timestamp
		// This allows empty blocks to maintain proper timing
		if len(res.Batch.Transactions) == 0 {
			return &BatchData{
				Batch: res.Batch,
				Time:  res.Timestamp,
				Data:  res.BatchData,
			}, ErrNoBatch
		}
		return &BatchData{
			Batch: res.Batch,
			Time:  res.Timestamp,
			Data:  res.BatchData,
		}, nil
	}
	return nil, ErrNoBatch
}
    }
    ```

2. **Header Creation for Empty Blocks**:

    ```go
    func (m *Manager) createHeader(ctx context.Context, batchData *BatchData) (*types.Header, error) {
        // Use batch timestamp even for empty blocks
        timestamp := batchData.Time

        if batchData.Transactions == nil || len(batchData.Transactions) == 0 {
            // Use dataHashForEmptyTxs to indicate empty batch
            return &types.Header{
                DataHash: dataHashForEmptyTxs,
                Timestamp: timestamp,
                // ... other header fields
            }, nil
        }
        // Normal header creation for non-empty blocks
        // ...
    }
    ```

3. **Block Syncing**:
The existing sync mechanism already handles empty blocks correctly:

    ```go
    func (m *Manager) handleEmptyDataHash(ctx context.Context, header *types.Header) {
        // Existing code that handles headers with dataHashForEmptyTxs
        // This allows nodes to sync empty blocks without waiting for data
        if bytes.Equal(header.DataHash, dataHashForEmptyTxs) {
            // ... existing empty block handling
        }
    }
    ```

### Key Changes

1. Return batch with timestamp even when empty, allowing proper block timing
2. Use existing `dataHashForEmptyTxs` for empty block indication
3. Leverage current sync mechanisms that already handle empty blocks
4. No new data structures or special transactions needed

### Efficiency Considerations

- Minimal DA layer overhead for empty blocks
- Reuses existing empty block detection mechanism
- Maintains proper block timing using batch timestamps

### Testing Strategy

1. **Unit Tests**:
   - Empty batch handling
   - Timestamp preservation
   - Block height consistency

2. **Integration Tests**:
   - DA layer block mapping
   - Empty block syncing
   - Block timing verification

## Status

Proposed

## Consequences

### Positive

- Maintains consistent block heights between DA and execution layers
- Leverages existing empty block mechanisms
- Simpler implementation than sentinel-based approach
- Preserves proper block timing

### Negative

- Small DA layer overhead for empty blocks

### Neutral

- Requires careful handling of batch timestamps

## References

- [Block Manager Implementation](block/manager.go)
- [Block Sync Implementation](block/sync.go)
- [Existing Empty Block Handling](block/sync.go#L170)
