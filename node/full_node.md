# Full Node

## Abstract

A Full Node is a top-level service that encapsulates different components of Rollkit and initializes/manages them.

## Details

### Full Node Details

A Full Node is initialized inside the Cosmos SDK start script along with the node configuration, a private key to use in the P2P client, a private key for signing blocks as a block proposer, a client creator, a genesis document, and a logger. It uses them to initialize the components described above. The components TxIndexer, BlockIndexer, and IndexerService exist to ensure cometBFT compatibility since they are needed for most of the RPC calls from the `SignClient` interface from cometBFT.

Note that unlike a light node which only syncs and stores block headers seen on the P2P layer, the full node also syncs and stores full blocks seen on both the P2P network and the DA layer. Full blocks contain all the transactions published as part of the block.

The Full Node mainly encapsulates and initializes/manages the following components:

### proxyApp

The Cosmos SDK start script passes a client creator constructed using the relevant Cosmos SDK application to the Full Node's constructor which is then used to create the proxy app interface. When the proxy app is started, it establishes different [ABCI app connections] including Mempool, Consensus, Query, and Snapshot. The full node uses this interface to interact with the application.

### genesisDoc

The [genesis] document contains information about the initial state of the rollup chain, in particular its validator set.

### conf

The [node configuration] contains all the necessary settings for the node to be initialized and function properly.

### P2P

The [peer-to-peer client] is used to gossip transactions between full nodes in the network.

### Mempool

The [Mempool] is the transaction pool where all the transactions are stored before they are added to a block.

### Store

The [Store] is initialized with `DefaultStore`, an implementation of the [store interface] which is used for storing and retrieving blocks, commits, and state. |

### blockManager

The [Block Manager] is responsible for managing the operations related to blocks such as creating and validating blocks.

### dalc

The [Data Availability Layer Client][dalc] is used to interact with the data availability layer. It is initialized with the DA Layer and DA Config specified in the node configuration.

### hExService

The [Header Sync Service] is used for syncing block headers between nodes over P2P.

### bSyncService

The [Block Sync Service] is used for syncing blocks between nodes over P2P.

## Message Structure/Communication Format

The Full Node communicates with other nodes in the network using the P2P client. It also communicates with the application using the ABCI proxy connections. The communication format is based on the P2P and ABCI protocols.

## Assumptions and Considerations

The Full Node assumes that the configuration, private keys, client creator, genesis document, and logger are correctly passed in by the Cosmos SDK. It also assumes that the P2P client, data availability layer client, mempool, block manager, and other services can be started and stopped without errors.

## Implementation

See [full node]

## References

[1] [Full Node][full node]

[2] [ABCI Methods][ABCI app connections]

[3] [Genesis Document][genesis]

[4] [Node Configuration][node configuration]

[5] [Peer to Peer Client][peer-to-peer client]

[6] [Mempool][Mempool]

[7] [Store][Store]

[8] [Store Interface][store interface]

[9] [Block Manager][block manager]

[10] [Data Availability Layer Client][dalc]

[11] [Header Sync Service][Header Sync Service]

[12] [Block Sync Service][Block Sync Service]

[full node]: https://github.com/rollkit/rollkit/blob/main/node/full.go
[ABCI app connections]: https://github.com/cometbft/cometbft/blob/main/spec/abci/abci%2B%2B_basic_concepts.md
[genesis]: https://github.com/cometbft/cometbft/blob/main/spec/core/genesis.md
[node configuration]: https://github.com/rollkit/rollkit/blob/main/config/config.go
[peer-to-peer client]: https://github.com/rollkit/rollkit/blob/main/p2p/client.go
[Mempool]: https://github.com/rollkit/rollkit/blob/main/mempool/mempool.go
[Store]: https://github.com/rollkit/rollkit/blob/main/store/store.go
[store interface]: https://github.com/rollkit/rollkit/blob/main/store/types.go
[Block Manager]: https://github.com/rollkit/rollkit/blob/main/block/manager.go
[dalc]: https://github.com/rollkit/rollkit/blob/main/da/da.go
[Header Sync Service]: https://github.com/rollkit/rollkit/blob/main/block/header_sync.go
[Block Sync Service]: https://github.com/rollkit/rollkit/blob/main/block/block_sync.go
