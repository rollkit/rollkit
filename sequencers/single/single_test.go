package single

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
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
	seq, err := NewSequencer(context.Background(), log.NewNopLogger(), db, dummyDA, []byte("namespace"), []byte("rollup1"), 10*time.Second, metrics)
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

	if seq.bq == nil {
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
	seq, err := NewSequencer(context.Background(), log.NewNopLogger(), db, dummyDA, []byte("namespace"), []byte("rollup1"), 10*time.Second, metrics)
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
	nextBatchresp, err := seq.GetNextBatch(context.Background(), coresequencer.GetNextBatchRequest{RollupId: rollupId, LastBatchHash: nil})
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
		bq:          NewBatchQueue(db, "pending"),
		sbq:         NewBatchQueue(db, "submitted"),
		seenBatches: sync.Map{},
		rollupId:    []byte("rollup"),
	}
	defer func() {
		err := db.Close()
		if err != nil {
			t.Fatalf("Failed to close sequencer: %v", err)
		}
	}()

	// Test case where lastBatchHash and seq.lastBatchHash are both nil
	res, err := seq.GetNextBatch(context.Background(), coresequencer.GetNextBatchRequest{RollupId: seq.rollupId, LastBatchHash: nil})
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
		bq:          NewBatchQueue(db, "pending"),
		sbq:         NewBatchQueue(db, "submitted"),
		seenBatches: sync.Map{},
		rollupId:    []byte("rollup"),
	}
	defer func() {
		err := db.Close()
		if err != nil {
			t.Fatalf("Failed to close sequencer: %v", err)
		}
	}()

	// Add mock batch to the BatchQueue
	err := seq.sbq.AddBatch(context.Background(), *mockBatch)
	if err != nil {
		t.Fatalf("Failed to add batch: %v", err)
	}

	// Test success case with no previous lastBatchHash
	res, err := seq.GetNextBatch(context.Background(), coresequencer.GetNextBatchRequest{RollupId: seq.rollupId, LastBatchHash: nil})
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
	// Ensure the batch hash was added to seenBatches
	_, exists := seq.seenBatches.Load(hex.EncodeToString(batchHash))
	if !exists {
		t.Fatal("Expected seenBatches to not be empty")
	}
}

func TestSequencer_VerifyBatch(t *testing.T) {
	db := ds.NewMapDatastore()
	// Initialize a new sequencer with a seen batch
	seq := &Sequencer{
		bq:          NewBatchQueue(db, "pending"),
		sbq:         NewBatchQueue(db, "submitted"),
		seenBatches: sync.Map{},
		rollupId:    []byte("rollup"),
	}
	defer func() {
		err := db.Close()
		if err != nil {
			t.Fatalf("Failed to close sequencer: %v", err)
		}
	}()

	// Simulate adding a batch hash
	batchHash := []byte("validHash")
	seq.seenBatches.Store(hex.EncodeToString(batchHash), struct{}{})

	// Test that VerifyBatch returns true for an existing batch
	res, err := seq.VerifyBatch(context.Background(), coresequencer.VerifyBatchRequest{RollupId: seq.rollupId, BatchHash: batchHash})
	if err != nil {
		t.Fatalf("Failed to verify batch: %v", err)
	}
	if !res.Status {
		t.Fatal("Expected status to be true for valid batch hash")
	}

	// Test that VerifyBatch returns false for a non-existing batch
	res, err = seq.VerifyBatch(context.Background(), coresequencer.VerifyBatchRequest{RollupId: seq.rollupId, BatchHash: []byte("invalidHash")})
	if err != nil {
		t.Fatalf("Failed to verify batch: %v", err)
	}
	if res.Status {
		t.Fatal("Expected status to be false for invalid batch hash")
	}
}
