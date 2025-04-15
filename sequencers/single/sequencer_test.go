package single

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"cosmossdk.io/log"
	ds "github.com/ipfs/go-datastore"

	coreda "github.com/rollkit/rollkit/core/da"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
)

func TestNewSequencer(t *testing.T) {
	// Create a new sequencer with mock DA client
	dummyDA := coreda.NewDummyDA(100_000_000, 0, 0)
	metrics, _ := NopMetrics()
	db := ds.NewMapDatastore()
	seq, err := NewSequencer(log.NewNopLogger(), db, dummyDA, []byte("namespace"), []byte("rollup1"), 10*time.Second, metrics, false)
	if err != nil {
		t.Fatalf("Failed to create sequencer: %v", err)
	}
	defer func() {
		err := db.Close()
		if err != nil {
			t.Fatalf("Failed to close sequencer: %v", err)
		}
	}()

	// Check if the sequencer was created with the correct values
	if seq == nil {
		t.Fatal("Expected sequencer to not be nil")
	}

	if seq.queue == nil {
		t.Fatal("Expected batch queue to not be nil")
	}
	if seq.dalc == nil {
		t.Fatal("Expected DA client to not be nil")
	}
}

func TestSequencer_SubmitRollupBatchTxs(t *testing.T) {
	// Initialize a new sequencer
	metrics, _ := NopMetrics()
	dummyDA := coreda.NewDummyDA(100_000_000, 0, 0)
	db := ds.NewMapDatastore()
	seq, err := NewSequencer(log.NewNopLogger(), db, dummyDA, []byte("namespace"), []byte("rollup1"), 10*time.Second, metrics, false)
	if err != nil {
		t.Fatalf("Failed to create sequencer: %v", err)
	}
	defer func() {
		err := db.Close()
		if err != nil {
			t.Fatalf("Failed to close sequencer: %v", err)
		}
	}()

	// Test with initial rollup ID
	rollupId := []byte("rollup1")
	tx := []byte("transaction1")

	res, err := seq.SubmitRollupBatchTxs(context.Background(), coresequencer.SubmitRollupBatchTxsRequest{RollupId: rollupId, Batch: &coresequencer.Batch{Transactions: [][]byte{tx}}})
	if err != nil {
		t.Fatalf("Failed to submit rollup transaction: %v", err)
	}
	if res == nil {
		t.Fatal("Expected response to not be nil")
	}

	// Wait for the transaction to be processed
	time.Sleep(2 * time.Second)

	// Verify the transaction was added
	nextBatchresp, err := seq.GetNextBatch(context.Background(), coresequencer.GetNextBatchRequest{RollupId: rollupId})
	if err != nil {
		t.Fatalf("Failed to get next batch: %v", err)
	}
	if len(nextBatchresp.Batch.Transactions) != 1 {
		t.Fatalf("Expected 1 transaction, got %d", len(nextBatchresp.Batch.Transactions))
	}

	// Test with a different rollup ID (expecting an error due to mismatch)
	res, err = seq.SubmitRollupBatchTxs(context.Background(), coresequencer.SubmitRollupBatchTxsRequest{RollupId: []byte("rollup2"), Batch: &coresequencer.Batch{Transactions: [][]byte{tx}}})
	if err == nil {
		t.Fatal("Expected error for invalid rollup ID, got nil")
	}
	if !errors.Is(err, ErrInvalidRollupId) {
		t.Fatalf("Expected ErrInvalidRollupId, got %v", err)
	}
	if res != nil {
		t.Fatal("Expected nil response for error case")
	}
}

