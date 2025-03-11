# ADR 010: Sequencing

## Changelog

- 2025-03-11: Initial draft

## Context

This ADR introduces a couple of changes to the sequencing interface to accommodate both the centralized sequencer and the based sequencer. The changes also makes it clear how the header producer and the full nodes utilize the sequencing functionalities. The current sequencing interface as as defined below.

```
// Sequencer is a generic interface for a rollup sequencer
type Sequencer interface {
	SequencerInput
	SequencerOutput
	BatchVerifier
}

// SequencerInput provides a method for submitting a transaction from rollup to sequencer
type SequencerInput interface {
	// SubmitRollupTransaction submits a transaction from rollup to sequencer
	// RollupId is the unique identifier for the rollup chain
	// Tx is the transaction to submit
	// returns an error if any from the sequencer
	SubmitRollupTransaction(ctx context.Context, req SubmitRollupTransactionRequest) (*SubmitRollupTransactionResponse, error)
}

// SequencerOutput provides a method for getting the next batch of transactions from sequencer to rollup
type SequencerOutput interface {
	// GetNextBatch returns the next batch of transactions from sequencer to rollup
	// RollupId is the unique identifier for the rollup chain
	// LastBatchHash is the cryptographic hash of the last batch received by the rollup
	// MaxBytes is the maximum number of bytes to return in the batch
	// returns the next batch of transactions and an error if any from the sequencer
	GetNextBatch(ctx context.Context, req GetNextBatchRequest) (*GetNextBatchResponse, error)
}

// BatchVerifier provides a method for verifying a batch of transactions received from the sequencer
type BatchVerifier interface {
	// VerifyBatch verifies a batch of transactions received from the sequencer
	// RollupId is the unique identifier for the rollup chain
	// BatchHash is the cryptographic hash of the batch to verify
	// returns a boolean indicating if the batch is valid and an error if any from the sequencer
	VerifyBatch(ctx context.Context, req VerifyBatchRequest) (*VerifyBatchResponse, error)
}

// Batch is a collection of transactions
type Batch struct {
	Transactions [][]byte
}

// SubmitRollupTransactionRequest is a request to submit a transaction from rollup to sequencer
type SubmitRollupTransactionRequest struct {
	RollupId []byte
	Tx       []byte
}

// SubmitRollupTransactionResponse is a response to submitting a transaction from rollup to sequencer
type SubmitRollupTransactionResponse struct {
}

// GetNextBatchRequest is a request to get the next batch of transactions from sequencer to rollup
type GetNextBatchRequest struct {
	RollupId      []byte
	LastBatchHash []byte
	MaxBytes      uint64
}

// GetNextBatchResponse is a response to getting the next batch of transactions from sequencer to rollup
type GetNextBatchResponse struct {
	Batch     *Batch
	Timestamp time.Time
}

// VerifyBatchRequest is a request to verify a batch of transactions received from the sequencer
type VerifyBatchRequest struct {
	RollupId  []byte
	BatchHash []byte
}

// VerifyBatchResponse is a response to verifying a batch of transactions received from the sequencer
type VerifyBatchResponse struct {
	Status bool
}
```

## Alternative Approaches

The current sequencing interface lacks the following features:
* The design and interface is not clear how the rollkit node types (header producers and full nodes) are supposed to use it
* The verification (VerifyBatch) is completely omitted
* The last batch hash is not sufficient to refer to the previous batch while producing or fetching the next batch

## Decision

The proposed sequencing interface is as follows:

