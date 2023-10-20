# Indexer Service

## Abstract

The Indexer service indexes transactions and blocks using events emitted by the block executor that can later be used to build services like block explorers that ingest this data.

## Protocol/Component Description

The Indexer service consists of three main components: an event bus, a transaction indexer and a block indexer.

### Event Bus

The event bus is a messaging service between the full node and the indexer service. It serves events emitted by the block executor of the full node to the indexer service where events are routed to the relevant indexer component based on type.

### Block Indexer

The [Block Indexer][block_indexer] indexes BeginBlock and EndBlock events with an underlying KV store. Block events are indexed by their height, such that matching search criteria returns the respective block height(s).

### Transaction Indexer

The [Transaction Indexer][tx_indexer] is a key-value store-backed indexer that provides functionalities for indexing and searching transactions. It allows for the addition of a batch of transactions, indexing and storing a single transaction, retrieving a transaction specified by hash, and querying for transactions based on specific conditions. The indexer also supports range queries and can return results based on the intersection of multiple conditions.

## Message Structure/Communication Format

The `publishEvents` [method][publish_events_method] in the block executor sends several types of messages through the event bus, which are then received by `indexer_service.go`. These messages are:

1. `EventNewBlock`: This event is published when a new block is created. It contains the block data and the results of the `BeginBlock` and `EndBlock` ABCI methods.

2. `EventNewBlockHeader`: This event is published along with the `EventNewBlock` event. It contains the block header data, the number of transactions in the block, and the results of the `BeginBlock` and `EndBlock` ABCI methods.

3. `EventNewEvidence`: This event is published for each piece of evidence in the block. It contains the evidence and the height of the block.

4. `EventTx`: This event is published for each transaction in the block. It contains the result of the `DeliverTx` ABCI method for the transaction.

These events are subscribed to in the `OnStart` method of `indexer_service.go`, and are used to index the transactions and blocks.

## Assumptions and Considerations

The indexer service assumes that the messages passed by the block executor are valid block headers and valid transactions with the required fields such that they can be indexed by the respective block indexer and transaction indexer.

## Implementation

See [indexer service]

## References

[1] [Block Indexer][block_indexer]

[2] [Transaction Indexer][tx_indexer]

[3] [Publish Events][publish_events_method]

[4] [Indexer Service][indexer service]

[block_indexer]: https://github.com/rollkit/rollkit/blob/e3218bb78e06aa49e2fde57ec9b96cc2cdfc071e/state/indexer/block.go#L11
[tx_indexer]: https://github.com/rollkit/rollkit/blob/e3218bb78e06aa49e2fde57ec9b96cc2cdfc071e/state/txindex/indexer.go#L14
[publish_events_method]: https://github.com/rollkit/rollkit/blob/e3218bb78e06aa49e2fde57ec9b96cc2cdfc071e/state/executor.go#L352
[indexer service]: https://github.com/rollkit/rollkit/blob/e3218bb78e06aa49e2fde57ec9b96cc2cdfc071e/state/txindex/indexer_service.go#L20
