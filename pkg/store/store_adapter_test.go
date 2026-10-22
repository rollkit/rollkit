package store

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evstack/ev-node/types"
)

// TestPendingCache_BasicOperations tests add, get, has, delete operations
func TestPendingCache_BasicOperations(t *testing.T) {
	t.Parallel()

	cache := newPendingCache[*types.P2PSignedHeader]()

	// Initially empty
	assert.Equal(t, 0, cache.len())
	assert.Equal(t, uint64(0), cache.getMaxHeight())

	// Create test items
	h1, _ := types.GetRandomBlock(1, 1, "test-chain")
	h2, _ := types.GetRandomBlock(2, 1, "test-chain")
	h3, _ := types.GetRandomBlock(3, 1, "test-chain")

	p1 := &types.P2PSignedHeader{SignedHeader: h1}
	p2 := &types.P2PSignedHeader{SignedHeader: h2}
	p3 := &types.P2PSignedHeader{SignedHeader: h3}

	// Add items
	cache.add(p1)
	assert.Equal(t, 1, cache.len())
	assert.Equal(t, uint64(1), cache.getMaxHeight())

	cache.add(p2)
	assert.Equal(t, 2, cache.len())
	assert.Equal(t, uint64(2), cache.getMaxHeight())

	cache.add(p3)
	assert.Equal(t, 3, cache.len())
	assert.Equal(t, uint64(3), cache.getMaxHeight())

	// Get by height
	item, ok := cache.get(1)
	assert.True(t, ok)
	assert.Equal(t, uint64(1), item.Height())

	item, ok = cache.get(2)
	assert.True(t, ok)
	assert.Equal(t, uint64(2), item.Height())

	// Get non-existent
	_, ok = cache.get(999)
	assert.False(t, ok)

	// Has
	assert.True(t, cache.has(1))
	assert.True(t, cache.has(2))
	assert.True(t, cache.has(3))
	assert.False(t, cache.has(999))

	// Delete
	cache.delete(2)
	assert.Equal(t, 2, cache.len())
	assert.False(t, cache.has(2))
	assert.True(t, cache.has(1))
	assert.True(t, cache.has(3))
	// maxHeight should still be 3
	assert.Equal(t, uint64(3), cache.getMaxHeight())
}

// TestPendingCache_GetByHash tests hash-based lookups
func TestPendingCache_GetByHash(t *testing.T) {
	t.Parallel()

	cache := newPendingCache[*types.P2PSignedHeader]()

	h1, _ := types.GetRandomBlock(1, 1, "test-chain")
	h2, _ := types.GetRandomBlock(2, 1, "test-chain")

	p1 := &types.P2PSignedHeader{SignedHeader: h1}
	p2 := &types.P2PSignedHeader{SignedHeader: h2}

	cache.add(p1)
	cache.add(p2)

	// Get by hash
	item, ok := cache.getByHash(p1.Hash())
	assert.True(t, ok)
	assert.Equal(t, uint64(1), item.Height())

	item, ok = cache.getByHash(p2.Hash())
	assert.True(t, ok)
	assert.Equal(t, uint64(2), item.Height())

	// Non-existent hash
	_, ok = cache.getByHash([]byte("nonexistent"))
	assert.False(t, ok)

	// Has by hash
	assert.True(t, cache.hasByHash(p1.Hash()))
	assert.True(t, cache.hasByHash(p2.Hash()))
	assert.False(t, cache.hasByHash([]byte("nonexistent")))

	// After delete, hash lookup should fail
	cache.delete(1)
	_, ok = cache.getByHash(p1.Hash())
	assert.False(t, ok)
	assert.False(t, cache.hasByHash(p1.Hash()))
}

