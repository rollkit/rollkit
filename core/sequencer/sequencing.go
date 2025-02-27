package sequencer

import (
	"context"
	"time"
)

// Sequencer is a generic interface for a rollup sequencer
type Sequencer interface {
	SequencerInput
	SequencerOutput
	BatchVerifier
}

// SequencerInput provides a method for submitting a transaction from rollup to sequencer
type SequencerInput interface {
	// SubmitRollupBatchTxs submits a batch of transactions from rollup to sequencer
	// RollupId is the unique identifier for the rollup chain
	// Batch is the batch of transactions to submit
	// returns an error if any from the sequencer
	SubmitRollupBatchTxs(ctx context.Context, req SubmitRollupBatchTxsRequest) (*SubmitRollupBatchTxsResponse, error)
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

// SubmitRollupBatchTxsRequest is a request to submit a batch of transactions from rollup to sequencer
type SubmitRollupBatchTxsRequest struct {
	RollupId []byte
	Batch    *Batch
}

// SubmitRollupBatchTxsResponse is a response to submitting a batch of transactions from rollup to sequencer
type SubmitRollupBatchTxsResponse struct {
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
