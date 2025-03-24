# ADR 017: Node Pruning

## Changelog

- 2025-03-22: Initial draft

## Context

Rollkit currently stores all historical blocks, headers, and state data in the node's local storage without any mechanism to prune old data. As rollup chains grow, this leads to increasing storage requirements that can become challenging to manage for node operators, especially for long-running chains.

The current storage implementation in Rollkit uses a key-value store (BadgerDB) to persist blockchain data. The `DefaultStore` implementation in the `store` package maintains blocks, headers, signatures, and state with no built-in mechanism for removing old data that may no longer be needed for node operation.

It's worth noting that in the header and data sync service, the store is primarily used for P2P gossiping purposes. Since the retrieved headers and data are already cached in `headerCache` and `dataCache` respectively, the contents in the store can be safely deleted once they have been retrieved and processed.

For rollup chains that have been operational for a significant period or have high transaction throughput, the storage requirements can grow substantially, leading to:

1. Increased disk space requirements for node operators
2. Longer startup times as the node loads and processes more data
3. Potential performance degradation as the database size grows
4. Higher resource requirements for deploying new nodes

A pruning mechanism would allow node operators to reduce storage requirements while maintaining the ability to serve recent chain data and participate in consensus. Additionally, providing support for attaching external volumes for specific block height ranges would allow for more flexible storage management over extended periods of time.

## Alternative Approaches

### 1. Full Historical Node / Archive Node (Status Quo)

Continue with the current approach where nodes store the entire history of the blockchain. This approach ensures that any historical data can be queried but requires ever-increasing storage capacity.

### 2. Pruning with Height-Based Retention

Implement a pruning mechanism that retains blocks and associated data only up to a certain number of blocks from the current height. This is the most common approach in blockchain systems and balances storage efficiency with access to recent history.

### 3. Pruning with State Snapshots

Create periodic state snapshots and prune all block data except for the snapshots. This approach dramatically reduces storage requirements but limits the ability to query historical transactions or state transitions.

### 4. External Volume Storage

Store historical blocks in separate external volumes that can be mounted and unmounted as needed. This allows for flexible storage management but adds complexity to the implementation and operation.

### 5. Configurable Pruning Strategies with Volume Support (Chosen)

Implement a flexible pruning system that allows node operators to choose from different pruning strategies based on their needs, with added support for external volume management:
   - **None** (default): No pruning, keep all data (archive mode)
   - **Default**: Keep recent blocks (configurable, e.g., last 100 blocks) and periodic snapshots
   - **Custom**: Allow specifying exact retention policies for blocks, snapshots, and other data
   - **Volume-Based**: Allow specifying block height ranges to be stored in separate volumes

## Decision

We will implement a configurable pruning system that allows node operators to choose from different pruning strategies, with added support for external volume storage for specific block height ranges. This approach provides flexibility while addressing the storage concerns for long-running rollup chains and enables efficient long-term data management.

The pruning system will be implemented as an extension to the existing `DefaultStore` in the `store` package, with configuration options available through the node configuration.

## Detailed Design

### Pruning Configuration

The pruning configuration will be added to the node's configuration file with the following options:

```go
type PruningConfig struct {
    // Strategy determines the pruning approach
    // Options: "none", "default", "custom", "volume-based"
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

    // VolumeConfig specifies configuration for volume-based storage
    VolumeConfig *VolumeStorageConfig `mapstructure:"volume_config"`
}

// VolumeStorageConfig defines how block data is distributed across different volumes
type VolumeStorageConfig struct {
    // Enabled indicates whether volume-based storage is enabled
    Enabled bool `mapstructure:"enabled"`

    // MainVolumePath is the path to the main storage volume (for recent blocks)
    MainVolumePath string `mapstructure:"main_volume_path"`

    // ArchiveVolumes is a list of archive volumes for specific block height ranges
    ArchiveVolumes []ArchiveVolume `mapstructure:"archive_volumes"`

    // MaxBlocksPerVolume is the maximum number of blocks to store in a single volume
    // Used for auto-splitting when creating new volumes
    MaxBlocksPerVolume uint64 `mapstructure:"max_blocks_per_volume"`

    // AutoCreateVolumes indicates whether new volumes should be created automatically
    // when a volume reaches MaxBlocksPerVolume
    AutoCreateVolumes bool `mapstructure:"auto_create_volumes"`

    // VolumesDirectory is the base directory where new volumes will be created
    // Used only when AutoCreateVolumes is true
    VolumesDirectory string `mapstructure:"volumes_directory"`
}

// ArchiveVolume defines a single archive volume for a specific block height range
type ArchiveVolume struct {
    // Path is the filesystem path to the volume
    Path string `mapstructure:"path"`

    // StartHeight is the inclusive starting block height for this volume
    StartHeight uint64 `mapstructure:"start_height"`

    // EndHeight is the inclusive ending block height for this volume
    // If 0, the volume extends indefinitely
    EndHeight uint64 `mapstructure:"end_height"`

    // ReadOnly indicates whether this volume is read-only
    ReadOnly bool `mapstructure:"read_only"`
}
```

