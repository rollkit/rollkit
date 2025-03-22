# ADR 017: Node Pruning

## Changelog

- 2024-05-07: Initial draft

## Context

Rollkit currently stores all historical blocks, headers, and state data in the node's local storage without any mechanism to prune old data. As rollup chains grow, this leads to increasing storage requirements that can become challenging to manage for node operators, especially for long-running chains.

The current storage implementation in Rollkit uses a key-value store (BadgerDB) to persist blockchain data. The `DefaultStore` implementation in the `store` package maintains blocks, headers, signatures, and state with no built-in mechanism for removing old data that may no longer be needed for node operation.

For rollup chains that have been operational for a significant period or have high transaction throughput, the storage requirements can grow substantially, leading to:

1. Increased disk space requirements for node operators
2. Longer startup times as the node loads and processes more data
3. Potential performance degradation as the database size grows
4. Higher resource requirements for deploying new nodes

A pruning mechanism would allow node operators to reduce storage requirements while maintaining the ability to serve recent chain data and participate in consensus.

## Alternative Approaches

### 1. Full Historical Node / Archive Node (Status Quo)

Continue with the current approach where nodes store the entire history of the blockchain. This approach ensures that any historical data can be queried but requires ever-increasing storage capacity.

### 2. Pruning with Height-Based Retention

Implement a pruning mechanism that retains blocks and associated data only up to a certain number of blocks from the current height. This is the most common approach in blockchain systems and balances storage efficiency with access to recent history.

### 3. Pruning with State Snapshots

Create periodic state snapshots and prune all block data except for the snapshots. This approach dramatically reduces storage requirements but limits the ability to query historical transactions or state transitions.

### 4. Configurable Pruning Strategies (Chosen)

Implement a flexible pruning system that allows node operators to choose from different pruning strategies based on their needs:
   - **None** (default): No pruning, keep all data (archive mode)
   - **Default**: Keep recent blocks (configurable, e.g., last 100 blocks) and periodic snapshots
   - **Custom**: Allow specifying exact retention policies for blocks, snapshots, and other data

## Decision

We will implement a configurable pruning system that allows node operators to choose from different pruning strategies. This approach provides flexibility while addressing the storage concerns for long-running rollup chains.

The pruning system will be implemented as an extension to the existing `DefaultStore` in the `store` package, with configuration options available through the node configuration.

## Detailed Design

### Pruning Configuration

The pruning configuration will be added to the node's configuration file with the following options:

```go
type PruningConfig struct {
    // Strategy determines the pruning approach
    // Options: "none", "default", "custom"
    Strategy string `mapstructure:"strategy"`

    // KeepRecent specifies the number of recent blocks to keep (used in "default" and "custom" strategies)
    KeepRecent uint64 `mapstructure:"keep_recent"`

    // KeepEvery specifies the periodic blocks to keep for historical reference (used in "custom" strategy)
    // For example, if set to 100, every 100th block will be kept
    KeepEvery uint64 `mapstructure:"keep_every"`

    // SnapshotInterval specifies how often to create state snapshots (used in "default" and "custom" strategies)
    // For example, if set to 1000, a snapshot will be created every 1000 blocks
    SnapshotInterval uint64 `mapstructure:"snapshot_interval"`

    // PruneInterval specifies how often (in blocks) the pruning process should run
    PruneInterval uint64 `mapstructure:"prune_interval"`
}
```

These configuration options will be accessible under the `pruning` namespace in the node configuration.

### Store Interface Extension

The `Store` interface in `store/types.go` will be extended to include pruning functionality:

```go
type Store interface {
    // ... existing methods ...

    // PruneBlocks removes block data up to the specified height based on the pruning strategy
    PruneBlocks(ctx context.Context, height uint64) error

    // SetPruningStrategy sets the pruning strategy for the store
    SetPruningStrategy(strategy PruningStrategy) error

    // GetPruningStrategy returns the current pruning strategy
    GetPruningStrategy() PruningStrategy
}
```