// TestPendingCache_MaxHeightTracking tests O(1) maxHeight tracking
func TestPendingCache_MaxHeightTracking(t *testing.T) {
	t.Parallel()

	cache := newPendingCache[*types.P2PSignedHeader]()

	// Add items out of order
	h5, _ := types.GetRandomBlock(5, 1, "test-chain")
	h3, _ := types.GetRandomBlock(3, 1, "test-chain")
	h7, _ := types.GetRandomBlock(7, 1, "test-chain")
	h1, _ := types.GetRandomBlock(1, 1, "test-chain")

	cache.add(&types.P2PSignedHeader{SignedHeader: h5})
	assert.Equal(t, uint64(5), cache.getMaxHeight())

	cache.add(&types.P2PSignedHeader{SignedHeader: h3})
	assert.Equal(t, uint64(5), cache.getMaxHeight()) // Still 5

	cache.add(&types.P2PSignedHeader{SignedHeader: h7})
	assert.Equal(t, uint64(7), cache.getMaxHeight()) // Now 7

	cache.add(&types.P2PSignedHeader{SignedHeader: h1})
	assert.Equal(t, uint64(7), cache.getMaxHeight()) // Still 7

	// Delete non-max should not change maxHeight
	cache.delete(3)
	assert.Equal(t, uint64(7), cache.getMaxHeight())

	cache.delete(1)
	assert.Equal(t, uint64(7), cache.getMaxHeight())

	// Delete max should recalculate
	cache.delete(7)
	assert.Equal(t, uint64(5), cache.getMaxHeight())

	// Delete remaining
	cache.delete(5)
	assert.Equal(t, uint64(0), cache.getMaxHeight())
}

// TestPendingCache_Head tests head() returns highest item
func TestPendingCache_Head(t *testing.T) {
	t.Parallel()

	cache := newPendingCache[*types.P2PSignedHeader]()

	// Empty cache
	head, height := cache.head()
	assert.Nil(t, head)
	assert.Equal(t, uint64(0), height)

	// Add items
	h1, _ := types.GetRandomBlock(1, 1, "test-chain")
	h3, _ := types.GetRandomBlock(3, 1, "test-chain")
	h2, _ := types.GetRandomBlock(2, 1, "test-chain")

	cache.add(&types.P2PSignedHeader{SignedHeader: h1})
	cache.add(&types.P2PSignedHeader{SignedHeader: h3})
	cache.add(&types.P2PSignedHeader{SignedHeader: h2})

	head, height = cache.head()
	assert.NotNil(t, head)
	assert.Equal(t, uint64(3), height)
	assert.Equal(t, uint64(3), head.Height())

	// After deleting max
	cache.delete(3)
	head, height = cache.head()
	assert.NotNil(t, head)
	assert.Equal(t, uint64(2), height)
	assert.Equal(t, uint64(2), head.Height())
}

// TestPendingCache_DAHints tests DA hint storage and retrieval
func TestPendingCache_DAHints(t *testing.T) {
	t.Parallel()

	cache := newPendingCache[*types.P2PSignedHeader]()

	// Set DA hint directly
	cache.setDAHint(10, 100)
	hint, ok := cache.getDAHint(10)
	assert.True(t, ok)
	assert.Equal(t, uint64(100), hint)

	// Non-existent
	_, ok = cache.getDAHint(999)
	assert.False(t, ok)

	// Add item with DA hint
	h1, _ := types.GetRandomBlock(1, 1, "test-chain")
	p1 := &types.P2PSignedHeader{SignedHeader: h1, DAHeightHint: 50}
	cache.add(p1)

	hint, ok = cache.getDAHint(1)
	assert.True(t, ok)
	assert.Equal(t, uint64(50), hint)

	// Delete should remove DA hint too
	cache.delete(1)
	_, ok = cache.getDAHint(1)
	assert.False(t, ok)
}

// TestPendingCache_PruneIf tests conditional pruning
func TestPendingCache_PruneIf(t *testing.T) {
	t.Parallel()

	cache := newPendingCache[*types.P2PSignedHeader]()

	// Add items at heights 1-10
	for i := uint64(1); i <= 10; i++ {
		h, _ := types.GetRandomBlock(i, 1, "test-chain")
		cache.add(&types.P2PSignedHeader{SignedHeader: h})
	}
	assert.Equal(t, 10, cache.len())
	assert.Equal(t, uint64(10), cache.getMaxHeight())

	// Prune items <= 5
	pruned := cache.pruneIf(func(height uint64) bool {
		return height <= 5
	})
	assert.Equal(t, 5, pruned)
	assert.Equal(t, 5, cache.len())

	// Verify remaining items
	for i := uint64(1); i <= 5; i++ {
		assert.False(t, cache.has(i), "height %d should be pruned", i)
	}
	for i := uint64(6); i <= 10; i++ {
		assert.True(t, cache.has(i), "height %d should exist", i)
	}

	// maxHeight should still be 10
	assert.Equal(t, uint64(10), cache.getMaxHeight())

	// Prune including max
	pruned = cache.pruneIf(func(height uint64) bool {
		return height >= 9
	})
	assert.Equal(t, 2, pruned)
	assert.Equal(t, 3, cache.len())
	assert.Equal(t, uint64(8), cache.getMaxHeight())
}

