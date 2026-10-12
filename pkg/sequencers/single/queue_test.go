package single

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	ds "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
	dssync "github.com/ipfs/go-datastore/sync"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	coresequencer "github.com/evstack/ev-node/core/sequencer"
	"github.com/evstack/ev-node/pkg/store"
	pb "github.com/evstack/ev-node/types/pb/evnode/v1"
)

var testBatchNonce atomic.Uint64

// failingDeleteOnceDatastore fails the first batched WAL delete commit.
type failingDeleteOnceDatastore struct {
	ds.Batching
	failed bool
}

func (d *failingDeleteOnceDatastore) Batch(ctx context.Context) (ds.Batch, error) {
	b, err := d.Batching.Batch(ctx)
	if err != nil {
		return nil, err
	}
	return &failingOnceBatch{Batch: b, parent: d}, nil
}

// failingOnceBatch fails its first Commit, then delegates.
type failingOnceBatch struct {
	ds.Batch
	parent *failingDeleteOnceDatastore
}

func (b *failingOnceBatch) Commit(ctx context.Context) error {
	if !b.parent.failed {
		b.parent.failed = true
		return assert.AnError
	}
	return b.Batch.Commit(ctx)
}

// createTestBatch creates a batch with unique dummy transactions for testing.
// each tx is globally unique via an incrementing nonce.
func createTestBatch(t *testing.T, txCount int) coresequencer.Batch {
	txs := make([][]byte, txCount)
	for i := range txCount {
		n := testBatchNonce.Add(1)
		txs[i] = fmt.Appendf(nil, "tx-%d-%d", n, i)
	}
	return coresequencer.Batch{Transactions: txs}
}

func setupTestQueue(t *testing.T) *BatchQueue {
	// Create an in-memory thread-safe datastore
	memdb := store.NewPrefixKVStore(ds.NewMapDatastore(), "single")
	return NewBatchQueue(memdb, "batching", 0, zerolog.Nop()) // 0 = unlimited for existing tests
}

