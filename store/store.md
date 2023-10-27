# Store

## Abstract

The Store interface defines methods for storing and retrieving blocks, commits, and the state of the blockchain.

## Protocol/Component Description

The Store interface defines the following methods:

- `Height`: Returns the height of the highest block in the store.
- `SetHeight`: Sets given height in the store if it's higher than the existing height in the store.
- `SaveBlock`: Saves a block along with its seen commit.
- `LoadBlock`: Returns a block at a given height.
- `LoadBlockByHash`: Returns a block with a given block header hash.
- `SaveBlockResponses`: Saves block responses in the Store.
- `LoadBlockResponses`: Returns block results at a given height.
- `LoadCommit`: Returns a commit for a block at a given height.
- `LoadCommitByHash`: Returns a commit for a block with a given block header hash.
- `UpdateState`: Updates the state saved in the Store. Only one State is stored.
- `LoadState`: Returns the last state saved with UpdateState.
- `SaveValidators`: Saves the validator set at a given height.
- `LoadValidators`: Returns the validator set at a given height.

The `TxnDatastore` interface inside [go-datastore] is used for constructing different key-value stores for the underlying storage of a full node. The are two different implementations of `TxnDatastore` in [kv.go]:

- `NewDefaultInMemoryKVStore`: Builds a key-value store that uses the [BadgerDB] library and operates in-memory, without accessing the disk. Used only across unit tests and integration tests.

- `NewDefaultKVStore`: Builds a key-value store that uses the [BadgerDB] library and stores the data on disk at the specified path.

A Rollkit full node is [initialized][full_node_store_initialization] using `NewDefaultKVStore` as the base key-value store for underlying storage. To store various types of data in this base key-value store, different prefixes are used: `mainPrefix`, `dalcPrefix`, and `indexerPrefix`. The `mainPrefix` equal to `0` is used for the main node data, `dalcPrefix` equal to `1` is used for Data Availability Layer Client (DALC) data, and `indexerPrefix` equal to `2` is used for indexing related data.

For the main node data, `DefaultStore` struct, an implementation of the Store interface, is used with the following prefixes for various types of data within it:

- `blockPrefix` with value "b": Used to store blocks in the key-value store.
- `indexPrefix` with value "i": Used to index the blocks stored in the key-value store.
- `commitPrefix` with value "c": Used to store commits related to the blocks.
- `statePrefix` with value "s": Used to store the state of the blockchain.
- `responsesPrefix` with value "r": Used to store responses related to the blocks.
- `validatorsPrefix` with value "v": Used to store validator sets at a given height.

For example, in a call to `LoadBlockByHash` for some block hash `<block_hash>`, the key used in the full node's base key-value store will be `/0/b/<block_hash>` where `0` is the main store prefix and `b` is the block prefix. Similarly, in a call to `LoadValidators` for some height `<height>`, the key used in the full node's base key-value store will be `/0/v/<height>` where `0` is the main store prefix and `v` is the validator set prefix.

Inside the key-value store, the value of these various types of data like `Block`, `Commit`, etc is stored as a byte array which is encoded and decoded using the corresponding Protobuf [marshal and unmarshal methods][serialization].

The store is most widely used inside the [block manager] and [full client] to perform their functions correctly. Within the block manager, since it has multiple go-routines in it, it is protected by a mutex lock, `lastStateMtx`, to synchronize read/write access to it and prevent race conditions.

## Message Structure/Communication Format

The Store does not communicate over the network, so there is no message structure or communication format.

## Assumptions and Considerations

The Store assumes that the underlying datastore is reliable and provides atomicity for transactions. It also assumes that the data passed to it for storage is valid and correctly formatted.

## Implementation

See [Store Interface][store_interface] and [Default Store][default_store] for its implementation.

## References

[1] [Store Interface][store_interface]

[2] [Default Store][default_store]

[3] [Full Node Store Initialization][full_node_store_initialization]

[4] [Block Manager][block manager]

[5] [Full Client][full client]

[6] [Badger DB][BadgerDB]

[7] [Go Datastore][go-datastore]

[8] [Key Value Store][kv.go]

[9] [Serialization][serialization]

[store_interface]: https://github.com/rollkit/rollkit/blob/main/store/types.go#L11
[default_store]: https://github.com/rollkit/rollkit/blob/main/store/store.go
[full_node_store_initialization]: https://github.com/rollkit/rollkit/blob/main/node/full.go#L106
[block manager]: https://github.com/rollkit/rollkit/blob/main/block/manager.go
[full client]: https://github.com/rollkit/rollkit/blob/main/node/full_client.go
[BadgerDB]: https://github.com/dgraph-io/badger
[go-datastore]: https://github.com/ipfs/go-datastore
[kv.go]: https://github.com/rollkit/rollkit/blob/main/store/kv.go
[serialization]: https://github.com/rollkit/rollkit/blob/main/types/serialization.go
