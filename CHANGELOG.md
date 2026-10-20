# Changelog

<!--
All notable changes to this module will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
-->

## [Unreleased]

### Added

- Client-side signed blob submission via the new `da/celestia` module: with `da.tx_signer.enabled`, `testapp` and `evm` sign PayForBlobs locally and broadcast them over consensus gRPC (`x-token` auth), so they can submit through DA endpoints that cannot sign, such as da-proxy. Off by default. `block.NewDAClient` and `pkg/cmd.StartNode` accept options (`block.WithSubmitter`) to inject a submitter; existing callers are unaffected.

### Fixed

- Prune per-height state snapshots in `all` pruning mode, keep the state needed to replay the oldest retained block, and stop pruning from falling behind when more than 10k blocks are produced per interval [#3469](https://github.com/evstack/ev-node/pull/3469).
- Do not halt the node when `SetFinal` fails while the DA-included height lags the head; the EVM client skips finalizing historical blocks the execution layer no longer serves, and the submitter retries on the next tick [#3473](https://github.com/evstack/ev-node/pull/3473).

## v1.2.3

### Changed

- Consolidate Go dependency updates across modules [#3420](https://github.com/evstack/ev-node/pull/3420).

### Fixed

- Support P2P synchronization of blocks up to the configured maximum block size [#3419](https://github.com/evstack/ev-node/pull/3419).
- Clean pending DA data after a height has already been processed, preventing stale pending data from accumulating [#3413](https://github.com/evstack/ev-node/pull/3413).
- Synchronize distant P2P data heads with a range request instead of fetching intermediate blocks one at a time [#3421](https://github.com/evstack/ev-node/pull/3421).

## v1.2.2

### Changed

- Update Go dependencies, including gRPC and QUIC transport packages.

### Fixed

- Skip DA cache restoration for P2P-only full followers, allowing them to restart without a configured DA endpoint [#3408](https://github.com/evstack/ev-node/pull/3408).

## v1.2.1


### Added

- **BREAKING:** Proposer key rotation. State now tracks `NextProposerAddress`, and signed headers validate signer identity (the signer address must derive from the included public key), allowing the aggregator signing key to be rotated across blocks [#3282](https://github.com/evstack/ev-node/pull/3282)
- Prepare for the Amsterdam fork: update ev-reth to reth 2.3 and add engine-API fork upgrade handling [#3352](https://github.com/evstack/ev-node/pull/3352)

### Removed

- **BREAKING:** Remove the obsolete `evgrpc` app and `execution/grpc` transport module [#3380](https://github.com/evstack/ev-node/pull/3380).

### Fixed

- Allow full followers without a DA endpoint to sync through P2P only, and reject aggregator startup without an explicit DA endpoint [#3386](https://github.com/evstack/ev-node/pull/3386).
- Reject unsupported `net-info --output` formats instead of falling back to text output [#3360](https://github.com/evstack/ev-node/pull/3360).
- Report readiness correctly for non-aggregator nodes [#3376](https://github.com/evstack/ev-node/pull/3376).
- Don't create a single sequencer for non-aggregator nodes [#3389](https://github.com/evstack/ev-node/pull/3389).

## v1.1.4

### Fixed

- DA client falls back to HTTP polling with `Retrieve` when the WebSocket connection fails, instead of trying to use the WS-only `Subscribe` over HTTP. A background goroutine retries WS every 30s so transient outages don't force a permanent downgrade [#3361](https://github.com/evstack/ev-node/pull/3361)

## v1.1.3

### Fixed

- Drain the pending tx queue in merged batches with a durable WAL-backed ack, fixing severe queue backlog under heavy tx load. Tx dedup moved from the reaper cache into the sequencer queue [#3351](https://github.com/evstack/ev-node/pull/3351)

## v1.1.2

### Changes

- Add max bytes contraints in simple solo sequnecer [#3312](https://github.com/evstack/ev-node/pull/3312)
- Add support for otlp in execution/grpc. [#3300](https://github.com/evstack/ev-node/pull/3300)
- Optimization of mutex usage in cache for reaper [#3286](https://github.com/evstack/ev-node/pull/3286)
- Add Unix domain socket support for gRPC execution endpoints via `unix:///path/to/socket` [#3297](https://github.com/evstack/ev-node/pull/3297)
- **BREAKING:** (execution/grpc)
  - Move execution service where it belongs in execution/grpc. [#3302](https://github.com/evstack/ev-node/pull/3302)
  - Replace legacy gRPC execution `txs` payload fields with `tx_batch` so clients and servers use contiguous transaction buffers [#3297](https://github.com/evstack/ev-node/pull/3297)
- Optimize metadata writes by making it async in cache store [#3298](https://github.com/evstack/ev-node/pull/3298)
- Reduce tx cache retention to avoid OOM under (really) heavy tx load [#3299](https://github.com/evstack/ev-node/pull/3299)

### Fixed

- Increase P2P pubsub max message size to match `DefaultMaxBlobSize`, preventing fullnode desync on large blocks [#3344](https://github.com/evstack/ev-node/pull/3344).

## v1.1.1

### Changes

- Improve P2P gossiping by switching pubsub internals from `GossipSub` to `FloodSub` [#3263](https://github.com/evstack/ev-node/pull/3263)
- Add `sequencer_blocks_synchronized_total` Prometheus counter metric tracking blocks synced by source (DA/P2P) [#3259](https://github.com/evstack/ev-node/pull/3259)
- Make it easier to override `DefaultMaxBlobSize` by ldflags [#3235](https://github.com/evstack/ev-node/pull/3235)
- Add solo sequencer (simple in memory single sequencer without force inclusion) [#3235](https://github.com/evstack/ev-node/pull/3235)
- Improve reaper to sustain txs burst better [#3236](https://github.com/evstack/ev-node/pull/3236)

### Fixed

- Replace persistent P2P connection gater with a no-op variant so stale blocklist entries can no longer prevent peers from connecting after a restart. The new `p2p.disable_connection_gater` flag (default `true`) can be set to `false` to re-enable peer filtering when experiencing P2P flooding [#3273](https://github.com/evstack/ev-node/pull/3273)
- Raft HA production hardening: leader fencing on SIGTERM, FSM data race, follower restart crash, log compaction config, and election timeout validation [#3230](https://github.com/evstack/ev-node/pull/3230)

## v1.1.0

No changes from v1.1.0-rc.2.

## v1.1.0-rc.2

### Changes

- Added publisher-mode synchronization option for failover scenarios with early P2P infrastructure readiness [#3222](https://github.com/evstack/ev-node/pull/3222)
- Improve P2P transient network failure [#3212](https://github.com/evstack/ev-node/pull/3212)
- Improve execution/evm check for stored meta not stale [#3221](https://github.com/evstack/ev-node/pull/3221)

## v1.1.0-rc.1

### Added

- Add AWS & GCP KMS signer backend [#3171](https://github.com/evstack/ev-node/pull/3171)
- Subscribe to forced inclusion namespace events [#3146](https://github.com/evstack/ev-node/pull/3146)
- Display block source in sync log [#3193](https://github.com/evstack/ev-node/pull/3193)

### Fixed

- Avoid evicting yet to be processed heights [#3204](https://github.com/evstack/ev-node/pull/3204)
- Bound Badger index cache memory to prevent growth with chain length [3209](https://github.com/evstack/ev-node/pull/3209)
- Refetch latest da height instead of da height +1 when P2P is offline [#3201](https://github.com/evstack/ev-node/pull/3201)
- Fix race on startup sync. [#3162](https://github.com/evstack/ev-node/pull/3162)
- Strict raft state. [#3167](https://github.com/evstack/ev-node/pull/3167)
- Retry fetching the timestamp on error in da-client [#3166](https://github.com/evstack/ev-node/pull/3166)

## v1.0.0

### Fixed

- Persist cache snapshot only once at shutdown to avoid Badger vlog
  increase. [#3153](https://github.com/evstack/ev-node/pull/3153)

## v1.0.0-rc.5

### Added

- Add disaster recovery for sequencer
  - Catch up possible DA-only blocks when restarting. [#3057](https://github.com/evstack/ev-node/pull/3057)
  - Verify DA and P2P state on restart (prevent double-signing). [#3061](https://github.com/evstack/ev-node/pull/3061)
- Node pruning support. [#2984](https://github.com/evstack/ev-node/pull/2984)
  - Two different sort of pruning implemented:
    _Classic pruning_ (`all`): prunes given `HEAD-n` blocks from the databases, including store metadatas.
    _Auto Storage Optimization_ (`metadata`): prunes only the state metadatas, keeps all blocks.
    By using one or the other, you are losing the ability to rollback or replay transactions earlier than `HEAD-n`.
    When using _classic pruning_, you aren't able to fetch blocks prior to `HEAD-n`.

### Fixed

- Fix block timer to account for execution time. Previously, the block timer reset to the full `block_time` duration
  after `ProduceBlock` completed, making the effective interval `block_time + execution_time`. Now the timer subtracts
  elapsed execution time so blocks are produced at the configured cadence.

### Changes

- Store pending blocks separately from executed blocks key. [#3073](https://github.com/evstack/ev-node/pull/3073)
- Fixes issues with force inclusion verification on sync nodes. [#3057](https://github.com/evstack/ev-node/pull/3057)
- Add flag to `local-da` to produce empty DA blocks (closer to the real
  system). [#3057](https://github.com/evstack/ev-node/pull/3057)
- Validate P2P DA height hints against the latest known DA height to prevent malicious peers from triggering runaway
  catchup. [#3128](https://github.com/evstack/ev-node/pull/3128)
- Replace syncer DA polling system by DA subscription via
  websockets. [#3131](https://github.com/evstack/ev-node/pull/3131)

## v1.0.0-rc.4

### Changes

- Skip draining when exec client unavailable. [#3060](https://github.com/evstack/ev-node/pull/3060)

## v1.0.0-rc.3

### Added

- Add DA Hints for P2P transactions. This allows a catching up node to be on sync with both DA and
  P2P. ([#2891](https://github.com/evstack/ev-node/pull/2891))

### Changes

- Improve `cache.NumPendingData` to not return empty data. Automatically bumps `LastSubmittedHeight` to reflect
  that. ([#3046](https://github.com/evstack/ev-node/pull/3046))
- **BREAKING** Make pending events cache and tx cache fully ephemeral. Those will be re-fetched on restart. DA Inclusion
  cache persists until cleared up after DA inclusion has been processed. Persist accross restart using store
  metadata. ([#3047](https://github.com/evstack/ev-node/pull/3047))
- Replace LRU cache by standard mem cache with manual eviction in `store_adapter`. When P2P blocks were fetched too
  fast, they would be evicted before being executed [#3051](https://github.com/evstack/ev-node/pull/3051)
- Fix replay logic leading to app hashes by verifying against the wrong
  block [#3053](https://github.com/evstack/ev-node/pull/3053).

## v1.0.0-rc.2

### Changes

- Improve cache handling when there is a significant backlog of pending headers and
  data. ([#3030](https://github.com/evstack/ev-node/pull/3030))
- Decrease MaxBytesSize to `5MB` to increase compatibility with public
  nodes. ([#3030](https://github.com/evstack/ev-node/pull/3030))
- Proper counting of `DASubmitterPendingBlobs` metrics. [#3038](https://github.com/evstack/ev-node/pull/3038)
- Replace `go-header` store by `ev-node` store. This avoid duplication of all blocks in `go-header` and `ev-node` store.
  Thanks to the cached store from #3030, this should improve p2p performance as
  well. [#3036](https://github.com/evstack/ev-node/pull/3036)

## v1.0.0-rc.1

### Added

- Added OpenTelemetry tracing support with OTLP export for distributed tracing across ev-node components including block
  production, syncing, DA submission/retrieval, sequencer, store operations, and RPC layer. Configurable via
  `instrumentation.tracing`, `instrumentation.tracing_endpoint`, `instrumentation.tracing_service_name`, and
  `instrumentation.tracing_sample_rate` settings. ([#2956](https://github.com/evstack/ev-node/issues/2956))
- **BREAKING:** Implement forced inclusion and batch sequencing ([#2797](https://github.com/evstack/ev-node/pull/2797))
  **This change requires adding a `da_epoch_forced_inclusion` field to the node's `genesis.json` file.** The recommended
  value is `100`.
  Full support for this feature will be available in a future release.
- Added `post-tx` command and force inclusion server to submit transactions directly to the DA
  layer. ([#2888](https://github.com/evstack/ev-node/pull/2888))
  Additionally, modified the core package to support marking transactions as forced included transactions.
  The execution client ought to perform basic validation on those transactions as they have skipped the execution
  client's mempool.
- Added batching strategies (default stay time-based, unchanged from previous betas). Currently available strategies are
  `time`, `size`, `immediate` and
  `adaptive`. [Full documentation can be found here](https://github.com/evstack/ev-node/blob/122486de98d09ecd37d792b88814dcf07238f28a/docs/learn/config.md?plain=1#L521-L597).
- Added `FilterTxs` method to the execution interface. This method is meant to filter txs by size and if the execution
  clients allows it, by gas. This is useful for force included transactions, as those aren't filtered by the sequencer's
  mempool.
- Added `GetExecutionInfo` method to the execution interface. This method returns some execution information, such as
  the maximum gas per block.

### Changed

- **BREAKING:** Renamed `evm-single` to `evm` and `grpc-single` to `evgrpc` for
  clarity. [#2839](https://github.com/evstack/ev-node/pull/2839). You may need to manually modify your evnode.yaml
  `signer.signer_path` if your $HOME folder is changed.
- Split cache interface into `CacheManager` and `PendingManager` and created `da` client to easy DA
  handling. [#2878](https://github.com/evstack/ev-node/pull/2878)
- Improved startup DA retrieval height when cache is cleared or
  empty. [#2880](https://github.com/evstack/ev-node/pull/2880)

### Removed

- **BREAKING:** Removed unused and confusing metrics from sequencers and block processing, including sequencer-specific
  metrics (gas price, blob size, transaction status, pending blocks), channel buffer metrics, overly granular error
  metrics, block production categorization metrics, and sync lag metrics. Essential metrics for DA submission health,
  block production, and performance monitoring are retained. [#2904](https://github.com/evstack/ev-node/pull/2904)
- **BREAKING**: Removed `core/da` package and replaced DAClient with internal implementation. The DA client is exposed
  as `block.FullDAClient`, `block.DAClient`, `block.DAVerifier` without leaking implementation
  details. [#2910](https://github.com/evstack/ev-node/pull/2910)

## v1.0.0-beta.11

### Improvements

- Loosen syncer validation for allowing swapping sequencer and full node
  state [#2925](https://github.com/evstack/ev-node/pull/2925)

## v1.0.0-beta.10

### Added

- Enhanced health check system with separate liveness (`/health/live`) and readiness (`/health/ready`) HTTP endpoints.
  Readiness endpoint includes P2P listening check and aggregator block production rate validation (5x block time
  threshold). ([#2800](https://github.com/evstack/ev-node/pull/2800))
- Added `GetP2PStoreInfo` RPC method to retrieve head/tail metadata for go-header stores used by P2P
  sync ([#2835](https://github.com/evstack/ev-node/pull/2835))
- Added protobuf definitions for `P2PStoreEntry` and `P2PStoreSnapshot` messages to support P2P store inspection

### Changed

- Improved EVM execution client payload status validation with proper retry logic for SYNCING states in `InitChain`,
  `ExecuteTxs`, and `SetFinal` methods. The implementation now follows Engine API specification by retrying
  SYNCING/ACCEPTED status with exponential backoff and failing immediately on INVALID status, preventing unnecessary
  node shutdowns during transient execution engine sync
  operations. ([#2863](https://github.com/evstack/ev-node/pull/2863))
- Remove GasPrice and GasMultiplier from DA interface and configuration to use celestia-node's native fee
  estimation. ([#2822](https://github.com/evstack/ev-node/pull/2822))
- Use cache instead of in memory store for reaper. Persist cache on reload. Autoclean after 24
  hours. ([#2811](https://github.com/evstack/ev-node/pull/2811))
- Improved P2P sync service store initialization to be atomic and prevent race
  conditions ([#2838](https://github.com/evstack/ev-node/pull/2838))
- Enhanced P2P bootstrap behavior to intelligently detect starting height from local store instead of requiring trusted
  hash
- Relaxed execution layer height validation in block replay to allow execution to be ahead of target height, enabling
  recovery from manual intervention scenarios

### Removed

- **BREAKING:** Removed `evnode.v1.HealthService` gRPC endpoint. Use HTTP endpoints: `GET /health/live` and
  `GET /health/ready`. ([#2800](https://github.com/evstack/ev-node/pull/2800))
- **BREAKING:** Removed `TrustedHash` configuration option and `--evnode.node.trusted_hash` flag. Sync service now
  automatically determines starting height from local store
  state ([#2838](https://github.com/evstack/ev-node/pull/2838))
- **BREAKING:** Removed unused and confusing metrics from sequencers and block processing, including sequencer-specific
  metrics (gas price, blob size, transaction status, pending blocks), channel buffer metrics, overly granular error
  metrics, block production categorization metrics, and sync lag metrics. Essential metrics for DA submission health,
  block production, and performance monitoring are retained. [#2904](https://github.com/evstack/ev-node/pull/2904)

### Fixed

- Fixed sync service initialization issue when node is not on genesis but has an empty store

## v1.0.0-beta.9

### Added

<!-- New features or capabilities -->

- Added automated upgrade test for the `evm` app that verifies compatibility when moving from v1.0.0-beta.8 to HEAD in
  CI ([#2780](https://github.com/evstack/ev-node/pull/2780))
- Added execution-layer replay mechanism so nodes can resynchronize by replaying missed batches against the
  executor ([#2771](https://github.com/evstack/ev-node/pull/2771))
- Added cache-pruning logic that evicts entries once heights are finalized to keep node memory usage
  bounded ([#2761](https://github.com/evstack/ev-node/pull/2761))
- Added Prometheus gauges and counters that surface DA submission failures, pending blobs, and resend attempts for
  easier operational monitoring ([#2756](https://github.com/evstack/ev-node/pull/2756))
- Added gRPC execution client implementation for remote execution services using Connect-RPC
  protocol ([#2490](https://github.com/evstack/ev-node/pull/2490))
- Added `ExecutorService` protobuf definition with InitChain, GetTxs, ExecuteTxs, and SetFinal
  RPCs ([#2490](https://github.com/evstack/ev-node/pull/2490))
- Added new `grpc` app for running EVNode with a remote execution layer via
  gRPC ([#2490](https://github.com/evstack/ev-node/pull/2490))

### Changed

<!-- Changes to existing functionality -->

- Hardened signer CLI and block pipeline per security audit: passphrases must be provided via
  `--evnode.signer.passphrase_file`, JWT secrets must be provided via `--evm.jwt-secret-file`, data/header validation
  enforces metadata and timestamp checks, and the reaper backs off on failures (
  BREAKING) ([#2764](https://github.com/evstack/ev-node/pull/2764))
- Added retries around executor `ExecuteTxs` calls to better tolerate transient execution
  errors ([#2784](https://github.com/evstack/ev-node/pull/2784))
- Increased default `ReadinessMaxBlocksBehind` from 3 to 30 blocks so `/health/ready` stays true during normal batch
  sync ([#2779](https://github.com/evstack/ev-node/pull/2779))
- Updated EVM execution client to use new `txpoolExt_getTxs` RPC API for retrieving pending transactions as RLP-encoded
  bytes

### Removed

- Removed `LastCommitHash`, `ConsensusHash`, and `LastResultsHash` from the canonical header representation in favor of
  slim headers (BREAKING; legacy hashes now live under
  `Header.Legacy`) ([#2766](https://github.com/evstack/ev-node/pull/2766))

[Unreleased]: https://github.com/evstack/ev-node/compare/v1.0.0-beta.1...HEAD
