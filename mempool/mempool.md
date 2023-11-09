# Mempool

## Abstract

The mempool module stores transactions which have not yet been included in a block, and provides an interface to check the validity of incoming transactions. It's defined by an interface [here](https://github.com/rollkit/rollkit/blob/main/mempool/mempool.go#L16), with an implementation [here](https://github.com/rollkit/rollkit/blob/main/mempool/v1/mempool.go).

## Component Description

Full nodes instantiate a mempool [here](https://github.com/rollkit/rollkit/blob/main/node/full.go#L133). A `p2p.GossipValidator` is constructed from the node's mempool [here](https://github.com/rollkit/rollkit/blob/main/node/full.go#L391), which is used by Rollkit's P2P code to deal with peers who gossip invalid transactions. The mempool is also passed into the [block manager constructor](https://github.com/rollkit/rollkit/blob/main/node/full.go#L136), which creates a [`BlockExecutor`](https://github.com/rollkit/rollkit/blob/main/block/manager.go#L143) from the mempool.

The [`BlockExecutor`](https://github.com/rollkit/rollkit/blob/main/state/block-executor.md) calls `ReapMaxBytesMaxGas` in [`CreateBlock`](https://github.com/rollkit/rollkit/blob/main/state/executor.go#L91) to get transactions from the pool for the new block. When `commit` is called, the `BlockExecutor` calls [`Update(...)`](https://github.com/rollkit/rollkit/blob/main/state/executor.go#L255) on the mempool, removing the old transactions from the pool.

## Communication

Several RPC methods query the mempool module: [`BroadcastTxCommit`](https://github.com/rollkit/rollkit/blob/main/node/full_client.go#L128), [`BroadcastTxAsync`](https://github.com/rollkit/rollkit/blob/main/node/full_client.go#L190), [`BroadcastTxSync`](https://github.com/rollkit/rollkit/blob/main/node/full_client.go#L207) call the mempool's `CheckTx(...)` method.

## Interface

| Function Name       | Input Arguments                              | Output Type      | Intended Behavior                                                |
|---------------------|---------------------------------------------|------------------|------------------------------------------------------------------|
| CheckTx             | tx types.Tx, callback func(*abci.Response), txInfo TxInfo | error            | Executes a new transaction against the application to determine its validity and whether it should be added to the mempool. |
| RemoveTxByKey       | txKey types.TxKey                           | error            | Removes a transaction, identified by its key, from the mempool. |
| ReapMaxBytesMaxGas  | maxBytes, maxGas int64                      | types.Txs         | Reaps transactions from the mempool up to maxBytes bytes total with the condition that the total gasWanted must be less than maxGas. If both maxes are negative, there is no cap on the size of all returned transactions (~ all available transactions). |
| ReapMaxTxs          | max int                                       | types.Txs         | Reaps up to max transactions from the mempool. If max is negative, there is no cap on the size of all returned transactions (~ all available transactions). |
| Lock                | N/A                                         | N/A              | Locks the mempool. The consensus must be able to hold the lock to safely update. |
| Unlock              | N/A                                         | N/A              | Unlocks the mempool. |
| Update              | blockHeight uint64, blockTxs types.Txs, deliverTxResponses []*abci.ResponseDeliverTx, newPreFn PreCheckFunc, newPostFn PostCheckFunc | error            | Informs the mempool that the given txs were committed and can be discarded. This should be called *after* block is committed by consensus. Lock/Unlock must be managed by the caller. |
| FlushAppConn        | N/A                                         | error            | Flushes the mempool connection to ensure async callback calls are done, e.g., from CheckTx. Lock/Unlock must be managed by the caller. |
| Flush               | N/A                                         | N/A              | Removes all transactions from the mempool and caches. |
| TxsAvailable        | N/A                                         | <-chan struct{}   | Returns a channel which fires once for every height when transactions are available in the mempool. The returned channel may be nil if EnableTxsAvailable was not called. |
| EnableTxsAvailable  | N/A                                         | N/A              | Initializes the TxsAvailable channel, ensuring it will trigger once every height when transactions are available. |
| Size                | N/A                                         | int              | Returns the number of transactions in the mempool. |
| SizeBytes           | N/A                                         | int64            | Returns the total size of all txs in the mempool. |
