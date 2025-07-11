# Header and Data Sync

## Abstract

The nodes in the P2P network sync headers and data using separate sync services that implement the [go-header][go-header] interface. Rollkit uses a header/data separation architecture where headers and transaction data are synchronized independently through parallel services. Each sync service consists of several components as listed below.

|Component|Description|
|---|---|
|store| a prefixed [datastore][datastore] where synced items are stored (`headerSync` prefix for headers, `dataSync` prefix for data)|
|subscriber | a [libp2p][libp2p] node pubsub subscriber for the specific data type|
|P2P server| a server for handling requests between peers in the P2P network|
|exchange| a client that enables sending in/out-bound requests from/to the P2P network|
|syncer| a service for efficient synchronization. When a P2P node falls behind and wants to catch up to the latest network head via P2P network, it can use the syncer.|

## Details

Rollkit implements two separate sync services:

### Header Sync Service

- Synchronizes `SignedHeader` structures containing block headers with signatures
- Used by all node types (sequencer, full, and light)
- Essential for maintaining the canonical view of the chain

### Data Sync Service  

- Synchronizes `Data` structures containing transaction data
- Used only by full nodes and sequencers
- Light nodes do not run this service as they only need headers

Both services:

- Utilize the generic `SyncService[H header.Header[H]]` implementation
- Inherit the `ConnectionGater` from the node's P2P client for peer management
- Use `NodeConfig.BlockTime` to determine outdated items during sync
- Operate independently on separate P2P topics and datastores

### Consumption of Sync Services

#### Header Sync

- Sequencer nodes publish signed headers to the P2P network after block creation
- Full and light nodes receive and store headers for chain validation
- Headers contain commitments (DataHash) that link to the corresponding data

#### Data Sync

- Sequencer nodes publish transaction data separately from headers
- Only full nodes receive and store data (light nodes skip this)
- Data is linked to headers through the DataHash commitment

#### Parallel Broadcasting

The block manager broadcasts headers and data in parallel when publishing blocks:

- Headers are sent through `headerBroadcaster`
- Data is sent through `dataBroadcaster`
- This enables efficient network propagation of both components

## Assumptions

- Separate datastores are created with different prefixes:
  - Headers: `headerSync` prefix on the main datastore
  - Data: `dataSync` prefix on the main datastore
- Network IDs are suffixed to distinguish services:
  - Header sync: `{network}-headerSync`
  - Data sync: `{network}-dataSync`
- Chain IDs for pubsub topics are also separated:
  - Headers: `{chainID}-headerSync` creates topic like `/gm-headerSync/header-sub/v0.0.1`
  - Data: `{chainID}-dataSync` creates topic like `/gm-dataSync/header-sub/v0.0.1`
- Both stores must be initialized with genesis items before starting:
  - Header store needs genesis header
  - Data store needs genesis data (if applicable)
- Genesis items can be loaded via `NodeConfig.TrustedHash` or P2P network query
- Sync services work only when connected to P2P network via `P2PConfig.Seeds`
- Node context is passed to all components for graceful shutdown
- Headers and data are linked through DataHash but synced independently

## Implementation

The sync service implementation can be found in [pkg/sync/sync_service.go][sync-service]. The generic `SyncService[H header.Header[H]]` is instantiated as:

- `HeaderSyncService` for syncing `*types.SignedHeader`
- `DataSyncService` for syncing `*types.Data`

Full nodes create and start both services, while light nodes only start the header sync service. The services are created in [full][fullnode] and [light][lightnode] node implementations.

The block manager integrates with both services through:

- `HeaderStoreRetrieveLoop()` for retrieving headers from P2P
- `DataStoreRetrieveLoop()` for retrieving data from P2P
- Separate broadcast channels for publishing headers and data

## References

[1] [Header Sync][sync-service]

[2] [Full Node][fullnode]

[3] [Light Node][lightnode]

[4] [go-header][go-header]

[sync-service]: https://github.com/rollkit/rollkit/blob/main/pkg/sync/sync_service.go
[fullnode]: https://github.com/rollkit/rollkit/blob/main/node/full.go
[lightnode]: https://github.com/rollkit/rollkit/blob/main/node/light.go
[go-header]: https://github.com/celestiaorg/go-header
[libp2p]: https://github.com/libp2p/go-libp2p
[datastore]: https://github.com/ipfs/go-datastore