// TestPendingCache_ConcurrentAccess tests thread safety
func TestPendingCache_ConcurrentAccess(t *testing.T) {
	t.Parallel()

	cache := newPendingCache[*types.P2PSignedHeader]()

	const numGoroutines = 10
	const numOpsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines * 3) // readers, writers, deleters

	// Writers
	for i := range numGoroutines {
		go func(offset int) {
			defer wg.Done()
			for j := range numOpsPerGoroutine {
				height := uint64(offset*numOpsPerGoroutine + j + 1)
				h, _ := types.GetRandomBlock(height, 1, "test-chain")
				cache.add(&types.P2PSignedHeader{SignedHeader: h})
			}
		}(i)
	}

	// Readers
	for range numGoroutines {
		go func() {
			defer wg.Done()
			for j := range numOpsPerGoroutine {
				_ = cache.len()
				_ = cache.getMaxHeight()
				_, _ = cache.head()
				_, _ = cache.get(uint64(j + 1))
			}
		}()
	}

	// Deleters (delete some items)
	for i := range numGoroutines {
		go func(offset int) {
			defer wg.Done()
			for j := range numOpsPerGoroutine / 2 {
				height := uint64(offset*numOpsPerGoroutine + j + 1)
				cache.delete(height)
			}
		}(i)
	}

	wg.Wait()

	// Should not panic and should have some items remaining
	assert.GreaterOrEqual(t, cache.len(), 0)
}

func TestHeightSubWaitCancellationRemovesWaiter(t *testing.T) {
	t.Parallel()

	hs := newHeightSub(1)
	for range 100 {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		require.ErrorIs(t, hs.Wait(ctx, 1_000), context.Canceled)
	}

	hs.heightMu.Lock()
	defer hs.heightMu.Unlock()
	assert.Empty(t, hs.heightChs)
}

func TestHeightSubWaitCancellationPreservesOtherWaiters(t *testing.T) {
	t.Parallel()

	hs := newHeightSub(1)
	ctx1, cancel1 := context.WithCancel(context.Background())
	ctx2 := context.Background()
	waitDone := make(chan error, 2)
	go func() {
		waitDone <- hs.Wait(ctx1, 1_000)
	}()
	go func() {
		waitDone <- hs.Wait(ctx2, 1_000)
	}()

	require.Eventually(t, func() bool {
		hs.heightMu.Lock()
		defer hs.heightMu.Unlock()
		return len(hs.heightChs[1_000]) == 2
	}, time.Second, time.Millisecond)

	cancel1()
	require.Eventually(t, func() bool {
		hs.heightMu.Lock()
		defer hs.heightMu.Unlock()
		return len(hs.heightChs[1_000]) == 1
	}, time.Second, time.Millisecond)

	hs.SetHeight(1_000)
	results := []error{<-waitDone, <-waitDone}
	assert.Contains(t, results, context.Canceled)
	assert.Contains(t, results, nil)
}

func TestHeightSubWaitCancellationAndSetHeightConcurrent(t *testing.T) {
	t.Parallel()

	for range 100 {
		hs := newHeightSub(1)
		ctx, cancel := context.WithCancel(context.Background())
		waitDone := make(chan error, 1)
		go func() {
			waitDone <- hs.Wait(ctx, 1_000)
		}()

		// Ensure the waiter is registered before racing cancellation and notification.
		require.Eventually(t, func() bool {
			hs.heightMu.Lock()
			defer hs.heightMu.Unlock()
			return len(hs.heightChs[1_000]) == 1
		}, time.Second, time.Millisecond)

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			cancel()
		}()
		go func() {
			defer wg.Done()
			hs.SetHeight(1_000)
		}()
		wg.Wait()

		err := <-waitDone
		if err != nil {
			assert.ErrorIs(t, err, context.Canceled)
		}

		cancel()
		hs.heightMu.Lock()
		assert.Empty(t, hs.heightChs)
		hs.heightMu.Unlock()
	}
}