// drainOne pops exactly one queue entry and acks it immediately.
func drainOne(ctx context.Context, t *testing.T, bq *BatchQueue) *coresequencer.Batch {
	t.Helper()
	batch, err := bq.Drain(ctx, 1)
	require.NoError(t, err)
	require.NoError(t, bq.Ack(ctx))
	return batch
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
			if bq.Size() != tc.expectQueueLen {
				t.Errorf("expected queue length %d, got %d", tc.expectQueueLen, bq.Size())
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
			expectQueueLen: 0, // empty batches are no-ops after dedup
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
			if bq.Size() != tc.expectQueueLen {
				t.Errorf("expected queue length %d, got %d", tc.expectQueueLen, bq.Size())
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

func TestDrainOneByOne(t *testing.T) {
	tests := []struct {
		name          string
		batchesToAdd  []int
		callNextCount int
		expectEmptyAt int // At which drain we expect an empty batch
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

			// Drain one entry at a time the specified number of times
			for i := 0; i < tc.callNextCount; i++ {
				batch, err := bq.Drain(ctx, 1)
				if err == nil {
					err = bq.Ack(ctx)
				}

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
	bq := NewBatchQueue(rawDB, queuePrefix, 0, zerolog.Nop()) // 0 = unlimited for test
	require.NotNil(bq)

	// 1. Add valid batch data under the correct prefix
	// Use valid hex sequence keys to ensure Load parses them correctly if needed
	key1 := "8000000000000001"
	validBatch1 := createTestBatch(t, 3)
	hash1, err := validBatch1.Hash()
	require.NoError(err)
	hexHash1 := hex.EncodeToString(hash1)
	pbBatch1 := &pb.Batch{Txs: validBatch1.Transactions}
	encodedBatch1, err := proto.Marshal(pbBatch1)
	require.NoError(err)
	err = rawDB.Put(ctx, ds.NewKey(queuePrefix+key1), encodedBatch1)
	require.NoError(err)

	key2 := "8000000000000002"
	validBatch2 := createTestBatch(t, 5)
	hash2, err := validBatch2.Hash()
	require.NoError(err)
	hexHash2 := hex.EncodeToString(hash2)
	pbBatch2 := &pb.Batch{Txs: validBatch2.Transactions}
	encodedBatch2, err := proto.Marshal(pbBatch2)
	require.NoError(err)
	err = rawDB.Put(ctx, ds.NewKey(queuePrefix+key2), encodedBatch2)
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
		queuePrefix + key1:     true,
		queuePrefix + key2:     true,
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
	require.Equal(2, bq.Size(), "Queue should contain only the 2 valid batches")
	// Check hashes to be sure (order might vary depending on datastore query)
	loadedHashes := make(map[string]bool)
	bq.mu.Lock()
	for i := bq.head; i < len(bq.queue); i++ {
		h, _ := bq.queue[i].Batch.Hash()
		loadedHashes[hex.EncodeToString(h)] = true
	}
	bq.mu.Unlock()
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

func TestBatchQueue_Load_SetsSequencesProperly(t *testing.T) {
	ctx := context.Background()
	db := ds.NewMapDatastore()
	prefix := "test-load-sequences"

	// Build some persisted state with keys on both sides of the initialSeqNum.
	// Prepend-side keys are written the same way the postponed-ack path does.
	q1 := NewBatchQueue(db, prefix, 0, zerolog.Nop())
	require.NoError(t, q1.Load(ctx))

	require.NoError(t, q1.AddBatch(ctx, coresequencer.Batch{Transactions: [][]byte{[]byte("add-1")}})) // initialSeqNum
	require.NoError(t, q1.AddBatch(ctx, coresequencer.Batch{Transactions: [][]byte{[]byte("add-2")}})) // initialSeqNum+1

	require.NoError(t, q1.persistBatch(ctx, coresequencer.Batch{Transactions: [][]byte{[]byte("pre-1")}}, seqToKey(initialSeqNum-1)))
	require.NoError(t, q1.persistBatch(ctx, coresequencer.Batch{Transactions: [][]byte{[]byte("pre-2")}}, seqToKey(initialSeqNum-2)))

	// Simulate restart.
	q2 := NewBatchQueue(db, prefix, 0, zerolog.Nop())
	require.NoError(t, q2.Load(ctx))

	// After Load(), the sequencers should be positioned to avoid collisions:
	// - nextAddSeq should be (maxSeq + 1)
	// - nextPrependSeq should be (minSeq - 1)
	require.Equal(t, initialSeqNum+2, q2.nextAddSeq, "nextAddSeq should continue after the max loaded key")
	require.Equal(t, initialSeqNum-3, q2.nextPrependSeq, "nextPrependSeq should continue before the min loaded key")

	// Verify we actually use those sequences when persisting new items.
	require.NoError(t, q2.AddBatch(ctx, coresequencer.Batch{Transactions: [][]byte{[]byte("add-after-load")}}))
	_, err := q2.db.Get(ctx, ds.NewKey(seqToKey(initialSeqNum+2)))
	require.NoError(t, err, "expected AddBatch after Load to persist using nextAddSeq key")

	// postponed txs are requeued during Ack using nextPrependSeq
	_, err = q2.Drain(ctx, 1)
	require.NoError(t, err)
	q2.SetPostponed([][]byte{[]byte("pre-after-load")})
	require.NoError(t, q2.Ack(ctx))
	_, err = q2.db.Get(ctx, ds.NewKey(seqToKey(initialSeqNum-3)))
	require.NoError(t, err, "expected postponed requeue after Load to persist using nextPrependSeq key")
}

func TestConcurrency(t *testing.T) {
	bq := setupTestQueue(t)
	ctx := context.Background()

	// Number of concurrent operations
	const numOperations = 100

	// Add batches concurrently
	addWg := new(sync.WaitGroup)
	addWg.Add(numOperations)

	for i := range numOperations {
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
	if bq.Size() != numOperations {
		t.Errorf("expected %d batches, got %d", numOperations, bq.Size())
	}

	// drain half the entries one at a time (single consumer)
	nextCount := numOperations / 2
	for range nextCount {
		batch := drainOne(ctx, t, bq)
		if len(batch.Transactions) == 0 {
			t.Error("expected non-empty batch")
		}
	}

	// Verify we have expected number of batches left
	if bq.Size() != numOperations-nextCount {
		t.Errorf("expected %d batches left, got %d", numOperations-nextCount, bq.Size())
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
			memdb := store.NewPrefixKVStore(ds.NewMapDatastore(), "single")
			bq := NewBatchQueue(memdb, "batching", tc.maxSize, zerolog.Nop())
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
			if tc.maxSize > 0 && bq.Size() > tc.maxSize {
				t.Errorf("queue size %d exceeds limit %d", bq.Size(), tc.maxSize)
			}

			// For unlimited queues, verify all batches were added
			if tc.maxSize == 0 && !tc.expectErr && bq.Size() != tc.batchesToAdd {
				t.Errorf("expected %d batches in unlimited queue, got %d", tc.batchesToAdd, bq.Size())
			}
		})
	}
}

func TestBatchQueue_QueueLimit_WithDrain(t *testing.T) {
	// Test that removing batches with Drain+Ack allows adding more batches
	maxSize := 3
	memdb := store.NewPrefixKVStore(ds.NewMapDatastore(), "single")
	bq := NewBatchQueue(memdb, "batching", maxSize, zerolog.Nop())
	ctx := context.Background()

	// Fill the queue to capacity
	for i := range maxSize {
		batch := createTestBatch(t, i+1)
		err := bq.AddBatch(ctx, batch)
		if err != nil {
			t.Fatalf("unexpected error adding batch %d: %v", i, err)
		}
	}

	// Verify queue is full
	if bq.Size() != maxSize {
		t.Errorf("expected queue size %d, got %d", maxSize, bq.Size())
	}

	// Try to add one more batch - should fail
	extraBatch := createTestBatch(t, 999)
	err := bq.AddBatch(ctx, extraBatch)
	if !errors.Is(err, ErrQueueFull) {
		t.Errorf("expected ErrQueueFull, got %v", err)
	}

	// Remove one batch
	batch := drainOne(ctx, t, bq)
	if len(batch.Transactions) == 0 {
		t.Error("expected non-empty batch from drain")
	}

	// Verify queue size decreased
	if bq.Size() != maxSize-1 {
		t.Errorf("expected queue size %d after drain, got %d", maxSize-1, bq.Size())
	}

	// Now adding a batch should succeed
	newBatch := createTestBatch(t, 1000)
	err = bq.AddBatch(ctx, newBatch)
	if err != nil {
		t.Errorf("unexpected error adding batch after drain: %v", err)
	}

	// Verify queue is full again
	if bq.Size() != maxSize {
		t.Errorf("expected queue size %d after adding back, got %d", maxSize, bq.Size())
	}
}

func TestBatchQueue_QueueLimit_Concurrency(t *testing.T) {
	// Test thread safety of queue limits under concurrent access
	maxSize := 10
	memdb := dssync.MutexWrap(ds.NewMapDatastore()) // Thread-safe datastore
	bq := NewBatchQueue(memdb, "batching", maxSize, zerolog.Nop())
	ctx := context.Background()

	numWorkers := 20
	batchesPerWorker := 5
	totalBatches := numWorkers * batchesPerWorker

	var wg sync.WaitGroup
	var addedCount int64
	var errorCount int64

	// Start multiple workers trying to add batches concurrently
	for i := range numWorkers {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := range batchesPerWorker {
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
	if bq.Size() > maxSize {
		t.Errorf("queue size %d exceeds limit %d", bq.Size(), maxSize)
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

func TestBatchQueue_Drain_MergesMultipleEntries(t *testing.T) {
	ctx := context.Background()
	db := ds.NewMapDatastore()
	queue := NewBatchQueue(db, "test-drain-merge", 0, zerolog.Nop())
	require.NoError(t, queue.Load(ctx))

	require.NoError(t, queue.AddBatch(ctx, coresequencer.Batch{Transactions: [][]byte{[]byte("tx1")}}))
	require.NoError(t, queue.AddBatch(ctx, coresequencer.Batch{Transactions: [][]byte{[]byte("tx2")}}))
	require.NoError(t, queue.AddBatch(ctx, coresequencer.Batch{Transactions: [][]byte{[]byte("tx3")}}))

	batch, err := queue.Drain(ctx, 0)
	require.NoError(t, err)
	assert.Equal(t, 3, len(batch.Transactions))
	assert.Equal(t, []byte("tx1"), batch.Transactions[0])
	assert.Equal(t, []byte("tx2"), batch.Transactions[1])
	assert.Equal(t, []byte("tx3"), batch.Transactions[2])

	// Size includes inFlight (3 drained entries)
	assert.Equal(t, 3, queue.Size())
}

func TestBatchQueue_Drain_RespectsMaxBytes(t *testing.T) {
	ctx := context.Background()
	db := ds.NewMapDatastore()
	queue := NewBatchQueue(db, "test-drain-maxbytes", 0, zerolog.Nop())
	require.NoError(t, queue.Load(ctx))

	// each tx is 3 bytes
	require.NoError(t, queue.AddBatch(ctx, coresequencer.Batch{Transactions: [][]byte{[]byte("aaa")}}))
	require.NoError(t, queue.AddBatch(ctx, coresequencer.Batch{Transactions: [][]byte{[]byte("bbb")}}))
	require.NoError(t, queue.AddBatch(ctx, coresequencer.Batch{Transactions: [][]byte{[]byte("ccc")}}))

	// maxBytes=5 should only fit the first entry (3 bytes), second would exceed
	batch, err := queue.Drain(ctx, 5)
	require.NoError(t, err)
	assert.Equal(t, 1, len(batch.Transactions))
	assert.Equal(t, []byte("aaa"), batch.Transactions[0])

	// 2 entries in queue + 1 inFlight = 3 total
	assert.Equal(t, 3, queue.Size())
}

func TestBatchQueue_Drain_AlwaysTakesAtLeastOne(t *testing.T) {
	ctx := context.Background()
	db := ds.NewMapDatastore()
	queue := NewBatchQueue(db, "test-drain-atleastone", 0, zerolog.Nop())
	require.NoError(t, queue.Load(ctx))

	// entry is 10 bytes, maxBytes is 1
	require.NoError(t, queue.AddBatch(ctx, coresequencer.Batch{Transactions: [][]byte{[]byte("0123456789")}}))

	batch, err := queue.Drain(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, 1, len(batch.Transactions))
	assert.Equal(t, 1, queue.Size()) // inFlight counts
}

func TestBatchQueue_Drain_EmptyQueue(t *testing.T) {
	ctx := context.Background()
	db := ds.NewMapDatastore()
	queue := NewBatchQueue(db, "test-drain-empty", 0, zerolog.Nop())
	require.NoError(t, queue.Load(ctx))

	batch, err := queue.Drain(ctx, 0)
	require.NoError(t, err)
	assert.Nil(t, batch.Transactions)
}

func TestBatchQueue_Drain_RollsBackUnackedInFlight(t *testing.T) {
	ctx := context.Background()
	db := ds.NewMapDatastore()
	queue := NewBatchQueue(db, "test-drain-rollback", 0, zerolog.Nop())
	require.NoError(t, queue.Load(ctx))

	require.NoError(t, queue.AddBatch(ctx, coresequencer.Batch{Transactions: [][]byte{[]byte("tx1")}}))
	require.NoError(t, queue.AddBatch(ctx, coresequencer.Batch{Transactions: [][]byte{[]byte("tx2")}}))

	// first drain takes both entries
	batch1, err := queue.Drain(ctx, 0)
	require.NoError(t, err)
	assert.Equal(t, 2, len(batch1.Transactions))
	assert.Equal(t, 2, queue.Size()) // inFlight counts

	// second drain without ack should roll back inFlight
	batch2, err := queue.Drain(ctx, 0)
	require.NoError(t, err)
	assert.Equal(t, 2, len(batch2.Transactions))
	assert.Equal(t, []byte("tx1"), batch2.Transactions[0])
	assert.Equal(t, []byte("tx2"), batch2.Transactions[1])
}

func TestBatchQueue_Drain_RollbackBulkPrependAfterCompact(t *testing.T) {
	ctx := context.Background()
	db := ds.NewMapDatastore()
	queue := NewBatchQueue(db, "test-drain-rollback-bulk", 0, zerolog.Nop())
	require.NoError(t, queue.Load(ctx))

	// drain >100 entries so compactLocked resets head to 0 while they
	// are in flight, forcing the bulk-prepend rollback path on next drain
	const n = 150
	for i := range n {
		tx := fmt.Appendf(nil, "tx-%03d", i)
		require.NoError(t, queue.AddBatch(ctx, coresequencer.Batch{Transactions: [][]byte{tx}}))
	}

	batch1, err := queue.Drain(ctx, 0)
	require.NoError(t, err)
	require.Len(t, batch1.Transactions, n)
	require.Equal(t, 0, queue.head, "compact should reset head while entries are in flight")

	// enqueue one more so the rollback has a tail to prepend in front of
	require.NoError(t, queue.AddBatch(ctx, coresequencer.Batch{Transactions: [][]byte{[]byte("tx-new")}}))

	// second drain without ack rolls back via the bulk-prepend path
	batch2, err := queue.Drain(ctx, 0)
	require.NoError(t, err)
	require.Len(t, batch2.Transactions, n+1)
	for i := range n {
		assert.Equal(t, fmt.Appendf(nil, "tx-%03d", i), batch2.Transactions[i])
	}
	assert.Equal(t, []byte("tx-new"), batch2.Transactions[n])
}

func TestBatchQueue_Ack_DeletesWALEntries(t *testing.T) {
	ctx := context.Background()
	db := ds.NewMapDatastore()
	queue := NewBatchQueue(db, "test-ack-wal", 0, zerolog.Nop())
	require.NoError(t, queue.Load(ctx))

	require.NoError(t, queue.AddBatch(ctx, coresequencer.Batch{Transactions: [][]byte{[]byte("tx1")}}))
	require.NoError(t, queue.AddBatch(ctx, coresequencer.Batch{Transactions: [][]byte{[]byte("tx2")}}))

	_, err := queue.Drain(ctx, 0)
	require.NoError(t, err)

	require.NoError(t, queue.Ack(ctx))

	// WAL should be empty — reload and verify
	queue2 := NewBatchQueue(db, "test-ack-wal", 0, zerolog.Nop())
	require.NoError(t, queue2.Load(ctx))
	assert.Equal(t, 0, queue2.Size())
}

func TestBatchQueue_Ack_RequeuesPostponedTxs(t *testing.T) {
	ctx := context.Background()
	db := ds.NewMapDatastore()
	queue := NewBatchQueue(db, "test-ack-postponed", 0, zerolog.Nop())
	require.NoError(t, queue.Load(ctx))

	require.NoError(t, queue.AddBatch(ctx, coresequencer.Batch{Transactions: [][]byte{[]byte("tx1"), []byte("tx2")}}))

	_, err := queue.Drain(ctx, 0)
	require.NoError(t, err)

	// set postponed txs and ack
	queue.SetPostponed([][]byte{[]byte("postponed1"), []byte("postponed2")})
	require.NoError(t, queue.Ack(ctx))

	// queue should have the postponed txs
	assert.Equal(t, 1, queue.Size())

	batch, err := queue.Drain(ctx, 0)
	require.NoError(t, err)
	assert.Equal(t, 2, len(batch.Transactions))
	assert.Equal(t, []byte("postponed1"), batch.Transactions[0])
	assert.Equal(t, []byte("postponed2"), batch.Transactions[1])
}

func TestBatchQueue_Ack_PostponedTxsStayDeduped(t *testing.T) {
	ctx := context.Background()
	db := ds.NewMapDatastore()
	queue := NewBatchQueue(db, "test-ack-postponed-dedup", 0, zerolog.Nop())
	require.NoError(t, queue.Load(ctx))

	postponedTx := []byte("postponed")
	require.NoError(t, queue.AddBatch(ctx, coresequencer.Batch{Transactions: [][]byte{[]byte("tx1"), postponedTx}}))

	_, err := queue.Drain(ctx, 0)
	require.NoError(t, err)

	queue.SetPostponed([][]byte{postponedTx})
	require.NoError(t, queue.Ack(ctx))
	assert.Equal(t, 1, queue.Size())

	// re-submitting the postponed tx (e.g. reaper rescrape) must be deduped,
	// otherwise it would be included twice in the next drained batch
	require.NoError(t, queue.AddBatch(ctx, coresequencer.Batch{Transactions: [][]byte{postponedTx}}))
	assert.Equal(t, 1, queue.Size())

	batch, err := queue.Drain(ctx, 0)
	require.NoError(t, err)
	assert.Equal(t, [][]byte{postponedTx}, batch.Transactions)

	// committed (non-postponed) txs are released from the dedup set on ack
	require.NoError(t, queue.Ack(ctx))
	require.NoError(t, queue.AddBatch(ctx, coresequencer.Batch{Transactions: [][]byte{[]byte("tx1")}}))
	assert.Equal(t, 1, queue.Size())
}

func TestBatchQueue_InFlight_SurvivesRestart(t *testing.T) {
	ctx := context.Background()
	db := ds.NewMapDatastore()
	queue := NewBatchQueue(db, "test-inflight-restart", 0, zerolog.Nop())
	require.NoError(t, queue.Load(ctx))

	require.NoError(t, queue.AddBatch(ctx, coresequencer.Batch{Transactions: [][]byte{[]byte("tx1")}}))
	require.NoError(t, queue.AddBatch(ctx, coresequencer.Batch{Transactions: [][]byte{[]byte("tx2")}}))

	// drain without ack
	_, err := queue.Drain(ctx, 0)
	require.NoError(t, err)

	// simulate restart — WAL entries should still exist since no ack
	queue2 := NewBatchQueue(db, "test-inflight-restart", 0, zerolog.Nop())
	require.NoError(t, queue2.Load(ctx))
	assert.Equal(t, 2, queue2.Size())

	batch, err := queue2.Drain(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, []byte("tx1"), batch.Transactions[0])
}

func TestBatchQueue_InFlight_CountsTowardQueueLimit(t *testing.T) {
	ctx := context.Background()
	db := ds.NewMapDatastore()
	maxSize := 3
	queue := NewBatchQueue(db, "test-inflight-limit", maxSize, zerolog.Nop())
	require.NoError(t, queue.Load(ctx))

	// fill to capacity
	for i := range maxSize {
		require.NoError(t, queue.AddBatch(ctx, coresequencer.Batch{
			Transactions: [][]byte{fmt.Appendf(nil, "tx%d", i)},
		}))
	}

	// drain all — moves entries to inFlight
	_, err := queue.Drain(ctx, 0)
	require.NoError(t, err)

	// Size includes inFlight
	assert.Equal(t, 3, queue.Size())

	// adding should still be rejected because inFlight counts
	err = queue.AddBatch(ctx, coresequencer.Batch{Transactions: [][]byte{[]byte("overflow")}})
	assert.ErrorIs(t, err, ErrQueueFull)

	// after ack, adding should succeed
	require.NoError(t, queue.Ack(ctx))
	err = queue.AddBatch(ctx, coresequencer.Batch{Transactions: [][]byte{[]byte("ok")}})
	assert.NoError(t, err)
}

func TestBatchQueue_Ack_PersistsPostponedBeforeDeletingWAL(t *testing.T) {
	ctx := context.Background()
	db := ds.NewMapDatastore()
	queue := NewBatchQueue(db, "test-ack-order", 0, zerolog.Nop())
	require.NoError(t, queue.Load(ctx))

	require.NoError(t, queue.AddBatch(ctx, coresequencer.Batch{Transactions: [][]byte{[]byte("tx1")}}))

	_, err := queue.Drain(ctx, 0)
	require.NoError(t, err)

	// set postponed txs and ack
	queue.SetPostponed([][]byte{[]byte("postponed")})
	require.NoError(t, queue.Ack(ctx))

	// simulate restart — postponed tx should survive
	queue2 := NewBatchQueue(db, "test-ack-order", 0, zerolog.Nop())
	require.NoError(t, queue2.Load(ctx))
	assert.Equal(t, 1, queue2.Size())

	batch, err := queue2.Drain(ctx, 0)
	require.NoError(t, err)
	assert.Equal(t, []byte("postponed"), batch.Transactions[0])
}

func TestBatchQueue_AckRetry_DoesNotDuplicatePostponed(t *testing.T) {
	ctx := context.Background()
	db := ds.NewMapDatastore()
	queue := NewBatchQueue(db, "test-ack-retry-postponed", 0, zerolog.Nop())
	require.NoError(t, queue.Load(ctx))

	require.NoError(t, queue.AddBatch(ctx, coresequencer.Batch{Transactions: [][]byte{[]byte("tx1")}}))
	_, err := queue.Drain(ctx, 0)
	require.NoError(t, err)
	queue.SetPostponed([][]byte{[]byte("postponed")})

	// Simulate the first WAL delete failing after postponed txs were already persisted.
	originalDB := queue.db
	queue.db = &failingDeleteOnceDatastore{Batching: originalDB}
	err = queue.Ack(ctx)
	require.Error(t, err)

	// Retry with the original datastore. The postponed batch should not be persisted twice.
	queue.db = originalDB
	require.NoError(t, queue.Ack(ctx))

	reloaded := NewBatchQueue(db, "test-ack-retry-postponed", 0, zerolog.Nop())
	require.NoError(t, reloaded.Load(ctx))
	assert.Equal(t, 1, reloaded.Size())

	batch, err := reloaded.Drain(ctx, 0)
	require.NoError(t, err)
	assert.Equal(t, [][]byte{[]byte("postponed")}, batch.Transactions)
}

func TestBatchQueue_AckFailureThenDrain_AllowsNewPostponedDecision(t *testing.T) {
	ctx := context.Background()
	db := ds.NewMapDatastore()
	queue := NewBatchQueue(db, "test-ack-fail-drain", 0, zerolog.Nop())
	require.NoError(t, queue.Load(ctx))

	tx1, tx2 := []byte("tx1"), []byte("tx2")
	require.NoError(t, queue.AddBatch(ctx, coresequencer.Batch{Transactions: [][]byte{tx1, tx2}}))

	_, err := queue.Drain(ctx, 0)
	require.NoError(t, err)
	queue.SetPostponed([][]byte{tx2})

	// postponed entry is persisted, then the WAL delete fails
	originalDB := queue.db
	queue.db = &failingDeleteOnceDatastore{Batching: originalDB}
	require.Error(t, queue.Ack(ctx))
	queue.db = originalDB

	// a new drain rolls the in-flight entries back; each tx must appear
	// exactly once even though a postponed entry was persisted
	batch, err := queue.Drain(ctx, 0)
	require.NoError(t, err)
	assert.Equal(t, [][]byte{tx1, tx2}, batch.Transactions)

	// the filter decision may differ on retry — it must not be ignored
	queue.SetPostponed([][]byte{tx1})
	require.NoError(t, queue.Ack(ctx))

	// only the new postponed tx is re-queued
	batch, err = queue.Drain(ctx, 0)
	require.NoError(t, err)
	assert.Equal(t, [][]byte{tx1}, batch.Transactions)
	require.NoError(t, queue.Ack(ctx))

	// tx2 was committed: its hash must be released for re-submission
	require.NoError(t, queue.AddBatch(ctx, coresequencer.Batch{Transactions: [][]byte{tx2}}))
	assert.Equal(t, 1, queue.Size())

	// a reload must not resurrect the stale postponed WAL entry
	reloaded := NewBatchQueue(db, "test-ack-fail-drain", 0, zerolog.Nop())
	require.NoError(t, reloaded.Load(ctx))
	batch, err = reloaded.Drain(ctx, 0)
	require.NoError(t, err)
	assert.Equal(t, [][]byte{tx2}, batch.Transactions)
}

func TestBatchQueue_DropIncluded_CrashBetweenCommitAndAck(t *testing.T) {
	ctx := context.Background()
	db := ds.NewMapDatastore()
	queue := NewBatchQueue(db, "test-drop-included", 0, zerolog.Nop())
	require.NoError(t, queue.Load(ctx))

	committed1 := []byte("committed1")
	committed2 := []byte("committed2")
	postponed := []byte("postponed")
	require.NoError(t, queue.AddBatch(ctx, coresequencer.Batch{Transactions: [][]byte{committed1, postponed}}))
	require.NoError(t, queue.AddBatch(ctx, coresequencer.Batch{Transactions: [][]byte{committed2}}))

	// drain, block commits with committed1+committed2, then crash before Ack
	_, err := queue.Drain(ctx, 0)
	require.NoError(t, err)

	// restart — WAL reloads all entries
	queue2 := NewBatchQueue(db, "test-drop-included", 0, zerolog.Nop())
	require.NoError(t, queue2.Load(ctx))
	assert.Equal(t, 2, queue2.Size())

	dropped, err := queue2.DropIncluded(ctx, [][]byte{committed1, committed2})
	require.NoError(t, err)
	assert.Equal(t, 2, dropped)

	// only the postponed tx remains
	batch, err := queue2.Drain(ctx, 0)
	require.NoError(t, err)
	assert.Equal(t, [][]byte{postponed}, batch.Transactions)
	require.NoError(t, queue2.Ack(ctx))

	// dropped txs are released from the dedup set
	require.NoError(t, queue2.AddBatch(ctx, coresequencer.Batch{Transactions: [][]byte{committed1}}))
	assert.Equal(t, 1, queue2.Size())

	// rewritten WAL is consistent across another reload
	queue3 := NewBatchQueue(db, "test-drop-included", 0, zerolog.Nop())
	require.NoError(t, queue3.Load(ctx))
	batch, err = queue3.Drain(ctx, 0)
	require.NoError(t, err)
	assert.Equal(t, [][]byte{committed1}, batch.Transactions)
}

func TestBatchQueue_DropIncluded_NoMatches(t *testing.T) {
	ctx := context.Background()
	db := ds.NewMapDatastore()
	queue := NewBatchQueue(db, "test-drop-none", 0, zerolog.Nop())
	require.NoError(t, queue.Load(ctx))

	require.NoError(t, queue.AddBatch(ctx, coresequencer.Batch{Transactions: [][]byte{[]byte("tx1")}}))

	dropped, err := queue.DropIncluded(ctx, [][]byte{[]byte("other")})
	require.NoError(t, err)
	assert.Equal(t, 0, dropped)
	assert.Equal(t, 1, queue.Size())
}

// countWALEntries returns the number of WAL keys stored under the queue's prefix.
func countWALEntries(t *testing.T, bq *BatchQueue) int {
	t.Helper()
	results, err := bq.db.Query(context.Background(), query.Query{})
	require.NoError(t, err)
	defer results.Close()
	count := 0
	for result := range results.Next() {
		require.NoError(t, result.Error)
		count++
	}
	return count
}

func TestBatchQueue_Load_DeletesFullyDuplicateWALEntries(t *testing.T) {
	ctx := context.Background()
	db := ds.NewMapDatastore()
	queue := NewBatchQueue(db, "test-load-dup-clean", 0, zerolog.Nop())

	// craft a WAL with the same tx persisted under two keys,
	// simulating a partially failed ack/cleanup from a previous run
	tx := []byte("dup-tx")
	batch := coresequencer.Batch{Transactions: [][]byte{tx}}
	require.NoError(t, queue.persistBatch(ctx, batch, seqToKey(initialSeqNum)))
	require.NoError(t, queue.persistBatch(ctx, batch, seqToKey(initialSeqNum+1)))

	require.NoError(t, queue.Load(ctx))
	assert.Equal(t, 1, queue.Size())

	// the duplicate WAL entry must be deleted, not just dropped in memory
	assert.Equal(t, 1, countWALEntries(t, queue))

	// a fresh reload sees the same clean state
	queue2 := NewBatchQueue(db, "test-load-dup-clean", 0, zerolog.Nop())
	require.NoError(t, queue2.Load(ctx))
	assert.Equal(t, 1, queue2.Size())
}

func TestBatchQueue_Load_RewritesPartiallyDuplicateWALEntries(t *testing.T) {
	ctx := context.Background()
	db := ds.NewMapDatastore()
	queue := NewBatchQueue(db, "test-load-partial-clean", 0, zerolog.Nop())

	// a prepend-keyed entry (loads first) shares one tx with an add-keyed entry
	shared := []byte("shared")
	other := []byte("other")
	require.NoError(t, queue.persistBatch(ctx, coresequencer.Batch{Transactions: [][]byte{shared}}, seqToKey(initialSeqNum-1)))
	require.NoError(t, queue.persistBatch(ctx, coresequencer.Batch{Transactions: [][]byte{shared, other}}, seqToKey(initialSeqNum)))

	require.NoError(t, queue.Load(ctx))
	assert.Equal(t, 2, queue.Size())

	// consume and ack the entry holding the shared tx, releasing its hash
	batch, err := queue.Drain(ctx, 1) // small cap drains only the first entry
	require.NoError(t, err)
	assert.Equal(t, [][]byte{shared}, batch.Transactions)
	require.NoError(t, queue.Ack(ctx))

	// reload: the second entry's WAL value must have been rewritten without
	// the shared tx, so it cannot be resurrected after the first was consumed
	queue2 := NewBatchQueue(db, "test-load-partial-clean", 0, zerolog.Nop())
	require.NoError(t, queue2.Load(ctx))
	nextBatch, err := queue2.Drain(ctx, 0)
	require.NoError(t, err)
	assert.Equal(t, [][]byte{other}, nextBatch.Transactions)
}

func TestBatchQueue_Dedup_SkipsDuplicateTxs(t *testing.T) {
	ctx := context.Background()
	db := ds.NewMapDatastore()
	queue := NewBatchQueue(db, "test-dedup", 0, zerolog.Nop())
	require.NoError(t, queue.Load(ctx))

	tx := []byte("same-tx")

	// first add succeeds
	require.NoError(t, queue.AddBatch(ctx, coresequencer.Batch{Transactions: [][]byte{tx}}))
	assert.Equal(t, 1, queue.Size())

	// second add of same tx is silently skipped (dedup)
	require.NoError(t, queue.AddBatch(ctx, coresequencer.Batch{Transactions: [][]byte{tx}}))
	assert.Equal(t, 1, queue.Size())

	// after drain + ack, the hash is freed and the tx can be re-enqueued
	_, err := queue.Drain(ctx, 0)
	require.NoError(t, err)
	require.NoError(t, queue.Ack(ctx))

	require.NoError(t, queue.AddBatch(ctx, coresequencer.Batch{Transactions: [][]byte{tx}}))
	assert.Equal(t, 1, queue.Size())
}

func TestBatchQueue_Dedup_PartialBatch(t *testing.T) {
	ctx := context.Background()
	db := ds.NewMapDatastore()
	queue := NewBatchQueue(db, "test-dedup-partial", 0, zerolog.Nop())
	require.NoError(t, queue.Load(ctx))

	tx1 := []byte("tx1")
	tx2 := []byte("tx2")

	require.NoError(t, queue.AddBatch(ctx, coresequencer.Batch{Transactions: [][]byte{tx1}}))

	// batch with one dup and one new tx: only new tx enqueued
	require.NoError(t, queue.AddBatch(ctx, coresequencer.Batch{Transactions: [][]byte{tx1, tx2}}))
	assert.Equal(t, 2, queue.Size())

	// drain entries one at a time: batch1 has tx1, batch2 has only tx2
	batch1 := drainOne(ctx, t, queue)
	assert.Equal(t, [][]byte{tx1}, batch1.Transactions)

	batch2 := drainOne(ctx, t, queue)
	assert.Equal(t, [][]byte{tx2}, batch2.Transactions)
}

func TestBatchQueue_Dedup_InFlightBlocksReenqueue(t *testing.T) {
	ctx := context.Background()
	db := ds.NewMapDatastore()
	queue := NewBatchQueue(db, "test-dedup-inflight", 0, zerolog.Nop())
	require.NoError(t, queue.Load(ctx))

	tx := []byte("inflight-tx")
	require.NoError(t, queue.AddBatch(ctx, coresequencer.Batch{Transactions: [][]byte{tx}}))

	// drain moves to inFlight — tx hash stays in dedup set
	_, err := queue.Drain(ctx, 0)
	require.NoError(t, err)

	// re-add same tx while in-flight: silently skipped
	require.NoError(t, queue.AddBatch(ctx, coresequencer.Batch{Transactions: [][]byte{tx}}))
	// Size is 1 (inFlight), not 2
	assert.Equal(t, 1, queue.Size())
}

func TestBatchQueue_Dedup_SurvivesLoad(t *testing.T) {
	ctx := context.Background()
	db := ds.NewMapDatastore()
	queue := NewBatchQueue(db, "test-dedup-load", 0, zerolog.Nop())
	require.NoError(t, queue.Load(ctx))

	tx := []byte("persist-tx")
	require.NoError(t, queue.AddBatch(ctx, coresequencer.Batch{Transactions: [][]byte{tx}}))

	// simulate restart
	queue2 := NewBatchQueue(db, "test-dedup-load", 0, zerolog.Nop())
	require.NoError(t, queue2.Load(ctx))

	// dedup set is rebuilt from WAL — re-add should be skipped
	require.NoError(t, queue2.AddBatch(ctx, coresequencer.Batch{Transactions: [][]byte{tx}}))
	assert.Equal(t, 1, queue2.Size())
}
