# Mempool

## Abstract

The mempool module stores transactions which have not yet been included in a block, and provides an interface to check the validity of incoming transactions. It's defined by an interface [here](https://github.com/rollkit/rollkit/blob/main/mempool/mempool.go#L16), with an implementation [here](https://github.com/rollkit/rollkit/blob/main/mempool/v1/mempool.go).

## Component Description

Full nodes instantiate a mempool [here](https://github.com/rollkit/rollkit/blob/main/node/full.go#L133). A `p2p.GossipValidator` is constructed from the node's mempool [here](https://github.com/rollkit/rollkit/blob/main/node/full.go#L391), which is used by Rollkit's p2p code to deal with peers who gossip invalid transactions. The mempool is also passed into the [block manager constructor](https://github.com/rollkit/rollkit/blob/main/node/full.go#L136), which creates a [`BlockExecutor`](https://github.com/rollkit/rollkit/blob/main/block/manager.go#L143) from the mempool.

The [`BlockExecutor`](../specs/src/specs/state.md) calls `ReapMaxBytesMaxGas` in [`CreateBlock`](https://github.com/rollkit/rollkit/blob/main/state/executor.go#L91) to get transactions from the pool for the new block. When `commit` is called, the `BlockExecutor` calls [`Update(...)`](https://github.com/rollkit/rollkit/blob/main/state/executor.go#L255) on the mempool, removing the old transactions from the pool.

## Communication

Several RPC methods query the mempool module: [`BroadcastTxCommit`](https://github.com/rollkit/rollkit/blob/main/node/full_client.go#L128), [`BroadcastTxAsync`](https://github.com/rollkit/rollkit/blob/main/node/full_client.go#L190), [`BroadcastTxSync`](https://github.com/rollkit/rollkit/blob/main/node/full_client.go#L207) call the mempool's `CheckTx(...)` method.

## Interface

| **Method Name**    | **Inputs** | **Outputs** | **Behavior** |
| ------------------ | ---------- | ----------- | ------------ |
| CheckTx            | Takes a transaction object           |             |              |
| RemoveTxByKey      |            |             |              |
| ReapMaxBytesMaxGas |            |             |              |
| ReapMaxTxs         |            |             |              |
| Lock               |            |             |              |
| Unlock             |            |             |              |
| Update             |            |             |              |
| FlushAppConn       |            |             |              |
| Flush              |            |             |              |
| TxsAvailable       |            |             |              |
| EnableTxsAvailable |            |             |              |
| Size               |            |             |              |
| SizeBytes          |            |             |              |