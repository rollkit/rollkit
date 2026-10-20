# Config

This document provides a comprehensive reference for all configuration options available in Evolve. Understanding these configurations will help you tailor Evolve's behavior to your specific needs, whether you're running an aggregator, a full node, or a light client.

## Table of Contents

- [DA-Only Sync Mode](#da-only-sync-mode)
- [Introduction to Configurations](#configs)
- [Base Configuration](#base-configuration)
  - [Root Directory](#root-directory)
  - [Database Path](#database-path)
  - [Chain ID](#chain-id)
- [Node Configuration (`node`)](#node-configuration-node)
  - [Aggregator Mode](#aggregator-mode)
  - [Based Sequencer Mode](#based-sequencer-mode)
  - [Light Client Mode](#light-client-mode)
  - [Block Time](#block-time)
  - [Maximum Pending Headers and Data](#maximum-pending-headers-and-data)
  - [Lazy Mode (Lazy Aggregator)](#lazy-mode-lazy-aggregator)
  - [Lazy Block Interval](#lazy-block-interval)
  - [Scrape Interval](#scrape-interval)
  - [Catchup Timeout](#catchup-timeout)
  - [Readiness Window Seconds](#readiness-window-seconds)
  - [Readiness Max Blocks Behind](#readiness-max-blocks-behind)
- [Pruning Configuration (`pruning`)](#pruning-configuration-pruning)
- [Data Availability Configuration (`da`)](#data-availability-configuration-da)
  - [DA Service Address](#da-service-address)
  - [DA Authentication Token](#da-authentication-token)
  - [DA Submit Options](#da-submit-options)
  - [DA Signing Addresses](#da-signing-addresses)
  - [DA Max Submit Attempts](#da-max-submit-attempts)
  - [DA Namespace](#da-namespace)
  - [DA Data Namespace](#da-data-namespace)
  - [DA Forced Inclusion Namespace](#da-forced-inclusion-namespace)
  - [DA Block Time](#da-block-time)
  - [DA Mempool TTL](#da-mempool-ttl)
  - [DA Request Timeout](#da-request-timeout)
  - [DA Batching Strategy](#da-batching-strategy)
  - [DA Batch Size Threshold](#da-batch-size-threshold)
  - [DA Batch Max Delay](#da-batch-max-delay)
  - [DA Batch Min Items](#da-batch-min-items)
  - [DA Tx Signer](#da-tx-signer)
- [P2P Configuration (`p2p`)](#p2p-configuration-p2p)
  - [P2P Listen Address](#p2p-listen-address)
  - [P2P Peers](#p2p-peers)
  - [P2P Blocked Peers](#p2p-blocked-peers)
  - [P2P Allowed Peers](#p2p-allowed-peers)
- [RPC Configuration (`rpc`)](#rpc-configuration-rpc)
  - [RPC Server Address](#rpc-server-address)
  - [Enable DA Visualization](#enable-da-visualization)
  - [Health Endpoints](#health-endpoints)
- [Instrumentation Configuration (`instrumentation`)](#instrumentation-configuration-instrumentation)
  - [Enable Prometheus Metrics](#enable-prometheus-metrics)
  - [Prometheus Listen Address](#prometheus-listen-address)
  - [Maximum Open Connections](#maximum-open-connections)
  - [Enable Pprof Profiling](#enable-pprof-profiling)
  - [Pprof Listen Address](#pprof-listen-address)
  - [Enable Tracing](#enable-tracing)
  - [Tracing Endpoint](#tracing-endpoint)
  - [Tracing Service Name](#tracing-service-name)
  - [Tracing Sample Rate](#tracing-sample-rate)
- [Logging Configuration (`log`)](#logging-configuration-log)
  - [Log Level](#log-level)
  - [Log Format](#log-format)
  - [Log Trace (Stack Traces)](#log-trace-stack-traces)
- [Signer Configuration (`signer`)](#signer-configuration-signer)
  - [Signer Type](#signer-type)
  - [Signer Path](#signer-path)
  - [Signer Passphrase File](#signer-passphrase-file)
- [Raft Configuration (`raft`)](#raft-configuration-raft)
  - [Enable Raft](#enable-raft)
  - [Raft Node ID](#raft-node-id)
  - [Raft Address](#raft-address)
  - [Raft Directory](#raft-directory)
  - [Raft Bootstrap](#raft-bootstrap)
  - [Raft Peers](#raft-peers)
  - [Raft Snap Count](#raft-snap-count)
  - [Raft Send Timeout](#raft-send-timeout)
  - [Raft Heartbeat Timeout](#raft-heartbeat-timeout)
  - [Raft Leader Lease Timeout](#raft-leader-lease-timeout)

## DA-Only Sync Mode

Evolve supports running nodes that sync exclusively from the Data Availability (DA) layer without participating in P2P networking. This mode is useful for:

- **Pure DA followers**: Nodes that only need the canonical chain data from DA
- **Resource optimization**: Reducing network overhead by avoiding P2P gossip
- **Simplified deployment**: No need to configure or maintain P2P peer connections
- **Isolated environments**: Nodes that should not participate in P2P communication

**To enable DA-only sync mode:**

1. **Leave P2P peers empty** (default behavior):

   ```yaml
   p2p:
     peers: "" # Empty or omit this field entirely
   ```

2. **Configure DA connection** (required):

   ```yaml
   da:
     address: "your-da-service:port"
     namespace: "your-namespace"
     # ... other DA configuration
   ```

3. **Optional**: You can still configure P2P listen address for potential future connections, but without peers, no P2P networking will occur.

When running in DA-only mode, the node will:

- ✅ Sync blocks and headers from the DA layer
- ✅ Validate transactions and maintain state
- ✅ Serve RPC requests
- ❌ Not participate in P2P gossip or peer discovery
- ❌ Not share blocks with other nodes via P2P
- ❌ Not receive transactions via P2P (only from direct RPC submission)

## Configs

Evolve configurations can be managed through a YAML file (typically `evnode.yml` located in `~/.evolve/config/` or `<your_home_dir>/config/`) and command-line flags. The system prioritizes configurations in the following order (highest priority first):

1. **Command-line flags:** Override all other settings.
2. **YAML configuration file:** Values specified in the `config.yaml` file.
3. **Default values:** Predefined defaults within Evolve.

Environment variables can also be used, typically prefixed with your executable's name (e.g., `YOURAPP_CHAIN_ID="my-chain"`).

## Base Configuration

These are fundamental settings for your Evolve node.

### Root Directory

**Description:**
The root directory where Evolve stores its data, including the database and configuration files. This is a foundational setting that dictates where all other file paths are resolved from.

**YAML:**
This option is not set within the YAML configuration file itself, as it specifies the location _of_ the configuration file and other application data.

**Command-line Flag:**
`--home <path>`
_Example:_ `--home /mnt/data/evolve_node`
_Default:_ `~/.evolve` (or a directory derived from the application name if `defaultHome` is customized).
_Constant:_ `FlagRootDir`

### Database Path

**Description:**
The path, relative to the Root Directory, where the Evolve database will be stored. This database contains blockchain state, blocks, and other critical node data.

**YAML:**
Set this in your configuration file at the top level:

```yaml
db_path: "data"
```

**Command-line Flag:**
`--evnode.db_path <path>`
_Example:_ `--evnode.db_path "node_db"`
_Default:_ `"data"`
_Constant:_ `FlagDBPath`

### Chain ID

**Description:**
The unique identifier for your chain. This ID is used to differentiate your network from others and is crucial for network communication and transaction validation.

**YAML:**
Set this in your configuration file at the top level:

```yaml
chain_id: "my-evolve-chain"
```

**Command-line Flag:**
`--chain_id <string>`
_Example:_ `--chain_id "super_rollup_testnet_v1"`
_Default:_ `"evolve"`
_Constant:_ `FlagChainID`

## Node Configuration (`node`)

Settings related to the core behavior of the Evolve node, including its mode of operation and block production parameters.

**YAML Section:**

```yaml
node:
  # ... node configurations ...
```

### Aggregator Mode

**Description:**
If true, the node runs in aggregator mode. Aggregators are responsible for producing blocks by collecting transactions, ordering them, and proposing them to the network.

**YAML:**

```yaml
node:
  aggregator: true
```

**Command-line Flag:**
`--evnode.node.aggregator` (boolean, presence enables it)
_Example:_ `--evnode.node.aggregator`
_Default:_ `false`
_Constant:_ `FlagAggregator`

### Based Sequencer Mode

**Description:**
If true, the node runs in based sequencer mode. In this mode the aggregator only processes transactions fetched from the DA forced inclusion namespace rather than from the P2P mempool. Requires aggregator mode to be enabled.

**YAML:**

```yaml
node:
  based_sequencer: true
```

**Command-line Flag:**
`--evnode.node.based_sequencer` (boolean, presence enables it)
_Example:_ `--evnode.node.based_sequencer`
_Default:_ `false`
_Constant:_ `FlagBasedSequencer`

### Light Client Mode

**Description:**
If true, the node runs in light client mode. Light clients rely on full nodes for block headers and state information, offering a lightweight way to interact with the chain without storing all data.

**YAML:**

```yaml
node:
  light: true
```

**Command-line Flag:**
`--evnode.node.light` (boolean, presence enables it)
_Example:_ `--evnode.node.light`
_Default:_ `false`
_Constant:_ `FlagLight`

### Block Time

**Description:**
The target time interval between consecutive blocks produced by an aggregator. This duration (e.g., "500ms", "1s", "5s") dictates the pace of block production.

**YAML:**

```yaml
node:
  block_time: "1s"
```

**Command-line Flag:**
`--evnode.node.block_time <duration>`
_Example:_ `--evnode.node.block_time 2s`
_Default:_ `"1s"`
_Constant:_ `FlagBlockTime`

### Maximum Pending Headers and Data

**Description:**
The maximum number of headers or data items that can be pending Data Availability (DA) confirmation. When this limit is reached, the aggregator pauses block production until some are confirmed on the DA layer. Use 0 for no limit. This helps manage resource usage and DA layer capacity.

**YAML:**

```yaml
node:
  max_pending_headers_and_data: 100
```

**Command-line Flag:**
`--evnode.node.max_pending_headers_and_data <uint64>`
_Example:_ `--evnode.node.max_pending_headers_and_data 50`
_Default:_ `0` (no limit)
_Constant:_ `FlagMaxPendingHeadersAndData`

### Lazy Mode (Lazy Aggregator)

**Description:**
Enables lazy aggregation mode. In this mode, blocks are produced only when new transactions are available in the mempool or after the `lazy_block_interval` has passed. This optimizes resource usage by avoiding the creation of empty blocks during periods of inactivity.

**YAML:**

```yaml
node:
  lazy_mode: true
```

**Command-line Flag:**
`--evnode.node.lazy_mode` (boolean, presence enables it)
_Example:_ `--evnode.node.lazy_mode`
_Default:_ `false`
_Constant:_ `FlagLazyAggregator`

### Lazy Block Interval

**Description:**
The maximum time interval between blocks when running in lazy aggregation mode (`lazy_mode`). This ensures that blocks are produced periodically even if there are no new transactions, keeping the chain active. This value is generally larger than `block_time`.

**YAML:**

```yaml
node:
  lazy_block_interval: "30s"
```

**Command-line Flag:**
`--evnode.node.lazy_block_interval <duration>`
_Example:_ `--evnode.node.lazy_block_interval 2m`
_Default:_ `"1m"`
_Constant:_ `FlagLazyBlockTime`

### Scrape Interval

**Description:**
The interval at which the reaper polls the execution layer for new transactions. Lower values reduce transaction detection latency but increase RPC load on the execution client.

**YAML:**

```yaml
node:
  scrape_interval: "1s"
```

**Command-line Flag:**
`--evnode.node.scrape_interval <duration>`
_Example:_ `--evnode.node.scrape_interval 500ms`
_Default:_ `"1s"`
_Constant:_ `FlagScrapeInterval`

### Catchup Timeout

**Description:**
When set to a non-zero duration, the aggregator recovers before producing blocks. If `p2p.peers` is configured, both header and data sync must initialize from P2P and the node must reach the highest observed P2P height; failure to establish continuity before this timeout is fatal. With no configured peers, recovery waits for the DA head and does not apply a P2P deadline. The default `0` disables recovery catchup. Requires aggregator mode. Mutually exclusive with Raft consensus.

**YAML:**

```yaml
node:
  catchup_timeout: "30s"
```

**Command-line Flag:**
`--evnode.node.catchup_timeout <duration>`
_Example:_ `--evnode.node.catchup_timeout 1m`
_Default:_ `"0s"` (disabled)
_Constant:_ `FlagCatchupTimeout`

### Readiness Window Seconds

**Description:**
The time window in seconds used to calculate how many blocks behind the node can be and still be considered ready. The actual block threshold is derived by dividing this window by the block time. Default is 15 seconds.

**YAML:**

```yaml
node:
  readiness_window_seconds: 15
```

**Command-line Flag:**
`--evnode.node.readiness_window_seconds <uint64>`
_Example:_ `--evnode.node.readiness_window_seconds 30`
_Default:_ `15`
_Constant:_ `FlagReadinessWindowSeconds`

### Readiness Max Blocks Behind

**Description:**
Explicit override for how many blocks behind best-known head the node can be and still be considered ready. When set to 0, the value is calculated automatically from `readiness_window_seconds` and the block time. Override this to set an absolute block count instead of a time-based window.

**YAML:**

```yaml
node:
  readiness_max_blocks_behind: 15
```

**Command-line Flag:**
`--evnode.node.readiness_max_blocks_behind <uint64>`
_Example:_ `--evnode.node.readiness_max_blocks_behind 30`
_Default:_ `0` (calculated from `readiness_window_seconds`)
_Constant:_ `FlagReadinessMaxBlocksBehind`

## Pruning Configuration (`pruning`)

**Description:**
Controls automatic pruning of stored block data and metadata from the local store. Pruning helps manage disk space by periodically removing old blocks and their associated state, while keeping a recent window of history for validation and queries.

**Pruning Modes:**

- **`disabled`** (default): Archive mode - keeps all blocks and metadata indefinitely
- **`metadata`**: Prunes only state metadata (execution state snapshots), keeps all blocks
- **`all`**: Prunes both blocks (headers, data, signatures) and metadata

**How Pruning Works:**

When pruning is enabled, the pruner runs at the configured interval and removes data beyond the retention window (`pruning_keep_recent`). The system uses intelligent batching to avoid overwhelming the node:

- **Batch sizes are automatically calculated** based on your `pruning_interval` and `block_time`
- **Catch-up mode**: When first enabling pruning on an existing node, smaller batches (2× blocks per interval) are used to gradually catch up without impacting performance
- **Normal mode**: Once caught up, larger batches (4× blocks per interval) are used for efficient maintenance
- **Progress tracking**: Pruning progress is saved after each batch, so restarts don't lose progress

**Batch Size Examples:**

With default settings (15 minute interval, 1 second blocks):

- Catch-up: ~1,800 blocks per run
- Normal: ~3,600 blocks per run

With high-throughput chain (15 minute interval, 100ms blocks):

- Catch-up: ~18,000 blocks per run
- Normal: ~36,000 blocks per run

**YAML:**

```yaml
pruning:
  pruning_mode: "all"
  pruning_keep_recent: 100000
  pruning_interval: "15m"
```

**Command-line Flags:**

- `--evnode.pruning.pruning_mode <string>`
  - _Description:_ Pruning mode: 'disabled' (keep all), 'metadata' (prune state only), or 'all' (prune blocks and state)
  - _Example:_ `--evnode.pruning.pruning_mode all`
  - _Default:_ `"disabled"`

- `--evnode.pruning.pruning_keep_recent <uint64>`
  - _Description:_ Number of most recent blocks/metadata to retain when pruning is enabled. Must be > 0 when pruning is enabled.
  - _Example:_ `--evnode.pruning.pruning_keep_recent 100000`
  - _Default:_ `0`

- `--evnode.pruning.pruning_interval <duration>`
  - _Description:_ How often to run the pruning process. Must be >= block_time when pruning is enabled. Larger intervals allow larger batch sizes.
  - _Example:_ `--evnode.pruning.pruning_interval 15m`
  - _Default:_ `0` (disabled)

_Constants:_ `FlagPruningMode`, `FlagPruningKeepRecent`, `FlagPruningInterval`

**Important Notes:**

- When DA is enabled (DA address is configured), pruning only removes blocks that have been confirmed on the DA layer (for mode `all`) to ensure data safety
- When DA is not enabled (no DA address configured), pruning proceeds based solely on store height, allowing nodes without DA to manage disk space
- The first pruning run after enabling may take several cycles to catch up, processing data in smaller batches
- Pruning cannot be undone - ensure your retention window is sufficient for your use case
- For production deployments, consider keeping at least 100,000 recent blocks
- The pruning interval should be balanced with your disk space growth rate

## Data Availability Configuration (`da`)

Parameters for connecting and interacting with the Data Availability (DA) layer, which Evolve uses to publish block data.

**YAML Section:**

```yaml
da:
  # ... DA configurations ...
```

### DA Service Address

**Description:**
The network address (host:port) of the Data Availability layer service. A full follower with this value unset syncs exclusively through P2P and does not create a DA client. Aggregators must configure this value before startup.

**YAML:**

```yaml
da:
  address: "localhost:26659"
```

**Command-line Flag:**
`--evnode.da.address <string>`
_Example:_ `--evnode.da.address 192.168.1.100:26659`
_Default:_ `""` (empty; P2P-only follower)
_Constant:_ `FlagDAAddress`

### DA Authentication Token

**Description:**
The authentication token required to interact with the DA layer service, if the service mandates authentication.

**YAML:**

```yaml
da:
  auth_token: "YOUR_DA_AUTH_TOKEN"
```

**Command-line Flag:**
`--evnode.da.auth_token <string>`
_Example:_ `--evnode.da.auth_token mysecrettoken`
_Default:_ `""` (empty)
_Constant:_ `FlagDAAuthToken`

### DA Submit Options

**Description:**
Additional options passed to the DA layer when submitting data. The format and meaning of these options depend on the specific DA implementation being used. For example, with Celestia, this can include custom gas settings or other submission parameters in JSON format.

**Note:** If you configure multiple signing addresses (see [DA Signing Addresses](#da-signing-addresses)), the selected signing address will be automatically merged into these options as a JSON field `signer_address` (matching Celestia's TxConfig schema). If the base options are already valid JSON, the signing address is added to the existing object; otherwise, a new JSON object is created.

**YAML:**

```yaml
da:
  submit_options: '{"key":"value"}' # Example, format depends on DA layer
```

**Command-line Flag:**
`--evnode.da.submit_options <string>`
_Example:_ `--evnode.da.submit_options '{"custom_param":true}'`
_Default:_ `""` (empty)
_Constant:_ `FlagDASubmitOptions`

### DA Signing Addresses

**Description:**
A comma-separated list of signing addresses to use for DA blob submissions. When multiple addresses are provided, they will be used in round-robin fashion to prevent sequence mismatches that can occur with high-throughput Cosmos SDK-based DA layers. This is particularly useful for Celestia when submitting many transactions concurrently.

Each submission will select the next address in the list, and that address will be automatically added to the `submit_options` as `signer_address`. This ensures that the DA layer (e.g., celestia-node) uses the specified account for signing that particular blob submission.

**Setup Requirements:**

- All addresses must be loaded into the DA node's keyring and have sufficient funds for transaction fees
- For Celestia, see the guide on setting up multiple accounts in the DA node documentation

**YAML:**

```yaml
da:
  signing_addresses:
    - "celestia1abc123..."
    - "celestia1def456..."
    - "celestia1ghi789..."
```

**Command-line Flag:**
`--evnode.da.signing_addresses <string>`
_Example:_ `--evnode.da.signing_addresses celestia1abc...,celestia1def...,celestia1ghi...`
_Default:_ `[]` (empty, uses default DA node behavior)
_Constant:_ `FlagDASigningAddresses`

**Behavior:**

- If no signing addresses are configured, submissions use the DA layer's default signing behavior
- If one address is configured, all submissions use that address
- If multiple addresses are configured, they are used in round-robin order to distribute the load and prevent nonce/sequence conflicts
- The address selection is thread-safe for concurrent submissions

### DA Max Submit Attempts

**Description:**
The maximum number of attempts to submit data to the DA layer before giving up. Higher values provide more resilience against transient DA failures but can delay error reporting.

**YAML:**

```yaml
da:
  max_submit_attempts: 30
```

**Command-line Flag:**
`--evnode.da.max_submit_attempts <int>`
_Example:_ `--evnode.da.max_submit_attempts 10`
_Default:_ `30`
_Constant:_ `FlagDAMaxSubmitAttempts`

### DA Namespace

**Description:**
The namespace ID used when submitting blobs (block data) to the DA layer. This helps segregate data from different chains or applications on a shared DA layer.

**Note:** If only `namespace` is provided, it will be used for both headers and data, otherwise the `data_namespace` will be used for data. Doing so allows speeding up light clients.

**YAML:**

```yaml
da:
  namespace: "MY_UNIQUE_NAMESPACE_ID"
```

**Command-line Flag:**
`--evnode.da.namespace <string>`
_Example:_ `--evnode.da.namespace 0x1234567890abcdef`
_Default:_ randomly generated at startup
_Constant:_ `FlagDANamespace`

### DA Data Namespace

**Description:**
The namespace ID specifically for submitting transaction data to the DA layer. Transaction data is submitted separately from headers, enabling nodes to sync only the data they need. The namespace value is encoded by the node to ensure proper formatting and compatibility with the DA layer.

**YAML:**

```yaml
da:
  data_namespace: "DATA_NAMESPACE_ID"
```

**Command-line Flag:**
`--evnode.da.data_namespace <string>`
_Example:_ `--evnode.da.data_namespace my_data_namespace`
_Default:_ `""` (falls back to `namespace` if not set)
_Constant:_ `FlagDADataNamespace`

### DA Forced Inclusion Namespace

**Description:**
The namespace ID used for forced inclusion transactions on the DA layer. When set, the based sequencer will fetch transactions from this namespace. Required when running in based sequencer mode.

**YAML:**

```yaml
da:
  forced_inclusion_namespace: "FORCED_INCLUSION_NAMESPACE_ID"
```

**Command-line Flag:**
`--evnode.da.forced_inclusion_namespace <string>`
_Example:_ `--evnode.da.forced_inclusion_namespace 0xabcdef1234567890`
_Default:_ `""` (empty)
_Constant:_ `FlagDAForcedInclusionNamespace`

### DA Block Time

**Description:**
The average block time of the Data Availability chain (specified as a duration string, e.g., "15s", "1m"). This value influences:

- The frequency of DA layer syncing.
- The maximum backoff time for retrying DA submissions.
- Calculation of transaction expiration when multiplied by `mempool_ttl`.

**YAML:**

```yaml
da:
  block_time: "6s"
```

**Command-line Flag:**
`--evnode.da.block_time <duration>`
_Example:_ `--evnode.da.block_time 12s`
_Default:_ `"6s"`
_Constant:_ `FlagDABlockTime`

### DA Mempool TTL

**Description:**
The number of DA blocks after which a transaction submitted to the DA layer is considered expired and potentially dropped from the DA layer's mempool. This also controls the retry backoff timing for DA submissions.

**YAML:**

```yaml
da:
  mempool_ttl: 20
```

**Command-line Flag:**
`--evnode.da.mempool_ttl <uint64>`
_Example:_ `--evnode.da.mempool_ttl 30`
_Default:_ `0`
_Constant:_ `FlagDAMempoolTTL`

### DA Request Timeout

**Description:**
Per-request timeout applied to DA `GetIDs` and `Get` RPC calls while retrieving blobs. Increase this value if your DA endpoint has high latency to avoid premature failures; decrease it to make the syncer fail fast and free resources sooner when the DA node becomes unresponsive.

**YAML:**

```yaml
da:
  request_timeout: "30s"
```

**Command-line Flag:**
`--evnode.da.request_timeout <duration>`
_Example:_ `--evnode.da.request_timeout 45s`
_Default:_ `"1m"`
_Constant:_ `FlagDARequestTimeout`

### DA Batching Strategy

**Description:**
Controls how blocks are batched before submission to the DA layer. Different strategies offer trade-offs between latency, cost efficiency, and throughput. All strategies pass through the DA submitter which performs additional size checks and may further split batches that exceed the DA layer's blob size limit.

Available strategies:

- **`immediate`**: Submits as soon as any items are available. Best for low-latency requirements where cost is not a concern.
- **`size`**: Waits until the batch reaches a size threshold (fraction of max blob size). Best for maximizing blob utilization and minimizing costs when latency is flexible.
- **`time`**: Waits for a time interval before submitting. Provides predictable submission timing aligned with DA block times.
- **`adaptive`**: Balances between size and time constraints—submits when either the size threshold is reached OR the max delay expires. Recommended for most production deployments as it optimizes both cost and latency.

**YAML:**

```yaml
da:
  batching_strategy: "time"
```

**Command-line Flag:**
`--evnode.da.batching_strategy <string>`
_Example:_ `--evnode.da.batching_strategy adaptive`
_Default:_ `"time"`
_Constant:_ `FlagDABatchingStrategy`

### DA Batch Size Threshold

**Description:**
The minimum blob size threshold (as a fraction of the maximum blob size, between 0.0 and 1.0) before submitting a batch. Only applies to the `size` and `adaptive` strategies. For example, a value of 0.8 means the batch will be submitted when it reaches 80% of the maximum blob size.

Higher values maximize blob utilization and reduce costs but may increase latency. Lower values reduce latency but may result in less efficient blob usage.

**YAML:**

```yaml
da:
  batch_size_threshold: 0.8
```

**Command-line Flag:**
`--evnode.da.batch_size_threshold <float64>`
_Example:_ `--evnode.da.batch_size_threshold 0.9`
_Default:_ `0.8` (80% of max blob size)
_Constant:_ `FlagDABatchSizeThreshold`

### DA Batch Max Delay

**Description:**
The maximum time to wait before submitting a batch regardless of size. Applies to the `time` and `adaptive` strategies. Lower values reduce latency but may increase costs due to smaller batches. This value is typically aligned with the DA chain's block time to ensure submissions land in consecutive blocks.

When set to 0, defaults to the DA BlockTime value.

**YAML:**

```yaml
da:
  batch_max_delay: "6s"
```

**Command-line Flag:**
`--evnode.da.batch_max_delay <duration>`
_Example:_ `--evnode.da.batch_max_delay 12s`
_Default:_ `0` (uses DA BlockTime)
_Constant:_ `FlagDABatchMaxDelay`

### DA Batch Min Items

**Description:**
The minimum number of items (headers or data) to accumulate before considering submission. This helps avoid submitting single items when more are expected soon, improving batching efficiency. All strategies respect this minimum.

**YAML:**

```yaml
da:
  batch_min_items: 1
```

**Command-line Flag:**
`--evnode.da.batch_min_items <uint64>`
_Example:_ `--evnode.da.batch_min_items 5`
_Default:_ `1`
_Constant:_ `FlagDABatchMinItems`

### DA Tx Signer

**Description:**
Signs PayForBlobs transactions locally and broadcasts them over Celestia consensus gRPC, instead of asking the DA node to sign with its own keyring through `blob.Submit`. Use it with DA endpoints that cannot sign, such as da-proxy. Reads still go through `da.address`. Disabled by default; when disabled, submission is unchanged.

Only binaries built with the `da/celestia` module (`testapp`, `evm`) act on this section; other binaries ignore it.

**YAML:**

```yaml
da:
  tx_signer:
    enabled: true
    grpc_address: "grpc.da-proxy-lb.celestia-mocha.com:443"
    grpc_auth_token: "" # defaults to da.auth_token
    grpc_tls: true
    keyring_dir: "keys" # relative to the node home
    keyring_backend: "file"
    key_name: "evnode"
    passphrase_file: "/run/secrets/da_keyring_passphrase"
```

| Key | Flag | Default | Notes |
|---|---|---|---|
| `enabled` | `--evnode.da.tx_signer.enabled` | `false` | |
| `grpc_address` | `--evnode.da.tx_signer.grpc_address` | `""` | Required when enabled. `host:port`. |
| `grpc_auth_token` | `--evnode.da.tx_signer.grpc_auth_token` | `da.auth_token` | Sent as `x-token` gRPC metadata. One da-proxy token covers both endpoints. |
| `grpc_tls` | `--evnode.da.tx_signer.grpc_tls` | `true` | |
| `keyring_dir` | `--evnode.da.tx_signer.keyring_dir` | `keys` | Relative paths resolve against the node home. |
| `keyring_backend` | `--evnode.da.tx_signer.keyring_backend` | `file` | `file` or `test` (unencrypted, local development only). |
| `key_name` | `--evnode.da.tx_signer.key_name` | `""` | Required. Key that signs and pays for blobs. |
| `passphrase_file` | `--evnode.da.tx_signer.passphrase_file` | `""` | File holding the `file` keyring passphrase. |

**Passphrase:**
The `file` backend needs a passphrase of at least 8 characters, read from the `EVNODE_DA_KEYRING_PASSPHRASE` environment variable or, if unset, from `passphrase_file`. Never put the passphrase itself in YAML or flags.

**Validation:**

- `grpc_address`, `key_name` and `da.address` are required when enabled.
- `da.signing_addresses` must be empty when enabled.

At startup the node logs the signer address and balance, and fails if the key is not in the keyring.

## P2P Configuration (`p2p`)

Settings for peer-to-peer networking, enabling nodes to discover each other, exchange blocks, and share transactions.

**YAML Section:**

```yaml
p2p:
  # ... P2P configurations ...
```

### P2P Listen Address

**Description:**
The network address (host:port) on which the Evolve node will listen for incoming P2P connections from other nodes.

**YAML:**

```yaml
p2p:
  listen_address: "0.0.0.0:7676"
```

**Command-line Flag:**
`--evnode.p2p.listen_address <string>`
_Example:_ `--evnode.p2p.listen_address /ip4/127.0.0.1/tcp/26656`
_Default:_ `"/ip4/0.0.0.0/tcp/7676"`
_Constant:_ `FlagP2PListenAddress`

### P2P Peers

**Description:**
A comma-separated list of peer addresses (e.g., multiaddresses) that the node will attempt to connect to for bootstrapping its P2P connections. These are often referred to as seed nodes.

**For DA-only sync mode:** Leave this field empty (default) to disable P2P networking entirely. When no peers are configured, the node will sync exclusively from the Data Availability layer without participating in P2P gossip, peer discovery, or block sharing. This is useful for nodes that only need to follow the canonical chain data from DA.

**YAML:**

```yaml
p2p:
  peers: "/ip4/some_peer_ip/tcp/7676/p2p/PEER_ID1,/ip4/another_peer_ip/tcp/7676/p2p/PEER_ID2"
  # For DA-only sync, leave peers empty:
  # peers: ""
```

**Command-line Flag:**
`--evnode.p2p.peers <string>`
_Example:_ `--evnode.p2p.peers /dns4/seed.example.com/tcp/26656/p2p/12D3KooW...`
_Default:_ `""` (empty - enables DA-only sync mode)
_Constant:_ `FlagP2PPeers`

### P2P Blocked Peers

**Description:**
A comma-separated list of peer IDs that the node should block from connecting. This can be used to prevent connections from known malicious or problematic peers.

**YAML:**

```yaml
p2p:
  blocked_peers: "PEER_ID_TO_BLOCK1,PEER_ID_TO_BLOCK2"
```

**Command-line Flag:**
`--evnode.p2p.blocked_peers <string>`
_Example:_ `--evnode.p2p.blocked_peers 12D3KooW...,12D3KooX...`
_Default:_ `""` (empty)
_Constant:_ `FlagP2PBlockedPeers`

### P2P Allowed Peers

**Description:**
A comma-separated list of peer IDs that the node should exclusively allow connections from. If this list is non-empty, only peers in this list will be able to connect.

**YAML:**

```yaml
p2p:
  allowed_peers: "PEER_ID_TO_ALLOW1,PEER_ID_TO_ALLOW2"
```

**Command-line Flag:**
`--evnode.p2p.allowed_peers <string>`
_Example:_ `--evnode.p2p.allowed_peers 12D3KooY...,12D3KooZ...`
_Default:_ `""` (empty, allow all unless blocked)
_Constant:_ `FlagP2PAllowedPeers`

## RPC Configuration (`rpc`)

Settings for the Remote Procedure Call (RPC) server, which allows clients and applications to interact with the Evolve node.

**YAML Section:**

```yaml
rpc:
  # ... RPC configurations ...
```

### RPC Server Address

**Description:**
The network address (host:port) to which the RPC server will bind and listen for incoming requests.

**YAML:**

```yaml
rpc:
  address: "127.0.0.1:7331"
```

**Command-line Flag:**
`--evnode.rpc.address <string>`
_Example:_ `--evnode.rpc.address 0.0.0.0:26657`
_Default:_ `"127.0.0.1:7331"`
_Constant:_ `FlagRPCAddress`

### Enable DA Visualization

**Description:**
If true, enables the Data Availability (DA) visualization endpoints that provide real-time monitoring of blob submissions to the DA layer. This includes a web-based dashboard and REST API endpoints for tracking submission statistics, monitoring DA health, and analyzing blob details. Only aggregator nodes submit data to the DA layer, so this feature is most useful when running in aggregator mode.

**YAML:**

```yaml
rpc:
  enable_da_visualization: true
```

**Command-line Flag:**
`--evnode.rpc.enable_da_visualization` (boolean, presence enables it)
_Example:_ `--evnode.rpc.enable_da_visualization`
_Default:_ `false`
_Constant:_ `FlagRPCEnableDAVisualization`

See the [DA Visualizer Guide](../guides/da/visualizer.md) for detailed information on using this feature.

### Health Endpoints

#### `/health/live`

Returns `200 OK` if the process is alive and can access the store.

```bash
curl http://localhost:7331/health/live
```

#### `/health/ready`

Returns `200 OK` if the node can serve correct data. Checks:

- P2P is listening (if enabled)
- Has synced blocks
- Not too far behind network
- Non-aggregators: has peers
- Aggregators: producing blocks at expected rate

```bash
curl http://localhost:7331/health/ready
```

Configure these via [`readiness_window_seconds`](#readiness-window-seconds) and [`readiness_max_blocks_behind`](#readiness-max-blocks-behind) in the node configuration.

## Instrumentation Configuration (`instrumentation`)

Settings for enabling and configuring metrics and profiling endpoints, useful for monitoring node performance and debugging.

**YAML Section:**

```yaml
instrumentation:
  # ... instrumentation configurations ...
```

### Enable Prometheus Metrics

**Description:**
If true, enables the Prometheus metrics endpoint, allowing Prometheus to scrape operational data from the Evolve node.

**YAML:**

```yaml
instrumentation:
  prometheus: true
```

**Command-line Flag:**
`--evnode.instrumentation.prometheus` (boolean, presence enables it)
_Example:_ `--evnode.instrumentation.prometheus`
_Default:_ `false`
_Constant:_ `FlagPrometheus`

### Prometheus Listen Address

**Description:**
The network address (host:port) where the Prometheus metrics server will listen for scraping requests.

See [Metrics](../guides/metrics.md) for more details on what metrics are exposed.

**YAML:**

```yaml
instrumentation:
  prometheus_listen_addr: ":2112"
```

**Command-line Flag:**
`--evnode.instrumentation.prometheus_listen_addr <string>`
_Example:_ `--evnode.instrumentation.prometheus_listen_addr 0.0.0.0:9090`
_Default:_ `":26660"`
_Constant:_ `FlagPrometheusListenAddr`

### Maximum Open Connections

**Description:**
The maximum number of simultaneous connections allowed for the metrics server (e.g., Prometheus endpoint).

**YAML:**

```yaml
instrumentation:
  max_open_connections: 100
```

**Command-line Flag:**
`--evnode.instrumentation.max_open_connections <int>`
_Example:_ `--evnode.instrumentation.max_open_connections 50`
_Default:_ `3`
_Constant:_ `FlagMaxOpenConnections`

### Enable Pprof Profiling

**Description:**
If true, enables the pprof HTTP endpoint, which provides runtime profiling data for debugging performance issues. Accessing these endpoints can help diagnose CPU and memory usage.

**YAML:**

```yaml
instrumentation:
  pprof: true
```

**Command-line Flag:**
`--evnode.instrumentation.pprof` (boolean, presence enables it)
_Example:_ `--evnode.instrumentation.pprof`
_Default:_ `false`
_Constant:_ `FlagPprof`

### Pprof Listen Address

**Description:**
The network address (host:port) where the pprof HTTP server will listen for profiling requests.

**YAML:**

```yaml
instrumentation:
  pprof_listen_addr: "localhost:6060"
```

**Command-line Flag:**
`--evnode.instrumentation.pprof_listen_addr <string>`
_Example:_ `--evnode.instrumentation.pprof_listen_addr 0.0.0.0:6061`
_Default:_ `":6060"`
_Constant:_ `FlagPprofListenAddr`

### Enable Tracing

**Description:**
If true, enables OpenTelemetry tracing. Traces are exported via OTLP to the configured endpoint.

**YAML:**

```yaml
instrumentation:
  tracing: true
```

**Command-line Flag:**
`--evnode.instrumentation.tracing` (boolean, presence enables it)
_Example:_ `--evnode.instrumentation.tracing`
_Default:_ `false`
_Constant:_ `FlagTracing`

### Tracing Endpoint

**Description:**
The OTLP endpoint (host:port) to which traces are exported. Must be set when tracing is enabled.

**YAML:**

```yaml
instrumentation:
  tracing_endpoint: "localhost:4318"
```

**Command-line Flag:**
`--evnode.instrumentation.tracing_endpoint <string>`
_Example:_ `--evnode.instrumentation.tracing_endpoint otel-collector:4318`
_Default:_ `"localhost:4318"`
_Constant:_ `FlagTracingEndpoint`

### Tracing Service Name

**Description:**
The `service.name` resource attribute attached to all traces exported by this node. Use this to identify the node in your tracing backend.

**YAML:**

```yaml
instrumentation:
  tracing_service_name: "ev-node"
```

**Command-line Flag:**
`--evnode.instrumentation.tracing_service_name <string>`
_Example:_ `--evnode.instrumentation.tracing_service_name my-rollup-node`
_Default:_ `"ev-node"`
_Constant:_ `FlagTracingServiceName`

### Tracing Sample Rate

**Description:**
The TraceID ratio-based sampling rate for traces, between 0.0 and 1.0. A value of 1.0 samples all traces; 0.1 samples 10%.

**YAML:**

```yaml
instrumentation:
  tracing_sample_rate: 0.1
```

**Command-line Flag:**
`--evnode.instrumentation.tracing_sample_rate <float64>`
_Example:_ `--evnode.instrumentation.tracing_sample_rate 0.5`
_Default:_ `0.1`
_Constant:_ `FlagTracingSampleRate`

## Logging Configuration (`log`)

Settings that control the verbosity and format of log output from the Evolve node. These are typically set via global flags.

**YAML Section:**

```yaml
log:
  # ... logging configurations ...
```

### Log Level

**Description:**
Sets the minimum severity level for log messages to be displayed. Common levels include `debug`, `info`, `warn`, `error`.

**YAML:**

```yaml
log:
  level: "info"
```

**Command-line Flag:**
`--log.level <string>` (Note: some applications might use a different flag name like `--log_level`)
_Example:_ `--log.level debug`
_Default:_ `"info"`
_Constant:_ `FlagLogLevel` (value: "evolve.log.level", but often overridden by global app flags)

### Log Format

**Description:**
Sets the format for log output. Common formats include `text` (human-readable) and `json` (structured, machine-readable).

**YAML:**

```yaml
log:
  format: "text"
```

**Command-line Flag:**
`--log.format <string>` (Note: some applications might use a different flag name like `--log_format`)
_Example:_ `--log.format json`
_Default:_ `"text"`
_Constant:_ `FlagLogFormat` (value: "evolve.log.format", but often overridden by global app flags)

### Log Trace (Stack Traces)

**Description:**
If true, enables the inclusion of stack traces in error logs. This can be very helpful for debugging issues by showing the call stack at the point of an error.

**YAML:**

```yaml
log:
  trace: false
```

**Command-line Flag:**
`--log.trace` (boolean, presence enables it; Note: some applications might use a different flag name like `--log_trace`)
_Example:_ `--log.trace`
_Default:_ `false`
_Constant:_ `FlagLogTrace` (value: "evolve.log.trace", but often overridden by global app flags)

## Signer Configuration (`signer`)

Settings related to the signing mechanism used by the node, particularly for aggregators that need to sign blocks.

**YAML Section:**

```yaml
signer:
  # ... signer configurations ...
```

### Signer Type

**Description:**
Specifies the type of remote signer to use. Common options might include `file` (for key files) or `grpc` (for connecting to a remote signing service).

**YAML:**

```yaml
signer:
  signer_type: "file"
```

**Command-line Flag:**
`--evnode.signer.signer_type <string>`
_Example:_ `--evnode.signer.signer_type grpc`
_Default:_ `"file"`
_Constant:_ `FlagSignerType`

### Signer Path

**Description:**
The path to the signer file (if `signer_type` is `file`) or the address of the remote signer service (if `signer_type` is `grpc` or similar).

**YAML:**

```yaml
signer:
  signer_path: "/path/to/priv_validator_key.json" # For file signer
  # signer_path: "localhost:9000" # For gRPC signer
```

**Command-line Flag:**
`--evnode.signer.signer_path <string>`
_Example:_ `--evnode.signer.signer_path ./config`
_Default:_ `"config"`
_Constant:_ `FlagSignerPath`

### Signer Passphrase File

**Description:**
Path to a file containing the passphrase for the signer key. Required when using a `file` signer in aggregator mode. Reading the passphrase from a file avoids exposing it in shell history.

**YAML:**
This is not stored in the YAML file. Provide it via flag or environment variable.

**Command-line Flag:**
`--evnode.signer.passphrase_file <string>`
_Example:_ `--evnode.signer.passphrase_file /run/secrets/signer_passphrase`
_Default:_ `""` (empty)
_Constant:_ `FlagSignerPassphraseFile`

## Raft Configuration (`raft`)

Settings for Raft-based consensus used for leader election and state replication in multi-aggregator deployments. All fields are ignored when `raft.enable` is false.

**Note:** Raft consensus and `catchup_timeout` are mutually exclusive. Enabling both will produce a validation error.

**YAML Section:**

```yaml
raft:
  # ... raft configurations ...
```

### Enable Raft

**Description:**
If true, enables Raft consensus for leader election and state replication across multiple aggregator nodes.

**YAML:**

```yaml
raft:
  enable: true
```

**Command-line Flag:**
`--evnode.raft.enable` (boolean, presence enables it)
_Default:_ `false`
_Constant:_ `FlagRaftEnable`

### Raft Node ID

**Description:**
Unique identifier for this node within the Raft cluster. Required when Raft is enabled.

**YAML:**

```yaml
raft:
  node_id: "node1"
```

**Command-line Flag:**
`--evnode.raft.node_id <string>`
_Default:_ `""` (required when enabled)
_Constant:_ `FlagRaftNodeID`

### Raft Address

**Description:**
Network address (host:port) for Raft inter-node communication. Required when Raft is enabled.

**YAML:**

```yaml
raft:
  raft_addr: "0.0.0.0:7000"
```

**Command-line Flag:**
`--evnode.raft.raft_addr <string>`
_Default:_ `""` (required when enabled)
_Constant:_ `FlagRaftAddr`

### Raft Directory

**Description:**
Directory for storing Raft logs and snapshots. Required when Raft is enabled.

**YAML:**

```yaml
raft:
  raft_dir: "/home/user/.evnode/raft"
```

**Command-line Flag:**
`--evnode.raft.raft_dir <string>`
_Default:_ `"<home>/raft"`
_Constant:_ `FlagRaftDir`

### Raft Bootstrap

**Description:**
Legacy compatibility flag. Startup mode is now auto-selected from persisted raft configuration state, so this flag is not used to choose bootstrap vs rejoin.

**YAML:**

```yaml
raft:
  bootstrap: true
```

**Command-line Flag:**
`--evnode.raft.bootstrap` (boolean, presence enables it)
_Default:_ `false`
_Constant:_ `FlagRaftBootstrap`

### Raft Peers

**Description:**
Comma-separated list of peer Raft addresses in `nodeID@host:port` format.

**YAML:**

```yaml
raft:
  peers: "node2@192.168.1.2:7000,node3@192.168.1.3:7000"
```

**Command-line Flag:**
`--evnode.raft.peers <string>`
_Default:_ `""` (empty)
_Constant:_ `FlagRaftPeers`

### Raft Startup Mode

Raft startup mode is selected automatically from local raft configuration state:

- If the node already has persisted raft configuration in `raft.raft_dir`, it starts in rejoin mode.
- If no raft configuration exists yet, it bootstraps a cluster from configured peers.
- `raft.bootstrap` is retained for compatibility but does not control mode selection.

`--evnode.raft.rejoin` has been removed.

### Raft Snap Count

**Description:**
Number of log entries between Raft snapshots. Lower values reduce recovery time but increase snapshot I/O overhead.

**YAML:**

```yaml
raft:
  snap_count: 10000
```

**Command-line Flag:**
`--evnode.raft.snap_count <uint64>`
_Default:_ `0`
_Constant:_ `FlagRaftSnapCount`

### Raft Send Timeout

**Description:**
Maximum duration to wait for a message to be sent to a peer before considering it failed.

**YAML:**

```yaml
raft:
  send_timeout: "200ms"
```

**Command-line Flag:**
`--evnode.raft.send_timeout <duration>`
_Default:_ `"200ms"`
_Constant:_ `FlagRaftSendTimeout`

### Raft Heartbeat Timeout

**Description:**
Time between leader heartbeats sent to followers.

**YAML:**

```yaml
raft:
  heartbeat_timeout: "350ms"
```

**Command-line Flag:**
`--evnode.raft.heartbeat_timeout <duration>`
_Default:_ `"350ms"`
_Constant:_ `FlagRaftHeartbeatTimeout`

### Raft Leader Lease Timeout

**Description:**
Duration of the leader lease, which allows a leader to serve reads locally without round-tripping to followers.

**YAML:**

```yaml
raft:
  leader_lease_timeout: "175ms"
```

**Command-line Flag:**
`--evnode.raft.leader_lease_timeout <duration>`
_Default:_ `"175ms"`
_Constant:_ `FlagRaftLeaderLeaseTimeout`

---

This reference should help you configure your Evolve node effectively. Always refer to the specific version of Evolve you are using, as options and defaults may change over time.
