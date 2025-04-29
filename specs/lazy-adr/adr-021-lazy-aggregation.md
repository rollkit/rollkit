# ADR 021: Lazy Aggregation with DA Layer Consistency

## Changelog

- 2024-01-24: Initial draft

## Context

Rollkit's lazy aggregation mechanism currently produces blocks at set intervals when no transactions are present, and immediately when transactions are available. However, this approach creates inconsistency with the DA layer (Celestia) as empty blocks are not posted to the DA layer. This breaks the expected 1:1 mapping between DA layer blocks and execution layer blocks in EVM environments.

The current implementation in `block/aggregation.go` needs to be enhanced to maintain consistency between DA layer blocks and execution layer blocks, even during periods of inactivity.

## Alternative Approaches

### 1. Skip Empty Blocks Entirely

- Continue current behavior of not posting empty blocks
- **Rejected** because it breaks EVM execution layer expectations and creates inconsistent block mapping

### 2. Post Full Block Structure for Empty Blocks

- Post regular blocks with empty transaction lists
- **Rejected** because it wastes DA layer space unnecessarily

### 3. Sentinel Empty Block Marker (Chosen)

- Post a special single-byte transaction to mark empty blocks
- Optimize DA layer space usage while maintaining block consistency
- Provides clear distinction between empty and failed/missed blocks

## Decision

Implement a sentinel-based empty block marking system that:

1. Posts a special sentinel transaction to the DA layer for empty blocks
2. Maintains block height consistency between DA and execution layers
3. Optimizes DA layer space usage
4. Preserves existing lazy aggregation timing behavior

## Detailed Design

### User Requirements

- Maintain consistent block heights between DA and execution layers
- Minimize DA layer costs for empty blocks
- Preserve existing lazy aggregation performance characteristics
- Support EVM execution layer expectations

### Systems Affected

1. Block Manager (`block/aggregation.go`)
2. Transaction Processing Pipeline
3. DA Layer Interface
4. Block Execution Logic

### Data Structures

```go
const (
    // EmptyBlockSentinel is a special byte sequence marking empty blocks
    EmptyBlockSentinel = []byte{0x00, 0xE0, 0x0E} // "EMPTY" in minimal form
)

type BlockData struct {
    Txs [][]byte
    IsEmpty bool
}
```

### Implementation Details

1. **Empty Block Detection**:

    ```go
    func (m *Manager) isEmptyBlock(txs [][]byte) bool {
        return len(txs) == 1 && bytes.Equal(txs[0], EmptyBlockSentinel)
    }
    ```

2. **Modified Aggregation Loop**:

    ```go
    func (m *Manager) lazyAggregationLoop(ctx context.Context, blockTimer *timTimer) {
        // ... existing timer setup ...

        for {
            select {
            case <-ctx.Done():
                return
            case <-lazyTimer.C:
                // No transactions available, create empty block
                if err := m.publishEmptyBlock(ctx); err != nil {
                    m.logger.Error("error publishing empty block", "error", err)
                }
            case <-blockTimer.C:
                // Normal block production with available transactions
                if err := m.publishBlock(ctx); err != nil {
                    m.logger.Error("error publishing block", "error", err)
                }
            }
        }
    }

    func (m *Manager) publishEmptyBlock(ctx context.Context) error {
        return m.publishBlock(ctx, EmptyBlockSentinel)
    }
    ```

3. **Transaction Processing**:

    ```go
    func (m *Manager) executeTxs(txs [][]byte) ([][]byte, error) {
        if m.isEmptyBlock(txs) {
            // Skip execution but increment height
            return nil, nil
        }
        // Normal transaction execution
        return m.app.ExecuteTxs(txs)
    }
    ```

### Efficiency Considerations

- Minimal DA layer space usage for empty blocks (3 bytes)
- No additional computational overhead in normal operation
- Maintains existing timing characteristics

### Security Considerations

- Sentinel value must be unique and unlikely to appear in normal transactions
- Empty block detection must be consistent across all nodes
- No additional attack surface introduced

## Status

Proposed

## Consequences

### Positive

- Maintains consistent block heights between DA and execution layers
- Optimizes DA layer space usage for empty blocks
- Preserves existing lazy aggregation benefits
- Supports EVM execution layer requirements

### Negative

- Small additional DA layer cost for empty blocks
- Minor increase in implementation complexity

### Neutral

- Requires coordination between nodes to recognize sentinel values
- May require updates to block explorers and tooling

## References

- [Current Aggregation Implementation](block/aggregation.go)
- [ADR-013: Single Sequencer](specs/lazy-adr/adr-013-single-sequencer.md)