func TestSequencer_GetNextBatch_NoLastBatch(t *testing.T) {
	db := ds.NewMapDatastore()

	seq := &Sequencer{
		queue:    NewBatchQueue(db, "batches"),
		rollupId: []byte("rollup"),
	}
	defer func() {
		err := db.Close()
		if err != nil {
			t.Fatalf("Failed to close sequencer: %v", err)
		}
	}()

	// Test case where lastBatchHash and seq.lastBatchHash are both nil
	res, err := seq.GetNextBatch(context.Background(), coresequencer.GetNextBatchRequest{RollupId: seq.rollupId})
	if err != nil {
		t.Fatalf("Failed to get next batch: %v", err)
	}

	// Ensure the time is approximately the same
	if res.Timestamp.Day() != time.Now().Day() {
		t.Fatalf("Expected timestamp day to be %d, got %d", time.Now().Day(), res.Timestamp.Day())
	}

	// Should return an empty batch
	if len(res.Batch.Transactions) != 0 {
		t.Fatalf("Expected empty batch, got %d transactions", len(res.Batch.Transactions))
	}
}

func TestSequencer_GetNextBatch_Success(t *testing.T) {
	// Initialize a new sequencer with a mock batch
	mockBatch := &coresequencer.Batch{Transactions: [][]byte{[]byte("tx1"), []byte("tx2")}}

	db := ds.NewMapDatastore()

	seq := &Sequencer{
		logger:           log.NewNopLogger(),
		queue:            NewBatchQueue(db, "batches"),
		daSubmissionChan: make(chan coresequencer.Batch, 100),
		rollupId:         []byte("rollup"),
	}
	defer func() {
		err := db.Close()
		if err != nil {
			t.Fatalf("Failed to close sequencer: %v", err)
		}
	}()

	// Add mock batch to the BatchQueue
	err := seq.queue.AddBatch(context.Background(), *mockBatch)
	if err != nil {
		t.Fatalf("Failed to add batch: %v", err)
	}

	// Test success case with no previous lastBatchHash
	res, err := seq.GetNextBatch(context.Background(), coresequencer.GetNextBatchRequest{RollupId: seq.rollupId})
	if err != nil {
		t.Fatalf("Failed to get next batch: %v", err)
	}

	// Ensure the time is approximately the same
	if res.Timestamp.Day() != time.Now().Day() {
		t.Fatalf("Expected timestamp day to be %d, got %d", time.Now().Day(), res.Timestamp.Day())
	}

	fmt.Println("res.Batch.Transactions", res.Batch)
	// Ensure that the transactions are present
	if len(res.Batch.Transactions) != 2 {
		t.Fatalf("Expected 2 transactions, got %d", len(res.Batch.Transactions))
	}

	batchHash, err := mockBatch.Hash()
	if err != nil {
		t.Fatalf("Failed to get batch hash: %v", err)
	}
	if len(batchHash) == 0 {
		t.Fatal("Expected batch hash to not be empty")
	}
}

func TestSequencer_VerifyBatch(t *testing.T) {
	db := ds.NewMapDatastore()
	// Initialize a new sequencer with a seen batch
	seq := &Sequencer{
		logger:           log.NewNopLogger(),
		queue:            NewBatchQueue(db, "batches"),
		daSubmissionChan: make(chan coresequencer.Batch, 100),
		rollupId:         []byte("rollup"),
		proposer:         true,
	}
	defer func() {
		err := db.Close()
		if err != nil {
			t.Fatalf("Failed to close sequencer: %v", err)
		}
	}()

	// Simulate adding a batch hash
	batchHash := []byte("validHash")

	// Test that VerifyBatch returns true for an existing batch
	res, err := seq.VerifyBatch(context.Background(), coresequencer.VerifyBatchRequest{RollupId: seq.rollupId, BatchData: [][]byte{batchHash}})
	if err != nil {
		t.Fatalf("Failed to verify batch: %v", err)
	}
	if !res.Status {
		t.Fatal("Expected status to be true for valid batch hash")
	}

	// Test that VerifyBatch returns false for a non-existing batch
	res, err = seq.VerifyBatch(context.Background(), coresequencer.VerifyBatchRequest{RollupId: seq.rollupId, BatchData: [][]byte{[]byte("invalidHash")}})
	if err != nil {
		t.Fatalf("Failed to verify batch: %v", err)
	}
	if res.Status {
		t.Fatal("Expected status to be false for invalid batch hash")
	}
}