```
// Sequencer is a generic interface for a rollup sequencer
type Sequencer interface {
	SequencerInput
	SequencerOutput
	BatchVerifier
}

// SequencerInput provides a method for submitting a transaction from rollup to sequencer
type SequencerInput interface {
	// SubmitRollupTransactions submits a list of transactions from rollup to sequencer
	// RollupId is the unique identifier for the rollup chain
	// Tx is the transaction to submit
	// returns an error if any from the sequencer
	SubmitRollupTransactions(ctx context.Context, req SubmitRollupTransactionRequest) (*SubmitRollupTransactionResponse, error)
}

// SequencerOutput provides a method for getting the next batch of transactions from sequencer to rollup
type SequencerOutput interface {
	// GetNextBatch returns the next batch of transactions from sequencer to rollup
	// RollupId is the unique identifier for the rollup chain
	// LastBatchData is the key data related to batch that can be used to retrieve the batch or verify its existence 
	// MaxBytes is the maximum number of bytes to return in the batch
	// returns the next batch of transactions, timestamp, batch data and an error if any from the sequencer
	GetNextBatch(ctx context.Context, req GetNextBatchRequest) (*GetNextBatchResponse, error)
}

// BatchVerifier provides a method for verifying a batch of transactions received from the sequencer
type BatchVerifier interface {
	// VerifyBatch verifies a batch of transactions received from the sequencer
	// RollupId is the unique identifier for the rollup chain
	// BatchData is the key data related to batch that can be used to verify the batch
	// returns a boolean indicating if the batch is valid and an error if any from the sequencer
	VerifyBatch(ctx context.Context, req VerifyBatchRequest) (*VerifyBatchResponse, error)
}

// Batch is a collection of transactions
type Batch struct {
	Transactions [][]byte
}

// SubmitRollupTransactionRequest is a request to submit a list of transactions from rollup to sequencer
type SubmitRollupTransactionRequest struct {
	RollupId []byte
	Txs      [][]byte
}

// SubmitRollupTransactionResponse is a response to submitting a transaction from rollup to sequencer
type SubmitRollupTransactionResponse struct {
}

// GetNextBatchRequest is a request to get the next batch of transactions from sequencer to rollup
type GetNextBatchRequest struct {
	RollupId      []byte
	LastBatchData [][]byte
	MaxBytes      uint64
}

// GetNextBatchResponse is a response to getting the next batch of transactions from sequencer to rollup
type GetNextBatchResponse struct {
	Batch     *Batch
	Timestamp time.Time
    BatchData [][]byte
}

// VerifyBatchRequest is a request to verify a batch of transactions received from the sequencer
type VerifyBatchRequest struct {
	RollupId  []byte
	BatchData [][]byte
}

// VerifyBatchResponse is a response to verifying a batch of transactions received from the sequencer
type VerifyBatchResponse struct {
	Status bool
}
```

## Detailed Design

### Introduction of the BatchData instead of BatchHash

BatchData is any key required data that can be used to not only fetch the batch from the sequencing layer, but also using which efficiently verify that data. For instance, in Celestia this BatchData is `[]da.ID` where `da.ID` is `[]byte` that is computed by combining the `da height` and the `da commitment` for each rollup transaction blob.

Using the last BatchData, it is possible to know which da height to query next for fetching the next batch and it is also possible to efficiently verify the batch. For instance, using the BatchData (`[]da.ID`) one can get the `Proof` using `GetProofs` da interface and then invoke `Validate` using the BatchData and Proof.

```
GetProofs(ctx context.Context, ids []ID, namespace Namespace) ([]Proof, error)
Validate(ctx context.Context, ids []ID, proofs []Proof, namespace Namespace) ([]bool, error)
```

The BatchData is simply a `[][]byte` and the exact details are defined by the underlying DA implementation. 

### How different rollkit node types implement the sequencing interface

There are mainly two types of nodes in rollkit: 1) header producer and 2) full node. We now describe how the sequencing interface is implemented for these two types of nodes in the two popular sequencing designs: the centralized sequencer and the based sequencer.

#### For Centralized Sequencer

Header Producer:
* `SubmitRollupTxs([][]byte)` returns `nil`: put the txs in local queue so that batch is created and submitted to da on batchtime interval
* `GetNextBatch(lastBatchData, maxBytes)` returns `batch, timestamp, batchData`: using lastBatchData extract the last da height and search for the next height for getting the batch in the sequencer namespace
* VerifyBatch(lastBatchData): noop as the batch is self created


Full node:
* `SubmitRollupTxs`: noop 
* `GetNextBatch`: using lastBatchData find the last da height and search for next height for getting the batch in the sequencer namespace
* `VerifyBatch`: using the lastBatchData, getProofs from da and then call validate using lastBatchData and proofs.

#### For Based Sequencer

Header Producer:
* `SubmitRollupTxs([][]byte)` returns `nil`: directly post the txs to da (split to multiple blobs submission, if exceeds da maxblobsize limit)
* `GetNextBatch(lastBatchData, maxBytes)` returns `batch, timestamp, batchData`: using lastBatchData find the last da height, fetch all blobs check to see if there were any pending blobs to be included in the next batch. Also, search for next height to add more blobs until maxBytes.
* `VerifyBatch(lastBatchData)`: noop

Full node:
* `SubmitRollupTxs`: directly post the txs to da (split to multiple blobs submission, if exceeds da maxblobsize limit)
* `GetNextBatch`: using lastBatchData find the last da height, fetch all blobs check to see if there were any pending blobs to be included in the next batch. Also, search for next height to add more blobs until maxBytes.
* `VerifyBatch`: using the lastBatchData, getProofs from da and then call validate using lastBatchData and proofs.

## Status

Proposed and under implementation.

## Consequences

### Positive
* Clarity on how the sequencing interface can be utilized by the various node types in rollkit (header producer and full node)
* Covers two popular sequencing designs: the centralized and the based sequencing
* Covers both batch retrieval and efficient verification
* Possibly reduce the local storage requirements for the sequencing middlewares

### Negative

### Neutral

## References

> Are there any relevant PR comments, issues that led up to this, or articles referenced for why we made the given design choice? If so link them here!

- [go-sequencing](https://github.com/rollkit/go-sequencing)
- [go-da](https://github.com/rollkit/go-da)