These configuration options will be accessible under the `pruning` namespace in the node configuration.

### Store Interface Extension

The `Store` interface in `store/types.go` will be extended to include pruning and volume management functionality:

```go
type Store interface {
    // ... existing methods ...

    // PruneBlocks removes block data up to the specified height based on the pruning strategy
    PruneBlocks(ctx context.Context, height uint64) error

    // SetPruningStrategy sets the pruning strategy for the store
    SetPruningStrategy(strategy PruningStrategy) error

    // GetPruningStrategy returns the current pruning strategy
    GetPruningStrategy() PruningStrategy

    // AddVolume adds a new volume for a specific block height range
    AddVolume(ctx context.Context, volume ArchiveVolume) error

    // RemoveVolume removes a volume from the store configuration
    RemoveVolume(ctx context.Context, volumePath string) error

    // ListVolumes returns a list of all configured volumes
    ListVolumes(ctx context.Context) ([]ArchiveVolume, error)

    // MoveBlocksToVolume moves blocks within specified height range to a target volume
    MoveBlocksToVolume(ctx context.Context, startHeight, endHeight uint64, targetVolume string) error
}
```

### Multi-Volume Store Implementation

To support external volumes, we will implement a `MultiVolumeStore` that wraps the `DefaultStore`:

```go
type MultiVolumeStore struct {
    // The main store (for recent blocks)
    mainStore *DefaultStore

    // Archive stores mapped by path
    archiveStores map[string]*DefaultStore

    // Volume configuration
    volumeConfig *VolumeStorageConfig

    // Maps block heights to volume paths for quick lookups
    heightToVolume map[uint64]string

    // Mutex for concurrent access
    mu sync.RWMutex
}
```

This implementation will:

1. Route read requests to the appropriate volume based on the requested block height
2. Handle write operations to the main volume
3. Manage opening and closing volumes as needed
4. Implement background processes for moving data between volumes based on the configuration

### Volume Management

The volume management system will include:

1. **Volume Registry**: A component that tracks which block heights are stored in which volumes
2. **Volume Router**: Logic to direct read/write operations to the appropriate volume
3. **Volume Migration**: Tools to move blocks between volumes
4. **Auto-Volume Creation**: Optionally create new volumes when existing ones reach capacity

#### Volume Mounting and Unmounting

The store will support dynamically mounting and unmounting volumes:

```go
// MountVolume mounts a volume for read/write operations
func (s *MultiVolumeStore) MountVolume(ctx context.Context, volume ArchiveVolume) error {
    // Implementation details
}

// UnmountVolume safely unmounts a volume
func (s *MultiVolumeStore) UnmountVolume(ctx context.Context, volumePath string, force bool) error {
    // Implementation details
}
```

This will allow for efficient resource usage by only keeping actively needed volumes mounted.

### Block Lookup and Retrieval

The block lookup process will be updated to check multiple volumes:

1. First, check if the requested block is in the main volume
2. If not, consult the volume registry to determine which archive volume contains the block
3. If the volume is mounted, retrieve the block from there
4. If the volume is not mounted, automatically mount it, retrieve the block, and optionally unmount it after use

