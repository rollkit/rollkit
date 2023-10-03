# Full Node

## Abstract

A Full Node is a top-level service that encapsulates different components of Rollkit and initializes/manages them. The main components are listed in the table below:

| Component | Description |
| :--- | :--- |
| proxyApp | ProxyApp is the interface to the application that consists of multiple connections. It is used to communicate with the application. |
| genesisDoc | GenesisDoc is the genesis document that contains information about the initial state of the blockchain. |
| conf | Conf is the configuration of the node. It contains all the necessary settings for the node to be initialized and function properly. |
| P2P | P2P is the peer-to-peer client used for communication between nodes in the network. |
| Mempool | Mempool is the transaction pool where all the transactions are stored before they are added to a block. |
| Store | Store is used to store/retrieve blocks, commits, and state. |
| blockManager | BlockManager is responsible for managing the operations related to blocks such as creating and validating blocks. |
| dalc | DALC is the Data Availability Layer Client used to interact with the data availability layer. |
| hExService | HExService is the Header Exchange Service used for exchanging block headers between nodes over P2P. |
| bExService | BExService is the Block Exchange Service used for exchanging blocks between nodes over P2P. |

## Details

### Full Node
A Full Node is initialized inside the Cosmos SDK start script along with the node configuration, a private key to use in the P2P client, a private key for signing blocks as a block proposer, a client creator, a genesis document, and a logger. It uses them to initialize the components described above. The components TxIndexer, BlockIndexer, and IndexerService exist to ensure cometBFT compatibility since they are needed for most of the RPC calls from the `SignClient` interface from cometBFT.

 Note that unlike a light node which only syncs and stores block headers seen on the P2P layer, the full node also syncs and stores full blocks seen on both the P2P network and the DA layer. Full blocks contain all the transactions published as part of the block. 

## Message Structure/Communication Format

The Full Node communicates with other nodes in the network using the P2P client. It also communicates with the application using the ABCI proxy connections. The communication format is based on the P2P and ABCI protocols.

## Assumptions and Considerations

The Full Node assumes that the configuration, private keys, client creator, genesis document, and logger are correctly passed in by the Cosmos SDK. It also assumes that the P2P client, data availability layer client, mempool, block manager, and other services can be started and stopped without errors.

## Implementation

See [full node]

## References

[1] [Full Node][full node]

[full node]: ../node/full.go
