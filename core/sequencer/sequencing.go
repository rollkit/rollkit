package sequencer

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"time"
)

// Sequencer is a generic interface for a rollup sequencer
type Sequencer interface {
	// SubmitRollupBatchTxs submits a batch of transactions from rollup to sequencer
	// RollupId is the unique identifier for the rollup chain
	// Batch is the batch of transactions to submit
	// returns an error if any from the sequencer
	SubmitBatchTxs(ctx context.Context, req SubmitBatchTxsRequest) (*SubmitBatchTxsResponse, error)

	// GetNextBatch returns the next batch of transactions from sequencer to rollup
	// RollupId is the unique identifier for the rollup chain
	// LastBatchHash is the cryptographic hash of the last batch received by the rollup
	// MaxBytes is the maximum number of bytes to return in the batch
	// returns the next batch of transactions and an error if any from the sequencer
	GetNextBatch(ctx context.Context, req GetNextBatchRequest) (*GetNextBatchResponse, error)

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

// Hash returns the cryptographic hash of the batch
func (batch *Batch) Hash() ([]byte, error) {
	// Create a new hash instance
	hasher := sha256.New()

	// If batch is nil or has no transactions, hash an empty slice
	if batch == nil || len(batch.Transactions) == 0 {
		hasher.Write([]byte{})
		return hasher.Sum(nil), nil
	}

	// Write the number of transactions as a fixed-size value
	txCount := make([]byte, 8) // 8 bytes for uint64
	binary.BigEndian.PutUint64(txCount, uint64(len(batch.Transactions)))
	hasher.Write(txCount)

	// Hash each transaction and write to the hasher
	for _, tx := range batch.Transactions {
		// Write transaction length as fixed-size value
		txLen := make([]byte, 8) // 8 bytes for uint64
		binary.BigEndian.PutUint64(txLen, uint64(len(tx)))
		hasher.Write(txLen)

		// Write the transaction bytes
		hasher.Write(tx)
	}

	return hasher.Sum(nil), nil
}

// SubmitBatchTxsRequest is a request to submit a batch of transactions from chain to sequencer
type SubmitBatchTxsRequest struct {
	Id    []byte
	Batch *Batch
}

// SubmitBatchTxsResponse is a response to submitting a batch of transactions from chain to sequencer
type SubmitBatchTxsResponse struct {
}

// GetNextBatchRequest is a request to get the next batch of transactions from sequencer to chain
type GetNextBatchRequest struct {
	Id            []byte
	LastBatchData [][]byte
	MaxBytes      uint64
}

// GetNextBatchResponse is a response to getting the next batch of transactions from sequencer to chain
type GetNextBatchResponse struct {
	Batch     *Batch
	Timestamp time.Time
	BatchData [][]byte
}

// VerifyBatchRequest is a request to verify a batch of transactions received from the sequencer
type VerifyBatchRequest struct {
	Id        []byte
	BatchData [][]byte
}

// VerifyBatchResponse is a response to verifying a batch of transactions received from the sequencer
type VerifyBatchResponse struct {
	Status bool
}
