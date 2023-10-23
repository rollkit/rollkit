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

The [`publishEvents` method][publish_events_method] in the block executor is responsible for broadcasting several types of events through the event bus. These events include `EventNewBlock`, `EventNewBlockHeader`, `EventNewBlockEvents`, `EventNewEvidence`, and `EventTx`. Each of these events carries specific data related to the block or transaction they represent.

1. `EventNewBlock`: Triggered when a new block is finalized. It carries the block data along with the results of the `FinalizeBlock` ABCI method.

2. `EventNewBlockHeader`: Triggered alongside the `EventNewBlock` event. It carries the block header data.

3. `EventNewBlockEvents`: Triggered when a new block is finalized. It carries the block height, the events associated with the block, and the number of transactions in the block.

4. `EventNewEvidence`: Triggered for each piece of evidence in the block. It carries the evidence and the height of the block.

5. `EventTx`: Triggered for each transaction in the block. It carries the result of the `DeliverTx` ABCI method for the transaction.

The `OnStart` method in `indexer_service.go` subscribes to these events. It listens for new blocks and transactions, and upon receiving these events, it indexes the transactions and blocks accordingly. The block indexer indexes `EventNewBlockEvents`, while the transaction indexer indexes the events inside `EventTx`. The events, `EventNewBlock`, `EventNewBlockHeader`, and `EventNewEvidence` are not currently used by the indexer service.

## Assumptions and Considerations

The indexer service assumes that the messages passed by the block executor are valid block headers and valid transactions with the required fields such that they can be indexed by the respective block indexer and transaction indexer.

## Implementation

See [indexer service]

## References

[1] [Block Indexer][block_indexer]

[2] [Transaction Indexer][tx_indexer]

[3] [Publish Events][publish_events_method]

[4] [Indexer Service][indexer service]

[block_indexer]: https://github.com/rollkit/rollkit/blob/main/state/indexer/block.go#L11
[tx_indexer]: https://github.com/rollkit/rollkit/blob/main/state/txindex/indexer.go#L14
[publish_events_method]: https://github.com/rollkit/rollkit/blob/main/state/executor.go#L352
[indexer service]: https://github.com/rollkit/rollkit/blob/main/state/txindex/indexer_service.go#L20
