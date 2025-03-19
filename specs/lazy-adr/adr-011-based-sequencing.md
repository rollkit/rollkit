# ADR 010: Based Sequencing

## Changelog

- 2025-03-11: Initial draft

## Context

This ADR provides details regarding based sequencing using Celestia as base layer, Rollkit as rollup consensus layer, and EVM as execution layer.

### Based Sequencing

User transactions are ordered by the base layer.

### Rollkit

Is a combination of three layers:

* Sequencing layer
* Consensus layer
* Execution layer

#### Sequencing layer

* Implements sequencing interface
* Popular implementations are centralized sequencing and based sequencing

#### Consensus layer

* Two types of nodes: Proposer (aka header producer) and full node
* Proposer provides a soft-commitment (or preconfs) fetching the batch, executing it, producing the update state root, and proposing a header with updated state root and proof data for verification for later hard-confirmation. Same for both centralized and based sequencing
* Full node receives transaction batches and headers via p2p gossip (preconf). Using the proof data in the header, full node can perform full verification to hard confirm

#### Execution layer

* Implements the execution interface
* Popular implementations are evm and abci
* Mempool and RPC are part of the execution layer, hence if a user submits a rollup transaction, it gets to the mempool on the execution layer, which then pulled by the consensus layer (by calling GetTxs) to submit to sequencing layer.

### EVM

### Sequencing Interface

These methods are implemented in the sequencing layer (based-sequencer, which connects to Celestia as base layer). Rollkit node (consensus layer) is going to invoke these functions.

* SubmitRollupTxs: 
* GetNextBatch(lastBatchData):
* VerifyBatch(batchData):

### Execution Interface

These methods are implemented in the execution layer (go-execution-evm which runs reth for execution). Rollkit node (consensus layer) is going to invoke these functions.

* GetTxs: for fetching rollup mempool transactions (resides in the execution layer) and submitting to base layer.
* ExecuteTxs(txs):
* SetFinal(block):  

## Alternative Approaches


## Decision

A based sovereign evm rollup using Rollkit.

Components:
* Celestia for based sequencing
* Rollkit for consensus 
* EVM for execution (reth and op-geth to allow sequenced transaction submission)

## Detailed Design

### Transaction submission

* User can directly submit the rollup transaction to base layer. User need to compose a base layer transaction using the signed bytes of the rollup transaction. User also need to have access to base layer light node and an account for paying the base layer fee for transaction submission.
* Rollkit node (proposer or fullnode) can relay user transaction to base layer. Rollkit nodes won't gossip the transactions (to prevent duplicate submissions). Rollkit nodes will have a light node connection with a base layer account for paying the fees. 

### Transaction Processing

* The user rollup transaction can be directly submitted to base layer, hence no expectation that the transaction exists in rollup mempool. As part of the execute transactions, the transaction needs to be added to the rollup mempool.

### How the various rollkit nodes implement the sequencing interface?

Header Producer (aka. Proposer):
* `SubmitRollupTxs([][]byte)` returns `nil`: directly post the txs to da (split to multiple blobs submission, if exceeds da maxblobsize limit)
* `GetNextBatch(lastBatchData, maxBytes)` returns `batch, timestamp, batchData`: using lastBatchData find the last da height, fetch all blobs check to see if there were any pending blobs to be included in the next batch. Also, search for next height to add more blobs until maxBytes.
* `VerifyBatch(lastBatchData)`: noop

Full node:
* `SubmitRollupTxs`: directly post the txs to da (split to multiple blobs submission, if exceeds da maxblobsize limit)
* `GetNextBatch`: using lastBatchData find the last da height, fetch all blobs check to see if there were any pending blobs to be included in the next batch. Also, search for next height to add more blobs until maxBytes.
* `VerifyBatch`: using the batchData, getProofs from da and then call validate using batchData and proofs.

For convenience, the proposer can front-run the batch creation (pulling the sequenced transaction from base layer) for execution. The proposer can p2p gossip the batch along with the corresponding header (that contains the state root update and batch inclusion proof data) for full nodes to quickly process the batched transactions. However, the full nodes can run both GetNextBatch and VerifyBatch in the background to mark the transactions (or batch/header) as final. This will prevent the proposer from maliciously behaving (skipping sequenced txs or including more txs).

## Status

Proposed and under implementation.

## Consequences

### Positive

### Negative

### Neutral

## References

> Are there any relevant PR comments, issues that led up to this, or articles referenced for why we made the given design choice? If so link them here!

- [go-sequencing](https://github.com/rollkit/go-sequencing)
- [go-da](https://github.com/rollkit/go-da)
