package centralized

import (
	"bytes"
	"context"
	"encoding/binary"
	"sync"
	"testing"

	ds "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
	dssync "github.com/ipfs/go-datastore/sync"

	coresequencer "github.com/rollkit/rollkit/core/sequencer"
)

// createTestBatch creates a batch with dummy transactions for testing
func createTestBatch(t *testing.T, txCount int) coresequencer.Batch {
	txs := make([][]byte, txCount)
	for i := 0; i < txCount; i++ {
		txs[i] = []byte{byte(i), byte(i + 1), byte(i + 2)}
	}
	return coresequencer.Batch{Transactions: txs}
}

func setupTestQueue(t *testing.T) *BatchQueue {
	// Create an in-memory thread-safe datastore
	memdb := dssync.MutexWrap(ds.NewMapDatastore())
	return NewBatchQueue(memdb)
}

func TestNewBatchQueue(t *testing.T) {
	tests := []struct {
		name           string
		expectQueueLen int
	}{
		{
			name:           "queue initialization",
			expectQueueLen: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bq := setupTestQueue(t)
			if bq == nil {
				t.Fatal("expected non-nil BatchQueue")
			}
			if len(bq.queue) != tc.expectQueueLen {
				t.Errorf("expected queue length %d, got %d", tc.expectQueueLen, len(bq.queue))
			}
			if bq.db == nil {
				t.Fatal("expected non-nil db")
			}
		})
	}
}

func TestAddBatch(t *testing.T) {
	tests := []struct {
		name           string
		batchesToAdd   []int // Number of transactions in each batch
		expectQueueLen int
		expectErr      bool
	}{
		{
			name:           "add single batch",
			batchesToAdd:   []int{3},
			expectQueueLen: 1,
			expectErr:      false,
		},
		{
			name:           "add multiple batches",
			batchesToAdd:   []int{1, 2, 3},
			expectQueueLen: 3,
			expectErr:      false,
		},
		{
			name:           "add empty batch",
			batchesToAdd:   []int{0},
			expectQueueLen: 1,
			expectErr:      false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bq := setupTestQueue(t)
			ctx := context.Background()

			// Add batches
			for _, txCount := range tc.batchesToAdd {
				batch := createTestBatch(t, txCount)
				err := bq.AddBatch(ctx, batch)
				if tc.expectErr && err == nil {
					t.Error("expected error but got none")
				}
				if !tc.expectErr && err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}

			// Verify queue length
			if len(bq.queue) != tc.expectQueueLen {
				t.Errorf("expected queue length %d, got %d", tc.expectQueueLen, len(bq.queue))
			}

			// Verify batches were stored in datastore
			results, err := bq.db.Query(ctx, query.Query{})
			if err != nil {
				t.Fatalf("unexpected error querying datastore: %v", err)
			}
			defer results.Close()

			count := 0
			for range results.Next() {
				count++
			}
			if count != tc.expectQueueLen {
				t.Errorf("expected datastore count %d, got %d", tc.expectQueueLen, count)
			}
		})
	}
}