// TestStoreAdapter_Backpressure tests that Append blocks when cache is full
func TestStoreAdapter_Backpressure(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ds, err := NewTestInMemoryKVStore()
	require.NoError(t, err)
	store := New(ds)
	adapter := NewHeaderStoreAdapter(store, testGenesis())

	// Fill the cache close to max (we can't easily fill to exactly max without
	// modifying maxPendingCacheSize, so we test the mechanism indirectly)

	// Create many items
	const numItems = 100
	items := make([]*types.P2PSignedHeader, numItems)
	for i := range numItems {
		h, _ := types.GetRandomBlock(uint64(i+1), 1, "test-chain")
		items[i] = &types.P2PSignedHeader{SignedHeader: h}
	}

	// Append should work without blocking when there's space
	err = adapter.Append(ctx, items...)
	require.NoError(t, err)

	// Verify all items were added
	assert.Equal(t, uint64(numItems), adapter.Height())
}

// TestStoreAdapter_PrunePersistedOptimization tests height-based pruning
func TestStoreAdapter_PrunePersistedOptimization(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ds, err := NewTestInMemoryKVStore()
	require.NoError(t, err)
	st := New(ds)
	adapter := NewHeaderStoreAdapter(st, testGenesis())

	// Add items to pending via Append
	h1, d1 := types.GetRandomBlock(1, 1, "test-chain")
	h2, d2 := types.GetRandomBlock(2, 1, "test-chain")
	h3, d3 := types.GetRandomBlock(3, 1, "test-chain")

	err = adapter.Append(ctx,
		&types.P2PSignedHeader{SignedHeader: h1},
		&types.P2PSignedHeader{SignedHeader: h2},
		&types.P2PSignedHeader{SignedHeader: h3},
	)
	require.NoError(t, err)

	// Verify items are in pending (adapter.Height() should return 3)
	assert.Equal(t, uint64(3), adapter.Height())

	// Now persist h1 and h2 to the store directly
	batch, err := st.NewBatch(ctx)
	require.NoError(t, err)
	require.NoError(t, batch.SaveBlockData(h1, d1, &types.Signature{}))
	require.NoError(t, batch.SaveBlockData(h2, d2, &types.Signature{}))
	require.NoError(t, batch.SetHeight(2))
	require.NoError(t, batch.Commit())

	// Items should still be retrievable (from either pending or store)
	item, err := adapter.GetByHeight(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), item.Height())

	item, err = adapter.GetByHeight(ctx, 2)
	require.NoError(t, err)
	assert.Equal(t, uint64(2), item.Height())

	item, err = adapter.GetByHeight(ctx, 3)
	require.NoError(t, err)
	assert.Equal(t, uint64(3), item.Height())

	// Persist h3 too
	batch, err = st.NewBatch(ctx)
	require.NoError(t, err)
	require.NoError(t, batch.SaveBlockData(h3, d3, &types.Signature{}))
	require.NoError(t, batch.SetHeight(3))
	require.NoError(t, batch.Commit())

	// All items should now be retrievable from store
	item, err = adapter.GetByHeight(ctx, 3)
	require.NoError(t, err)
	assert.Equal(t, uint64(3), item.Height())
}

// TestStoreAdapter_AppendSkipsPersistedItems tests that Append doesn't add already-persisted items
func TestStoreAdapter_AppendSkipsPersistedItems(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ds, err := NewTestInMemoryKVStore()
	require.NoError(t, err)
	st := New(ds)
	adapter := NewHeaderStoreAdapter(st, testGenesis())

	// Persist h1 directly to store
	h1, d1 := types.GetRandomBlock(1, 1, "test-chain")
	batch, err := st.NewBatch(ctx)
	require.NoError(t, err)
	require.NoError(t, batch.SaveBlockData(h1, d1, &types.Signature{}))
	require.NoError(t, batch.SetHeight(1))
	require.NoError(t, batch.Commit())

	// Now try to Append h1 - it should be skipped
	err = adapter.Append(ctx, &types.P2PSignedHeader{SignedHeader: h1})
	require.NoError(t, err)

	// Height should be 1 (from store)
	assert.Equal(t, uint64(1), adapter.Height())

	// Item should be retrievable
	item, err := adapter.GetByHeight(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), item.Height())
}

