package single

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"sync"
	"sync/atomic"
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
	memdb := newPrefixKV(ds.NewMapDatastore(), "single")
	return NewBatchQueue(memdb, "batching", 0) // 0 = unlimited for existing tests
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

func TestLoad_WithMixedData(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	// Use a raw datastore to manually insert mixed data
	rawDB := dssync.MutexWrap(ds.NewMapDatastore())
	queuePrefix := "/batches/" // Define a specific prefix for the queue

	// Create the BatchQueue using the raw DB and the prefix
	bq := NewBatchQueue(rawDB, queuePrefix, 0) // 0 = unlimited for test
	require.NotNil(bq)

	// 1. Add valid batch data under the correct prefix
	validBatch1 := createTestBatch(t, 3)
	hash1, err := validBatch1.Hash()
	require.NoError(err)
	hexHash1 := hex.EncodeToString(hash1)
	pbBatch1 := &pb.Batch{Txs: validBatch1.Transactions}
	encodedBatch1, err := proto.Marshal(pbBatch1)
	require.NoError(err)
	err = rawDB.Put(ctx, ds.NewKey(queuePrefix+hexHash1), encodedBatch1)
	require.NoError(err)

	validBatch2 := createTestBatch(t, 5)
	hash2, err := validBatch2.Hash()
	require.NoError(err)
	hexHash2 := hex.EncodeToString(hash2)
	pbBatch2 := &pb.Batch{Txs: validBatch2.Transactions}
	encodedBatch2, err := proto.Marshal(pbBatch2)
	require.NoError(err)
	err = rawDB.Put(ctx, ds.NewKey(queuePrefix+hexHash2), encodedBatch2)
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
		queuePrefix + hexHash1: true,
		queuePrefix + hexHash2: true,
		otherDataKey1.String(): true,
		otherDataKey2.String(): true,
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
		loadedHashes[hex.EncodeToString(h)] = true
	}
	require.True(loadedHashes[hexHash1], "Valid batch 1 not found in queue")
	require.True(loadedHashes[hexHash2], "Valid batch 2 not found in queue")

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

func TestBatchQueue_QueueLimit(t *testing.T) {
	tests := []struct {
		name          string
		maxSize       int
		batchesToAdd  int
		expectErr     bool
		expectErrType error
	}{
		{
			name:          "unlimited queue (maxSize 0)",
			maxSize:       0,
			batchesToAdd:  100,
			expectErr:     false,
			expectErrType: nil,
		},
		{
			name:          "queue within limit",
			maxSize:       10,
			batchesToAdd:  5,
			expectErr:     false,
			expectErrType: nil,
		},
		{
			name:          "queue at exact limit",
			maxSize:       5,
			batchesToAdd:  5,
			expectErr:     false,
			expectErrType: nil,
		},
		{
			name:          "queue exceeds limit",
			maxSize:       3,
			batchesToAdd:  5,
			expectErr:     true,
			expectErrType: ErrQueueFull,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create in-memory datastore and queue with specified limit
			memdb := newPrefixKV(ds.NewMapDatastore(), "single")
			bq := NewBatchQueue(memdb, "batching", tc.maxSize)
			ctx := context.Background()

			var lastErr error

			// Add batches up to the specified count
			for i := 0; i < tc.batchesToAdd; i++ {
				batch := createTestBatch(t, i+1)
				err := bq.AddBatch(ctx, batch)
				if err != nil {
					lastErr = err
					if !tc.expectErr {
						t.Errorf("unexpected error at batch %d: %v", i, err)
					}
					break
				}
			}

			// Check final error state
			if tc.expectErr {
				if lastErr == nil {
					t.Error("expected error but got none")
				} else if !errors.Is(lastErr, tc.expectErrType) {
					t.Errorf("expected error %v, got %v", tc.expectErrType, lastErr)
				}
			} else {
				if lastErr != nil {
					t.Errorf("unexpected error: %v", lastErr)
				}
			}

			// For limited queues, verify the queue doesn't exceed maxSize
			if tc.maxSize > 0 && len(bq.queue) > tc.maxSize {
				t.Errorf("queue size %d exceeds limit %d", len(bq.queue), tc.maxSize)
			}

			// For unlimited queues, verify all batches were added
			if tc.maxSize == 0 && !tc.expectErr && len(bq.queue) != tc.batchesToAdd {
				t.Errorf("expected %d batches in unlimited queue, got %d", tc.batchesToAdd, len(bq.queue))
			}
		})
	}
}