### Pruning Implementation With Volume Support

The pruning system will be enhanced to work with the multi-volume setup:

1. **Volume-Aware Pruning**: The pruning process will be aware of volumes and their configuration
2. **Block Migration**: Instead of deletion, blocks can be migrated to archive volumes
3. **Volume Rotation**: Full volumes can be archived and replaced with new volumes
4. **Volume Consolidation**: Multiple sparse volumes can be consolidated into a single volume

The pruning manager will implement these strategies based on the configuration.

### Data Organization Within Volumes

Each volume will have a standardized internal structure:

```
/volume_root
    /blocks          # Block data organized by height
    /headers         # Block headers
    /signatures      # Block signatures
    /state           # State snapshots
    /indexes         # Index files for quick lookups
    /metadata.json   # Volume metadata (height range, creation time, etc.)
```

This structure ensures consistency across volumes and simplifies migration operations.

### Volume-Based Pruning Strategy

A new "volume-based" pruning strategy will be implemented with the following behavior:

1. Keep recent blocks (configurable number) in the main volume
2. Automatically migrate older blocks to archive volumes based on the configuration
3. Create new archive volumes when existing ones reach capacity
4. Optionally compress older volumes to save space

This strategy provides a balance between accessibility and storage efficiency.

### API Extensions for Volume Management

The node's API will be extended to include endpoints for volume management:

1. List all configured volumes and their status
2. Add/remove volumes
3. Mount/unmount volumes
4. Move blocks between volumes
5. Query volume statistics (usage, block range, etc.)

These endpoints will enable operators to manage storage resources effectively.

### Performance Considerations

To maintain performance with the multi-volume setup:

1. **Caching**: Frequently accessed blocks will be cached in memory regardless of their source volume
2. **Prefetching**: When retrieving blocks from archive volumes, nearby blocks will be prefetched
3. **Background Processing**: Data migration between volumes will happen in the background to minimize impact on node performance
4. **Index Optimization**: Each volume will maintain optimized indexes for fast lookups

### Migration Path

For existing nodes transitioning to the multi-volume setup:

1. An upgrade procedure will create the initial volume configuration
2. Existing block data will remain in the main volume
3. A migration tool will be provided to redistribute blocks according to the new configuration
4. The migration can be performed online with minimal disruption

## Status

Proposed

## Consequences

### Positive

1. **Reduced Storage Requirements**: Nodes will require less disk space, especially for long-running chains
2. **Improved Performance**: Database operations may become faster with a smaller dataset
3. **Lower Barrier to Entry**: New node operators can join with less storage overhead
4. **Flexibility**: Operators can choose a pruning strategy that fits their specific needs
5. **Better Resource Utilization**: More efficient use of available storage resources
6. **Scalable Storage**: External volumes can be added as needed for growing chains
7. **Cost Optimization**: Less frequently accessed data can be stored on cheaper storage tiers
8. **Historical Data Preservation**: Important historical blocks can be preserved in dedicated volumes

### Negative

1. **Limited Historical Queries**: Depending on the pruning strategy, historical data may not be available for queries
2. **Implementation Complexity**: Adding pruning and volume management adds complexity to the storage layer
3. **Potential Data Loss**: If configured incorrectly, important data could be pruned
4. **Migration Challenges**: Existing nodes with unpruned data may face challenges during migration
5. **Operational Overhead**: Managing multiple volumes requires additional operational knowledge
6. **Latency for Archive Data**: Accessing data from external or cold-storage volumes may introduce latency

### Neutral

1. **Configuration Requirements**: Node operators will need to understand and configure pruning options
2. **Network Diversity**: The network may consist of both archive nodes and pruned nodes
3. **Monitoring Needs**: Operators will need to monitor pruning operations and storage usage
4. **Infrastructure Dependencies**: Volume-based storage depends on the underlying infrastructure capabilities

## References

- [Rollkit Store Implementation](https://github.com/rollkit/rollkit/blob/main/store/store.go)
- [Block Manager](https://github.com/rollkit/rollkit/blob/main/block/manager.go)
- [Store Interface](https://github.com/rollkit/rollkit/blob/main/store/types.go)