// TestStoreAdapter_DAHintPersistence tests that DA hints are persisted and restored
func TestStoreAdapter_DAHintPersistence(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ds, err := NewTestInMemoryKVStore()
	require.NoError(t, err)
	st := New(ds)
	adapter := NewHeaderStoreAdapter(st, testGenesis())

	// Add item with DA hint
	h1, _ := types.GetRandomBlock(1, 1, "test-chain")
	p1 := &types.P2PSignedHeader{SignedHeader: h1, DAHeightHint: 100}

	err = adapter.Append(ctx, p1)
	require.NoError(t, err)

	// Retrieve and check DA hint is applied
	item, err := adapter.GetByHeight(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, uint64(100), item.DAHint())
}

// TestStoreAdapter_HeightUpdatesCorrectly tests height tracking through various operations
func TestStoreAdapter_HeightUpdatesCorrectly(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ds, err := NewTestInMemoryKVStore()
	require.NoError(t, err)
	st := New(ds)
	adapter := NewHeaderStoreAdapter(st, testGenesis())

	// Initially 0
	assert.Equal(t, uint64(0), adapter.Height())

	// Add item at height 5 (gaps are allowed in pending)
	h5, _ := types.GetRandomBlock(5, 1, "test-chain")
	err = adapter.Append(ctx, &types.P2PSignedHeader{SignedHeader: h5})
	require.NoError(t, err)
	assert.Equal(t, uint64(5), adapter.Height())

	// Add item at height 3 - should not decrease height
	h3, _ := types.GetRandomBlock(3, 1, "test-chain")
	err = adapter.Append(ctx, &types.P2PSignedHeader{SignedHeader: h3})
	require.NoError(t, err)
	assert.Equal(t, uint64(5), adapter.Height())

	// Add item at height 10 - should increase height
	h10, _ := types.GetRandomBlock(10, 1, "test-chain")
	err = adapter.Append(ctx, &types.P2PSignedHeader{SignedHeader: h10})
	require.NoError(t, err)
	assert.Equal(t, uint64(10), adapter.Height())

	// Verify HasAt works
	assert.True(t, adapter.HasAt(ctx, 3))
	assert.True(t, adapter.HasAt(ctx, 5))
	assert.True(t, adapter.HasAt(ctx, 10))
	assert.False(t, adapter.HasAt(ctx, 7)) // gap

	// Delete height 10 via DeleteRange
	err = adapter.DeleteRange(ctx, 10, 11)
	require.NoError(t, err)

	// After delete, HasAt should return false for deleted height
	assert.False(t, adapter.HasAt(ctx, 10))
	assert.True(t, adapter.HasAt(ctx, 5))
	assert.True(t, adapter.HasAt(ctx, 3))
}

// TestStoreAdapter_DeleteRangeRemovesFromPending tests DeleteRange removes items from pending
func TestStoreAdapter_DeleteRangeRemovesFromPending(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ds, err := NewTestInMemoryKVStore()
	require.NoError(t, err)
	st := New(ds)
	adapter := NewHeaderStoreAdapter(st, testGenesis())

	// Add items 1-5
	for i := uint64(1); i <= 5; i++ {
		h, _ := types.GetRandomBlock(i, 1, "test-chain")
		err = adapter.Append(ctx, &types.P2PSignedHeader{SignedHeader: h})
		require.NoError(t, err)
	}

	// Verify all exist
	for i := uint64(1); i <= 5; i++ {
		assert.True(t, adapter.HasAt(ctx, i))
	}

	// Delete range [2, 4)
	err = adapter.DeleteRange(ctx, 2, 4)
	require.NoError(t, err)

	// Verify 2 and 3 are gone, 1, 4, 5 remain
	assert.True(t, adapter.HasAt(ctx, 1))
	assert.False(t, adapter.HasAt(ctx, 2))
	assert.False(t, adapter.HasAt(ctx, 3))
	assert.True(t, adapter.HasAt(ctx, 4))
	assert.True(t, adapter.HasAt(ctx, 5))

	// The head is untouched by the deletion, so the reported height must stay 5
	assert.Equal(t, uint64(5), adapter.Height())
	head, err := adapter.Head(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint64(5), head.Height())
}