func TestBatchQueue_QueueLimit_WithNext(t *testing.T) {
	// Test that removing batches with Next() allows adding more batches
	maxSize := 3
	memdb := newPrefixKV(ds.NewMapDatastore(), "single")
	bq := NewBatchQueue(memdb, "batching", maxSize)
	ctx := context.Background()

	// Fill the queue to capacity
	for i := 0; i < maxSize; i++ {
		batch := createTestBatch(t, i+1)
		err := bq.AddBatch(ctx, batch)
		if err != nil {
			t.Fatalf("unexpected error adding batch %d: %v", i, err)
		}
	}

	// Verify queue is full
	if len(bq.queue) != maxSize {
		t.Errorf("expected queue size %d, got %d", maxSize, len(bq.queue))
	}

	// Try to add one more batch - should fail
	extraBatch := createTestBatch(t, 999)
	err := bq.AddBatch(ctx, extraBatch)
	if !errors.Is(err, ErrQueueFull) {
		t.Errorf("expected ErrQueueFull, got %v", err)
	}

	// Remove one batch using Next()
	batch, err := bq.Next(ctx)
	if err != nil {
		t.Fatalf("unexpected error in Next(): %v", err)
	}
	if batch == nil || len(batch.Transactions) == 0 {
		t.Error("expected non-empty batch from Next()")
	}

	// Verify queue size decreased
	if len(bq.queue) != maxSize-1 {
		t.Errorf("expected queue size %d after Next(), got %d", maxSize-1, len(bq.queue))
	}

	// Now adding a batch should succeed
	newBatch := createTestBatch(t, 1000)
	err = bq.AddBatch(ctx, newBatch)
	if err != nil {
		t.Errorf("unexpected error adding batch after Next(): %v", err)
	}

	// Verify queue is full again
	if len(bq.queue) != maxSize {
		t.Errorf("expected queue size %d after adding back, got %d", maxSize, len(bq.queue))
	}
}

func TestBatchQueue_QueueLimit_Concurrency(t *testing.T) {
	// Test thread safety of queue limits under concurrent access
	maxSize := 10
	memdb := dssync.MutexWrap(ds.NewMapDatastore()) // Thread-safe datastore
	bq := NewBatchQueue(memdb, "batching", maxSize)
	ctx := context.Background()

	numWorkers := 20
	batchesPerWorker := 5
	totalBatches := numWorkers * batchesPerWorker

	var wg sync.WaitGroup
	var addedCount int64
	var errorCount int64

	// Start multiple workers trying to add batches concurrently
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < batchesPerWorker; j++ {
				batch := createTestBatch(t, workerID*batchesPerWorker+j+1)
				err := bq.AddBatch(ctx, batch)
				if err != nil {
					if errors.Is(err, ErrQueueFull) {
						// Expected error when queue is full
						atomic.AddInt64(&errorCount, 1)
					} else {
						t.Errorf("unexpected error type from worker %d: %v", workerID, err)
					}
				} else {
					atomic.AddInt64(&addedCount, 1)
				}
			}
		}(i)
	}

	wg.Wait()

	// Verify queue size doesn't exceed limit
	if len(bq.queue) > maxSize {
		t.Errorf("queue size %d exceeds limit %d", len(bq.queue), maxSize)
	}

	// Verify some batches were successfully added
	if addedCount == 0 {
		t.Error("no batches were added successfully")
	}

	// Verify some batches were rejected due to queue being full
	if addedCount == int64(totalBatches) {
		t.Error("all batches were added, but queue should have been full at some point")
	}

	// Verify the sum makes sense
	if addedCount+errorCount != int64(totalBatches) {
		t.Errorf("expected %d total operations, got %d added + %d errors = %d", 
			totalBatches, addedCount, errorCount, addedCount+errorCount)
	}

	t.Logf("Successfully added %d batches, rejected %d due to queue being full", addedCount, errorCount)
}
