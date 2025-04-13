package single

import (
	"bytes"
	"context"
	"sync"
	"testing"

	ds "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
	dssync "github.com/ipfs/go-datastore/sync"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	pb "github.com/rollkit/rollkit/types/pb/rollkit/v1"
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
	memdb := ds.NewMapDatastore()
	return NewBatchQueue(memdb, "test")
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

func TestLoad_WithMixedData(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	// Use a raw datastore to manually insert mixed data
	rawDB := dssync.MutexWrap(ds.NewMapDatastore())
	queuePrefix := "/batches/" // Define a specific prefix for the queue

	// Create the BatchQueue using the raw DB and the prefix
	bq := NewBatchQueue(rawDB, queuePrefix)
	require.NotNil(bq)

	// 1. Add valid batch data under the correct prefix
	validBatch1 := createTestBatch(t, 3)
	hash1, err := validBatch1.Hash()
	require.NoError(err)
	pbBatch1 := &pb.Batch{Txs: validBatch1.Transactions}
	encodedBatch1, err := proto.Marshal(pbBatch1)
	require.NoError(err)
	err = rawDB.Put(ctx, ds.NewKey(queuePrefix+string(hash1)), encodedBatch1)
	require.NoError(err)

	validBatch2 := createTestBatch(t, 5)
	hash2, err := validBatch2.Hash()
	require.NoError(err)
	pbBatch2 := &pb.Batch{Txs: validBatch2.Transactions}
	encodedBatch2, err := proto.Marshal(pbBatch2)
	require.NoError(err)
	err = rawDB.Put(ctx, ds.NewKey(queuePrefix+string(hash2)), encodedBatch2)
	require.NoError(err)

	// 3. Add data outside the queue's prefix
	otherDataKey1 := ds.NewKey("/other/data")
	err = rawDB.Put(ctx, otherDataKey1, []byte("some other data"))
	require.NoError(err)
	otherDataKey2 := ds.NewKey("root_data") // No prefix slash
	err = rawDB.Put(ctx, otherDataKey2, []byte("more data"))
	require.NoError(err)

	// Ensure all data is initially present in the raw DB
	initialKeys := map[string]bool{
		queuePrefix + string(hash1): true,
		queuePrefix + string(hash2): true,
		otherDataKey1.String():      true,
		otherDataKey2.String():      true,
	}
	q := query.Query{}
	results, err := rawDB.Query(ctx, q)
	require.NoError(err)
	count := 0
	for res := range results.Next() {
		require.NoError(res.Error)
		_, ok := initialKeys[res.Key]
		require.True(ok, "Unexpected key found before load: %s", res.Key)
		count++
	}
	results.Close()
	require.Equal(len(initialKeys), count, "Initial data count mismatch")

	// Call Load
	loadErr := bq.Load(ctx)
	// The current implementation prints errors but continues, so we expect no error return
	require.NoError(loadErr, "Load returned an unexpected error")

	// Verify queue contains only the valid batches
	require.Equal(2, len(bq.queue), "Queue should contain only the 2 valid batches")
	// Check hashes to be sure (order might vary depending on datastore query)
	loadedHashes := make(map[string]bool)
	for _, batch := range bq.queue {
		h, _ := batch.Hash()
		loadedHashes[string(h)] = true
	}
	require.True(loadedHashes[string(hash1)], "Valid batch 1 not found in queue")
	require.True(loadedHashes[string(hash2)], "Valid batch 2 not found in queue")

	// Verify data outside the prefix remains untouched in the raw DB
	val, err := rawDB.Get(ctx, otherDataKey1)
	require.NoError(err)
	require.Equal([]byte("some other data"), val)
	val, err = rawDB.Get(ctx, otherDataKey2)
	require.NoError(err)
	require.Equal([]byte("more data"), val)

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