func TestNextBatch(t *testing.T) {
	tests := []struct {
		name          string
		batchesToAdd  []int
		callNextCount int
		expectEmptyAt int // At which call to Next() we expect an empty batch
		expectErrors  []bool
	}{
		{
			name:          "get single batch from non-empty queue",
			batchesToAdd:  []int{3},
			callNextCount: 2, // Call Next twice (second should return empty)
			expectEmptyAt: 1, // Second call (index 1) should be empty
			expectErrors:  []bool{false, false},
		},
		{
			name:          "get batches from queue in order",
			batchesToAdd:  []int{1, 2, 3},
			callNextCount: 4, // Call Next four times
			expectEmptyAt: 3, // Fourth call (index 3) should be empty
			expectErrors:  []bool{false, false, false, false},
		},
		{
			name:          "get from empty queue",
			batchesToAdd:  []int{},
			callNextCount: 1,
			expectEmptyAt: 0, // First call should be empty
			expectErrors:  []bool{false},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bq := setupTestQueue(t)
			ctx := context.Background()

			// Add batches
			addedBatches := make([]coresequencer.Batch, 0, len(tc.batchesToAdd))
			for _, txCount := range tc.batchesToAdd {
				batch := createTestBatch(t, txCount)
				addedBatches = append(addedBatches, batch)
				err := bq.AddBatch(ctx, batch)
				if err != nil {
					t.Fatalf("unexpected error adding batch: %v", err)
				}
			}

			// Call Next the specified number of times
			for i := 0; i < tc.callNextCount; i++ {
				batch, err := bq.Next(ctx)

				// Check error as expected
				if i < len(tc.expectErrors) && tc.expectErrors[i] {
					if err == nil {
						t.Errorf("expected error on call %d but got none", i)
					}
					continue
				} else if err != nil {
					t.Errorf("unexpected error on call %d: %v", i, err)
				}

				// Check if batch should be empty at this call
				if i == tc.expectEmptyAt {
					if len(batch.Transactions) != 0 {
						t.Errorf("expected empty batch at call %d, got %d transactions", i, len(batch.Transactions))
					}
				} else if i < tc.expectEmptyAt {
					// Verify the batch matches what we added in the right order
					expectedBatch := addedBatches[i]
					if len(expectedBatch.Transactions) != len(batch.Transactions) {
						t.Errorf("expected %d transactions, got %d", len(expectedBatch.Transactions), len(batch.Transactions))
					}

					// Check each transaction
					for j, tx := range batch.Transactions {
						if !bytes.Equal(expectedBatch.Transactions[j], tx) {
							t.Errorf("transaction mismatch at index %d", j)
						}
					}
				}
			}
		})
	}
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name             string
		setupBatchCounts []int // Transaction counts for batches to add before the test
		processCount     int   // Number of batches to process before reload
		expectedQueueLen int   // Expected queue length after reload
		expectErr        bool
	}{
		{
			name:             "reload from empty datastore",
			setupBatchCounts: []int{},
			processCount:     0,
			expectedQueueLen: 0,
			expectErr:        false,
		},
		{
			name:             "reload unprocessed batches only",
			setupBatchCounts: []int{1, 2, 3, 4, 5},
			processCount:     2, // Process the first two batches
			expectedQueueLen: 3, // Should reload the remaining 3
			expectErr:        false,
		},
		{
			name:             "reload when all batches processed",
			setupBatchCounts: []int{1, 2, 3},
			processCount:     3, // Process all batches
			expectedQueueLen: 0, // Should reload none
			expectErr:        false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bq := setupTestQueue(t)
			ctx := context.Background()

			// Add initial batches
			for _, txCount := range tc.setupBatchCounts {
				batch := createTestBatch(t, txCount)
				err := bq.AddBatch(ctx, batch)
				if err != nil {
					t.Fatalf("unexpected error adding batch: %v", err)
				}
			}

			// Process specified number of batches
			for i := 0; i < tc.processCount; i++ {
				if i < len(tc.setupBatchCounts) {
					_, err := bq.Next(ctx)
					if err != nil {
						t.Fatalf("unexpected error processing batch: %v", err)
					}
				}
			}

			// Clear the queue to simulate restart
			bq.queue = make([]coresequencer.Batch, 0)

			// Reload from datastore
			err := bq.Load(ctx)
			if tc.expectErr && err == nil {
				t.Error("expected error but got none")
			}
			if !tc.expectErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			// Verify queue length after reload
			if len(bq.queue) != tc.expectedQueueLen {
				t.Errorf("expected queue length %d, got %d", tc.expectedQueueLen, len(bq.queue))
			}
		})
	}
}

func TestConcurrency(t *testing.T) {
	bq := setupTestQueue(t)
	ctx := context.Background()

	// Number of concurrent operations
	const numOperations = 100

	// Add batches concurrently
	addWg := new(sync.WaitGroup)
	addWg.Add(numOperations)

	for i := 0; i < numOperations; i++ {
		go func(index int) {
			defer addWg.Done()
			batch := createTestBatch(t, index%10+1) // 1-10 transactions
			err := bq.AddBatch(ctx, batch)
			if err != nil {
				t.Errorf("unexpected error adding batch: %v", err)
			}
		}(i)
	}

	// Wait for all adds to complete
	addWg.Wait()

	// Verify we have expected number of batches
	if len(bq.queue) != numOperations {
		t.Errorf("expected %d batches, got %d", numOperations, len(bq.queue))
	}

	// Next operations concurrently (only half)
	nextWg := new(sync.WaitGroup)
	nextCount := numOperations / 2
	nextWg.Add(nextCount)

	for i := 0; i < nextCount; i++ {
		go func() {
			defer nextWg.Done()
			batch, err := bq.Next(ctx)
			if err != nil {
				t.Errorf("unexpected error getting batch: %v", err)
			}
			if batch == nil {
				t.Error("expected non-nil batch")
			}
		}()
	}

	// Wait for all nexts to complete
	nextWg.Wait()

	// Verify we have expected number of batches left
	if len(bq.queue) != numOperations-nextCount {
		t.Errorf("expected %d batches left, got %d", numOperations-nextCount, len(bq.queue))
	}

	// Test Load
	err := bq.Load(ctx)
	if err != nil {
		t.Errorf("unexpected error reloading: %v", err)
	}
}