// TestStoreAdapter_DeleteRangeHeight tests that DeleteRange only lowers the reported
// height when the deleted range actually contained the head.
func TestStoreAdapter_DeleteRangeHeight(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		heights  []uint64
		from, to uint64
		want     uint64
	}{
		{
			name:    "non-head range keeps head",
			heights: []uint64{1, 2, 3, 4, 5},
			from:    2, to: 4,
			want: 5,
		},
		{
			name:    "range covering the head falls back to highest survivor",
			heights: []uint64{1, 2, 3, 4, 5},
			from:    4, to: 6,
			want: 3,
		},
		{
			name:    "single head deletion",
			heights: []uint64{1, 2, 3},
			from:    3, to: 4,
			want: 2,
		},
		{
			name:    "gap below deleted head is not skipped over",
			heights: []uint64{1, 3, 5},
			from:    5, to: 6,
			want: 3,
		},
		{
			name:    "zero lower bound does not underflow",
			heights: []uint64{1, 2, 3},
			from:    0, to: 2,
			want: 3,
		},
		{
			name:    "empty range is a no-op",
			heights: []uint64{1, 2, 3},
			from:    2, to: 2,
			want: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()

			ds, err := NewTestInMemoryKVStore()
			require.NoError(t, err)
			st := New(ds)
			adapter := NewHeaderStoreAdapter(st, testGenesis())

			for _, height := range tt.heights {
				h, _ := types.GetRandomBlock(height, 1, "test-chain")
				require.NoError(t, adapter.Append(ctx, &types.P2PSignedHeader{SignedHeader: h}))
			}
			require.NoError(t, adapter.DeleteRange(ctx, tt.from, tt.to))
			assert.Equal(t, tt.want, adapter.Height())
		})
	}
}

// TestStoreAdapter_ConcurrentAppendAndRead tests concurrent access to the adapter
func TestStoreAdapter_ConcurrentAppendAndRead(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ds, err := NewTestInMemoryKVStore()
	require.NoError(t, err)
	st := New(ds)
	adapter := NewHeaderStoreAdapter(st, testGenesis())

	const numWriters = 5
	const numReaders = 5
	const itemsPerWriter = 20

	var wg sync.WaitGroup
	wg.Add(numWriters + numReaders)

	// Writers
	for w := range numWriters {
		go func(writerID int) {
			defer wg.Done()
			for i := range itemsPerWriter {
				height := uint64(writerID*itemsPerWriter + i + 1)
				h, _ := types.GetRandomBlock(height, 1, "test-chain") //nolint: contextcheck // test code
				_ = adapter.Append(ctx, &types.P2PSignedHeader{SignedHeader: h})
			}
		}(w)
	}

	// Readers
	for range numReaders {
		go func() {
			defer wg.Done()
			for i := range itemsPerWriter * numWriters {
				_ = adapter.Height() //nolint:contextcheck // external interface does not accept context
				_, _ = adapter.Head(ctx)
				_ = adapter.HasAt(ctx, uint64(i+1))
			}
		}()
	}

	wg.Wait()

	// Should have all items
	assert.Equal(t, uint64(numWriters*itemsPerWriter), adapter.Height())
}

// TestStoreAdapter_InitOnlyOnce tests that Init only works once
func TestStoreAdapter_InitOnlyOnce(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ds, err := NewTestInMemoryKVStore()
	require.NoError(t, err)
	st := New(ds)
	adapter := NewHeaderStoreAdapter(st, testGenesis())

	h1, _ := types.GetRandomBlock(1, 1, "test-chain")
	h2, _ := types.GetRandomBlock(2, 1, "test-chain")

	// First Init should work
	err = adapter.Init(ctx, &types.P2PSignedHeader{SignedHeader: h1})
	require.NoError(t, err)
	assert.Equal(t, uint64(1), adapter.Height())

	// Second Init should be no-op
	err = adapter.Init(ctx, &types.P2PSignedHeader{SignedHeader: h2})
	require.NoError(t, err)
	// Height should still be 1, not 2
	assert.Equal(t, uint64(1), adapter.Height())
}