### Pruning Implementation

The pruning system will be implemented in the `DefaultStore` struct with the following components:

1. **Pruning Manager**: A component responsible for tracking the current pruning state and executing pruning operations according to the strategy

2. **Strategy Implementation**: Code to implement the different pruning strategies:
   - `none`: No pruning actions performed
   - `default`: Keep recent blocks and periodic snapshots
   - `custom`: Apply custom retention policies

3. **Pruning Trigger**: Logic to trigger pruning based on the configured interval, activated when a new block is added to the chain

4. **Block Removal**: Methods to safely remove block data without corrupting the database

The pruning process will run asynchronously to avoid impacting chain operations.

### State Snapshots

For strategies that use state snapshots, the store will:

1. Create a snapshot of the current state at the configured interval
2. Store the snapshot with a reference to the block height
3. Maintain an index of available snapshots

These snapshots will be used when pruning blocks to ensure that the state can be reconstructed if needed.

### Safety Mechanisms

To ensure system stability and prevent data corruption, the pruning system will include:

1. **Minimum Height Protection**: Never prune below a minimum height (e.g., current height - 100) regardless of the strategy
2. **Transaction Checkpointing**: Ensure that database transactions are used for atomic operations
3. **Error Recovery**: If pruning fails, roll back the operation and log the error without affecting normal operation
4. **Lock Mechanism**: Prevent pruning during critical operations like state synchronization
5. **Validation**: Validate the database consistency after pruning operations

### Pruning Process Flow

The pruning process will follow this general flow:

1. When a new block is added to the chain, check if it's time to run pruning based on the `PruneInterval`
2. If pruning is due, determine which blocks can be pruned based on the strategy
3. Create a snapshot if needed according to the `SnapshotInterval`
4. Lock the pruning manager to prevent concurrent pruning operations
5. Begin a database transaction
6. Remove the selected blocks and associated data
7. Commit the transaction
8. Unlock the pruning manager
9. Update pruning state metadata

### Client API Impact

The node's API will be extended to include endpoints for:

1. Querying the current pruning status
2. Manually triggering pruning (for admin use)
3. Updating pruning configuration

These endpoints will be protected by appropriate authentication to prevent misuse.

### Migration Path

For existing nodes with unpruned data, the pruning system will:

1. Detect the current state of the database
2. Apply the configured pruning strategy only to blocks that meet the criteria
3. Log the number of blocks pruned and storage reclaimed

## Status

Proposed

## Consequences

### Positive

1. **Reduced Storage Requirements**: Nodes will require less disk space, especially for long-running chains
2. **Improved Performance**: Database operations may become faster with a smaller dataset
3. **Lower Barrier to Entry**: New node operators can join with less storage overhead
4. **Flexibility**: Operators can choose a pruning strategy that fits their specific needs
5. **Better Resource Utilization**: More efficient use of available storage resources

### Negative

1. **Limited Historical Queries**: Depending on the pruning strategy, historical data may not be available for queries
2. **Implementation Complexity**: Adding pruning adds complexity to the storage layer
3. **Potential Data Loss**: If configured incorrectly, important data could be pruned
4. **Migration Challenges**: Existing nodes with unpruned data may face challenges during migration

### Neutral

1. **Configuration Requirements**: Node operators will need to understand and configure pruning options
2. **Network Diversity**: The network may consist of both archive nodes and pruned nodes
3. **Monitoring Needs**: Operators will need to monitor pruning operations and storage usage

## References

- [Rollkit Store Implementation](https://github.com/rollkit/rollkit/blob/main/store/store.go)
- [DefaultStore Implementation](https://github.com/rollkit/rollkit/blob/main/store/store.go)
- [Block Manager](https://github.com/rollkit/rollkit/blob/main/block/manager.go)
- [Store Interface](https://github.com/rollkit/rollkit/blob/main/store/types.go)
- [Issue #2093: Node pruning implementation](https://github.com/rollkit/rollkit/issues/2093) 