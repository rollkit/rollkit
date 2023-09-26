# P2P Header Sync

## Abstract

The P2P Header Sync is a p2pP exchange service for rollkit headers that implements the [go-header][go-header] interface. The main components are:

* store: a `headerEx` prefixed datastore where synced headers are stored
* subscriber: a libp2p node pubsub subscriber
* p2p server: a server for handling header requests between peers in the p2p network
* exchange: a client that enables sending in/out-bound header requests from/to the p2p network
* syncer: a service for efficient synchronization for headers. When a p2p node falls behind and wants to catch up to the latest network head via p2p network, it can use the syncer.

## Details

All three types of nodes (sequencer, full, and light) run the P2P header exchange service to maintain the cannonical view of the rollup chain (with respect to the p2p network).

The header exchange service inherits the ConnectionGater from the node's p2p client which allows blocking and allowing peers as needed by specifying the `P2PConfig.BlockedPeers` and `P2PConfig.AllowedPeers`.

`NodeConfig.BlockTime` is used to configure the syncer such that it can effectively decide the outdated headers while it receives headers from the p2p network.

Both header and block sync utilizes go-header library and runs two separate exchange services (p2p header sync and p2p block sync). This distinction is mainly to serve light nodes which does not store blocks, but only headers synced from the p2p network. The two separate service may be redundant for the full nodes and can be optimized to single service in the future.

### Consumption of Header Sync

The sequencer node upon successfully creating the block publishes the signed block header to p2p network using the p2p header exchange service. The full/light nodes run the header exchange service in background to receive and store the signed headers from the p2p network. Currently the full/light nodes does not consume the p2p synced headers, however in future they have utilities to perform certain checks.

## Assumptions

* The header exchange store is created by prefixing `headerEx` the main datastore.
* The genesis `ChainID` is used to create the `PubsubTopicID` in go-header. For example, for ChainID `gm`, the pubsub topic id is `/gm/header-sub/v0.0.1`. Refer to go-header specs for further details.
* The header store must be initialized with genesis header before starting the syncer service. The genesis header can be loaded by passing the genesis header hash via `NodeConfig.TrustedHash` configuration parameter or by querying the p2p network. This imposes a time constraint that full/light nodes have to wait for the aggregator to publish the genesis header before starting the p2p header exchange service.
* The P2P Header sync works only when the node is connected to p2p network by specifying the initial seeds to connect to via `P2PConfig.Seeds` configuration parameter.
* Node's context is passed down to all the components of the p2p header exchange to control shutting down the service either abruptly (in case of failure) or gracefully (during successful scenarios).

## Implementation

The P2P header exchange implementation can be found in [node/header_exchange.go](https://github.com/rollkit/rollkit/blob/main/node/header_exchange.go). The full and light nodes create and start the header exchange service under [full](https://github.com/rollkit/rollkit/blob/main/node/full.go) and [light](https://github.com/rollkit/rollkit/blob/main/node/light.go).