func TestBatchEncodeDecode(t *testing.T) {
	tests := []struct {
		name       string
		batch      *coresequencer.Batch
		wantErr    bool
		errMessage string
	}{
		{
			name: "normal batch with multiple transactions",
			batch: &coresequencer.Batch{
				Transactions: [][]byte{
					[]byte("transaction1"),
					[]byte("transaction2"),
					[]byte("transaction3"),
				},
			},
			wantErr: false,
		},
		{
			name: "batch with empty transaction",
			batch: &coresequencer.Batch{
				Transactions: [][]byte{
					[]byte(""),
					[]byte("transaction"),
				},
			},
			wantErr: false,
		},
		{
			name:    "nil batch",
			batch:   nil,
			wantErr: false,
		},
		{
			name: "empty batch",
			batch: &coresequencer.Batch{
				Transactions: [][]byte{},
			},
			wantErr: false,
		},
		{
			name: "batch with large transactions",
			batch: &coresequencer.Batch{
				Transactions: [][]byte{
					bytes.Repeat([]byte("a"), 1000),
					bytes.Repeat([]byte("b"), 2000),
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test encoding
			encoded := encode(tt.batch)

			// Create a new batch for decoding
			decodedBatch, err := decode(encoded)

			// Check error expectations
			if (err != nil) != tt.wantErr {
				t.Errorf("Decode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				if err.Error() != tt.errMessage {
					t.Errorf("Decode() error message = %v, want %v", err.Error(), tt.errMessage)
				}
				return
			}

			// For nil batch case
			if tt.batch == nil {
				if len(decodedBatch.Transactions) != 0 {
					t.Errorf("Decode() nil batch should result in empty transactions")
				}
				return
			}

			// Compare original and decoded batch
			if len(tt.batch.Transactions) != len(decodedBatch.Transactions) {
				t.Errorf("Decode() got %v transactions, want %v",
					len(decodedBatch.Transactions),
					len(tt.batch.Transactions))
				return
			}

			// Compare each transaction
			for i := range tt.batch.Transactions {
				if !bytes.Equal(tt.batch.Transactions[i], decodedBatch.Transactions[i]) {
					t.Errorf("Decode() transaction %d = %v, want %v",
						i,
						decodedBatch.Transactions[i],
						tt.batch.Transactions[i])
				}
			}
		})
	}
}

func TestBatchDecodeErrors(t *testing.T) {
	tests := []struct {
		name       string
		input      []byte
		wantErr    bool
		errMessage string
	}{
		{
			name:       "too short input",
			input:      []byte{1, 2, 3},
			wantErr:    true,
			errMessage: "invalid batch data: too short",
		},
		{
			name:       "truncated transaction length",
			input:      append(make([]byte, 8), 1, 2, 3),
			wantErr:    true,
			errMessage: "invalid batch data: truncated at transaction 0 length",
		},
		{
			name: "truncated transaction data",
			input: func() []byte {
				buf := make([]byte, 16)                 // 8 bytes for num_txs + 8 bytes for tx length
				binary.BigEndian.PutUint64(buf[0:], 1)  // 1 transaction
				binary.BigEndian.PutUint64(buf[8:], 10) // transaction length of 10
				return buf
			}(),
			wantErr:    true,
			errMessage: "invalid batch data: truncated at transaction 0 data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decode(tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("Decode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && err.Error() != tt.errMessage {
				t.Errorf("Decode() error message = %v, want %v", err.Error(), tt.errMessage)
			}
		})
	}
}
