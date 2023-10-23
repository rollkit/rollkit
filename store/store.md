# Store

## Abstract

The Store interface defines methods for storing and retrieving blocks, commits, and the state of the blockchain.

## Protocol/Component Description

The Store interface defines the following methods:

- `Height`: Returns the height of the highest block in the store.
- `SetHeight`: Sets the height saved in the Store if it is higher than the existing height.
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

An implementation of the Store interface is the `DefaultStore` struct which uses a `TxnDatastore` interface for its underlying storage. The `TxnDatastore` interface has two key-value store implementations:

- `NewDefaultInMemoryKVStore`: Builds a key-value store that uses the BadgerDB library and operates in-memory, without accessing the disk. Used only across unit tests and integration tests.

- `NewDefaultKVStore`: Builds a key-value store that uses the BadgerDB library and stores the data on disk at the specified path.

A Rollkit full node is [initialized][full_node_store_initialization] using the `DefaultStore` struct with `NewDefaultKVStore` for underlying storage.

The store implementation is most widely used inside the [block manager] to perform its functions correctly. Within the block manager, it is protected by a mutex lock, `lastStateMtx`, to synchronize all access to it and prevent race conditions.

## Message Structure/Communication Format

The Store does not communicate over the network, so there is no message structure or communication format.

## Assumptions and Considerations

The Store assumes that the underlying datastore is reliable and provides atomicity for transactions. It also assumes that the data passed to it for storage is valid and correctly formatted.

## Implementation

See [Store Interface][store_interface] and [Default Store][default_store] for its implementation.

## References

[1] [Store Interace][store_interface]

[2] [Default Store][default_store]

[3] [Full Node Store Initialization][full_node_store_initialization]

[4] [Block Manager][block manager]

[store_interface]: https://github.com/rollkit/rollkit/blob/main/store/types.go#L11
[default_store]: https://github.com/rollkit/rollkit/blob/main/store/store.go
[full_node_store_initialization]: https://github.com/rollkit/rollkit/blob/main/node/full.go#L106
[block manager]: https://github.com/rollkit/rollkit/blob/main/block/manager.go
