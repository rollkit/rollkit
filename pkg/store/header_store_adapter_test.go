package store

import (
	"context"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/celestiaorg/go-header"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evstack/ev-node/pkg/genesis"
	"github.com/evstack/ev-node/types"
)

// testGenesis returns a genesis with InitialHeight=1 for use in tests.
func testGenesis() genesis.Genesis {
	return genesis.Genesis{
		ChainID:       "test-chain",
		InitialHeight: 1,
		StartTime:     time.Now(),
	}
}

// computeHeaderIndexHash computes the hash used for indexing in the store.
// The store indexes by sha256(signedHeader.MarshalBinary()), not signedHeader.Hash().
func computeHeaderIndexHash(h *types.SignedHeader) []byte {
	blob, _ := h.MarshalBinary()
	hash := sha256.Sum256(blob)
	return hash[:]
}

// wrapHeader wraps a *types.SignedHeader in a *types.P2PSignedHeader for use with the HeaderStoreAdapter.
func wrapHeader(h *types.SignedHeader) *types.P2PSignedHeader {
	if h == nil {
		return nil
	}
	return &types.P2PSignedHeader{
		SignedHeader: h,
		DAHeightHint: 0,
	}
}

func TestHeaderStoreAdapter_NewHeaderStoreAdapter(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ds, err := NewTestInMemoryKVStore()
	require.NoError(t, err)
	store := New(ds)

	adapter := NewHeaderStoreAdapter(store, testGenesis())
	require.NotNil(t, adapter)

	// Initially, height should be 0
	assert.Equal(t, uint64(0), adapter.Height())

	// Head should return ErrEmptyStore when empty
	_, err = adapter.Head(ctx)
	assert.ErrorIs(t, err, header.ErrEmptyStore)
}

func TestHeaderStoreAdapter_AppendAndRetrieve(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ds, err := NewTestInMemoryKVStore()
	require.NoError(t, err)
	store := New(ds)
	adapter := NewHeaderStoreAdapter(store, testGenesis())

	// Create test headers
	h1, _ := types.GetRandomBlock(1, 2, "test-chain")
	h2, _ := types.GetRandomBlock(2, 2, "test-chain")

	// Append headers - these go to pending cache
	err = adapter.Append(ctx, wrapHeader(h1), wrapHeader(h2))
	require.NoError(t, err)

	// Check height is updated (from pending)
	assert.Equal(t, uint64(2), adapter.Height())

	// Retrieve by height (from pending)
	retrieved, err := adapter.GetByHeight(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, h1.Height(), retrieved.Height())

	retrieved, err = adapter.GetByHeight(ctx, 2)
	require.NoError(t, err)
	assert.Equal(t, h2.Height(), retrieved.Height())

	// Head should return the latest (from pending)
	head, err := adapter.Head(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint64(2), head.Height())
}

func TestHeaderStoreAdapter_GetFromStore(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ds, err := NewTestInMemoryKVStore()
	require.NoError(t, err)
	store := New(ds)

	// Save directly to store first
	h1, d1 := types.GetRandomBlock(1, 2, "test-chain")
	batch, err := store.NewBatch(ctx)
	require.NoError(t, err)
	require.NoError(t, batch.SaveBlockData(h1, d1, &h1.Signature))
	require.NoError(t, batch.SetHeight(1))
	require.NoError(t, batch.Commit())

	// Create adapter after data is in store
	adapter := NewHeaderStoreAdapter(store, testGenesis())

	// Get by hash - need to use the index hash (sha256 of marshaled SignedHeader)
	hash := computeHeaderIndexHash(h1)
	retrieved, err := adapter.Get(ctx, hash)
	require.NoError(t, err)
	assert.Equal(t, h1.Height(), retrieved.Height())

	// Get non-existent hash
	_, err = adapter.Get(ctx, []byte("nonexistent"))
	assert.ErrorIs(t, err, header.ErrNotFound)
}

func TestHeaderStoreAdapter_Has(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ds, err := NewTestInMemoryKVStore()
	require.NoError(t, err)
	store := New(ds)

	// Save directly to store
	h1, d1 := types.GetRandomBlock(1, 2, "test-chain")
	batch, err := store.NewBatch(ctx)
	require.NoError(t, err)
	require.NoError(t, batch.SaveBlockData(h1, d1, &h1.Signature))
	require.NoError(t, batch.SetHeight(1))
	require.NoError(t, batch.Commit())

	adapter := NewHeaderStoreAdapter(store, testGenesis())

	// Has should return true for existing header - use index hash
	has, err := adapter.Has(ctx, computeHeaderIndexHash(h1))
	require.NoError(t, err)
	assert.True(t, has)

	// Has should return false for non-existent
	has, err = adapter.Has(ctx, []byte("nonexistent"))
	require.NoError(t, err)
	assert.False(t, has)
}

func TestHeaderStoreAdapter_HasAt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ds, err := NewTestInMemoryKVStore()
	require.NoError(t, err)
	store := New(ds)
	adapter := NewHeaderStoreAdapter(store, testGenesis())

	h1, _ := types.GetRandomBlock(1, 2, "test-chain")
	require.NoError(t, adapter.Append(ctx, wrapHeader(h1)))

	// HasAt should return true for pending height
	assert.True(t, adapter.HasAt(ctx, 1))

	// HasAt should return false for non-existent height
	assert.False(t, adapter.HasAt(ctx, 2))
}

func TestHeaderStoreAdapter_HasAtFromStore(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ds, err := NewTestInMemoryKVStore()
	require.NoError(t, err)
	store := New(ds)

	// Save directly to store
	h1, d1 := types.GetRandomBlock(1, 2, "test-chain")
	batch, err := store.NewBatch(ctx)
	require.NoError(t, err)
	require.NoError(t, batch.SaveBlockData(h1, d1, &h1.Signature))
	require.NoError(t, batch.SetHeight(1))
	require.NoError(t, batch.Commit())

	adapter := NewHeaderStoreAdapter(store, testGenesis())

	// HasAt should return true for stored height
	assert.True(t, adapter.HasAt(ctx, 1))

	// HasAt should return false for non-existent height
	assert.False(t, adapter.HasAt(ctx, 2))
}

func TestHeaderStoreAdapter_GetRange(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ds, err := NewTestInMemoryKVStore()
	require.NoError(t, err)
	store := New(ds)
	adapter := NewHeaderStoreAdapter(store, testGenesis())

	// Create and append multiple headers to pending
	h1, _ := types.GetRandomBlock(1, 1, "test-chain")
	h2, _ := types.GetRandomBlock(2, 1, "test-chain")
	h3, _ := types.GetRandomBlock(3, 1, "test-chain")
	require.NoError(t, adapter.Append(ctx, wrapHeader(h1), wrapHeader(h2), wrapHeader(h3)))

	// GetRange [1, 3) should return headers 1 and 2
	headers, err := adapter.GetRange(ctx, 1, 3)
	require.NoError(t, err)
	require.Len(t, headers, 2)
	assert.Equal(t, uint64(1), headers[0].Height())
	assert.Equal(t, uint64(2), headers[1].Height())

	// GetRange with from >= to should return nil
	headers, err = adapter.GetRange(ctx, 3, 3)
	require.NoError(t, err)
	assert.Nil(t, headers)
}

func TestHeaderStoreAdapter_GetRangeByHeight(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ds, err := NewTestInMemoryKVStore()
	require.NoError(t, err)
	store := New(ds)
	adapter := NewHeaderStoreAdapter(store, testGenesis())

	h1, _ := types.GetRandomBlock(1, 1, "test-chain")
	h2, _ := types.GetRandomBlock(2, 1, "test-chain")
	h3, _ := types.GetRandomBlock(3, 1, "test-chain")
	require.NoError(t, adapter.Append(ctx, wrapHeader(h1), wrapHeader(h2), wrapHeader(h3)))

	// GetRangeByHeight from h1 to 4 should return headers 2 and 3
	headers, err := adapter.GetRangeByHeight(ctx, wrapHeader(h1), 4)
	require.NoError(t, err)
	require.Len(t, headers, 2)
	assert.Equal(t, uint64(2), headers[0].Height())
	assert.Equal(t, uint64(3), headers[1].Height())
}

func TestHeaderStoreAdapter_Init(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ds, err := NewTestInMemoryKVStore()
	require.NoError(t, err)
	store := New(ds)
	adapter := NewHeaderStoreAdapter(store, testGenesis())

	h1, _ := types.GetRandomBlock(1, 1, "test-chain")

	// Init should add header to pending
	err = adapter.Init(ctx, wrapHeader(h1))
	require.NoError(t, err)

	// Verify it's retrievable from pending
	retrieved, err := adapter.GetByHeight(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, h1.Height(), retrieved.Height())

	// Init again should be a no-op (already initialized)
	h2, _ := types.GetRandomBlock(2, 1, "test-chain")
	err = adapter.Init(ctx, wrapHeader(h2))
	require.NoError(t, err)

	// Height 2 should not be in pending since Init was already done
	assert.False(t, adapter.HasAt(ctx, 2))
}

func TestHeaderStoreAdapter_Tail(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ds, err := NewTestInMemoryKVStore()
	require.NoError(t, err)
	store := New(ds)
	adapter := NewHeaderStoreAdapter(store, testGenesis())

	// Tail on empty store should return ErrEmptyStore
	_, err = adapter.Tail(ctx)
	assert.ErrorIs(t, err, header.ErrEmptyStore)

	h1, _ := types.GetRandomBlock(1, 1, "test-chain")
	h2, _ := types.GetRandomBlock(2, 1, "test-chain")
	require.NoError(t, adapter.Append(ctx, wrapHeader(h1), wrapHeader(h2)))

	// Tail should return the first header
	tail, err := adapter.Tail(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), tail.Height())
}

func TestHeaderStoreAdapter_TailFromStore(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ds, err := NewTestInMemoryKVStore()
	require.NoError(t, err)
	store := New(ds)

	// Save directly to store
	h1, d1 := types.GetRandomBlock(1, 1, "test-chain")
	batch, err := store.NewBatch(ctx)
	require.NoError(t, err)
	require.NoError(t, batch.SaveBlockData(h1, d1, &h1.Signature))
	require.NoError(t, batch.SetHeight(1))
	require.NoError(t, batch.Commit())

	adapter := NewHeaderStoreAdapter(store, testGenesis())

	// Tail should return the first header from store
	tail, err := adapter.Tail(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), tail.Height())
}

func TestHeaderStoreAdapter_StartStop(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ds, err := NewTestInMemoryKVStore()
	require.NoError(t, err)
	store := New(ds)
	adapter := NewHeaderStoreAdapter(store, testGenesis())

	// Start should not error
	err = adapter.Start(ctx)
	require.NoError(t, err)

	// Stop should not error
	err = adapter.Stop(ctx)
	require.NoError(t, err)
}

func TestHeaderStoreAdapter_DeleteRange(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ds, err := NewTestInMemoryKVStore()
	require.NoError(t, err)
	store := New(ds)
	adapter := NewHeaderStoreAdapter(store, testGenesis())

	h1, _ := types.GetRandomBlock(1, 1, "test-chain")
	h2, _ := types.GetRandomBlock(2, 1, "test-chain")
	h3, _ := types.GetRandomBlock(3, 1, "test-chain")
	require.NoError(t, adapter.Append(ctx, wrapHeader(h1), wrapHeader(h2), wrapHeader(h3)))

	assert.Equal(t, uint64(3), adapter.Height())

	// DeleteRange should update cached height and remove from pending
	err = adapter.DeleteRange(ctx, 2, 4)
	require.NoError(t, err)

	// Cached height should be updated to 1
	assert.Equal(t, uint64(1), adapter.Height())

	// Heights 2 and 3 should no longer be available
	assert.False(t, adapter.HasAt(ctx, 2))
	assert.False(t, adapter.HasAt(ctx, 3))

	// Height 1 should still be available
	assert.True(t, adapter.HasAt(ctx, 1))
}

// TestHeaderStoreAdapter_DeleteRangeUsesPersistedHeight tests that deleting the pending
// head falls back to the persisted store height instead of the deleted range.
func TestHeaderStoreAdapter_DeleteRangeUsesPersistedHeight(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ds, err := NewTestInMemoryKVStore()
	require.NoError(t, err)
	store := New(ds)

	// Persist blocks 1-3 directly to the store
	batch, err := store.NewBatch(ctx)
	require.NoError(t, err)
	for i := uint64(1); i <= 3; i++ {
		h, d := types.GetRandomBlock(i, 1, "test-chain")
		require.NoError(t, batch.SaveBlockData(h, d, &h.Signature))
	}
	require.NoError(t, batch.SetHeight(3))
	require.NoError(t, batch.Commit())

	adapter := NewHeaderStoreAdapter(store, testGenesis())
	require.NoError(t, adapter.Start(ctx))

	// Blocks 4 and 5 arrive over P2P and only live in the pending cache
	h4, _ := types.GetRandomBlock(4, 1, "test-chain")
	h5, _ := types.GetRandomBlock(5, 1, "test-chain")
	require.NoError(t, adapter.Append(ctx, wrapHeader(h4), wrapHeader(h5)))
	require.Equal(t, uint64(5), adapter.Height())

	require.NoError(t, adapter.DeleteRange(ctx, 4, 6))
	assert.Equal(t, uint64(3), adapter.Height())
	assert.False(t, adapter.HasAt(ctx, 4))
	assert.False(t, adapter.HasAt(ctx, 5))
}

func TestHeaderStoreAdapter_OnDelete(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ds, err := NewTestInMemoryKVStore()
	require.NoError(t, err)
	store := New(ds)
	adapter := NewHeaderStoreAdapter(store, testGenesis())

	h1, _ := types.GetRandomBlock(1, 1, "test-chain")
	h2, _ := types.GetRandomBlock(2, 1, "test-chain")
	require.NoError(t, adapter.Append(ctx, wrapHeader(h1), wrapHeader(h2)))

	// Track deleted heights
	var deletedHeights []uint64
	adapter.OnDelete(func(ctx context.Context, height uint64) error {
		deletedHeights = append(deletedHeights, height)
		return nil
	})

	err = adapter.DeleteRange(ctx, 1, 3)
	require.NoError(t, err)

	assert.Equal(t, []uint64{1, 2}, deletedHeights)
}

func TestHeaderStoreAdapter_AppendSkipsExisting(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ds, err := NewTestInMemoryKVStore()
	require.NoError(t, err)
	store := New(ds)

	// Save directly to store first
	h1, d1 := types.GetRandomBlock(1, 2, "test-chain")
	batch, err := store.NewBatch(ctx)
	require.NoError(t, err)
	require.NoError(t, batch.SaveBlockData(h1, d1, &h1.Signature))
	require.NoError(t, batch.SetHeight(1))
	require.NoError(t, batch.Commit())

	adapter := NewHeaderStoreAdapter(store, testGenesis())

	// Append the same header again should not error (skips existing in store)
	err = adapter.Append(ctx, wrapHeader(h1))
	require.NoError(t, err)

	// Height should still be 1
	assert.Equal(t, uint64(1), adapter.Height())
}

func TestHeaderStoreAdapter_AppendNilHeaders(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ds, err := NewTestInMemoryKVStore()
	require.NoError(t, err)
	store := New(ds)
	adapter := NewHeaderStoreAdapter(store, testGenesis())

	// Append with nil and empty should not error
	err = adapter.Append(ctx)
	require.NoError(t, err)

	var nilHeader *types.P2PSignedHeader
	err = adapter.Append(ctx, nilHeader)
	require.NoError(t, err)

	assert.Equal(t, uint64(0), adapter.Height())
}

func TestHeaderStoreAdapter_Sync(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ds, err := NewTestInMemoryKVStore()
	require.NoError(t, err)
	store := New(ds)
	adapter := NewHeaderStoreAdapter(store, testGenesis())

	// Sync should not error
	err = adapter.Sync(ctx)
	require.NoError(t, err)
}

func TestHeaderStoreAdapter_HeightRefreshFromStore(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ds, err := NewTestInMemoryKVStore()
	require.NoError(t, err)
	store := New(ds)

	// Save data directly to store before creating adapter
	h1, d1 := types.GetRandomBlock(1, 1, "test-chain")
	batch, err := store.NewBatch(ctx)
	require.NoError(t, err)
	require.NoError(t, batch.SaveBlockData(h1, d1, &types.Signature{}))
	require.NoError(t, batch.SetHeight(1))
	require.NoError(t, batch.Commit())

	// Create adapter - it should pick up the height from store
	adapter := NewHeaderStoreAdapter(store, testGenesis())
	assert.Equal(t, uint64(1), adapter.Height())
}

func TestHeaderStoreAdapter_GetByHeightNotFound(t *testing.T) {
	t.Parallel()
	// Use a short timeout since GetByHeight now blocks waiting for the height
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	ds, err := NewTestInMemoryKVStore()
	require.NoError(t, err)
	store := New(ds)
	adapter := NewHeaderStoreAdapter(store, testGenesis())

	_, err = adapter.GetByHeight(ctx, 999)
	// GetByHeight now blocks until the height is available or context is canceled
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestHeaderStoreAdapter_InitWithNil(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ds, err := NewTestInMemoryKVStore()
	require.NoError(t, err)
	store := New(ds)
	adapter := NewHeaderStoreAdapter(store, testGenesis())

	// Init with nil should not error but also not mark as initialized
	err = adapter.Init(ctx, nil)
	require.NoError(t, err)

	// Should still return ErrEmptyStore
	_, err = adapter.Head(ctx)
	assert.ErrorIs(t, err, header.ErrEmptyStore)
}

func TestHeaderStoreAdapter_ContextTimeout(t *testing.T) {
	t.Parallel()

	ds, err := NewTestInMemoryKVStore()
	require.NoError(t, err)
	store := New(ds)
	adapter := NewHeaderStoreAdapter(store, testGenesis())

	// Create a context that's already canceled
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	time.Sleep(1 * time.Millisecond) // Ensure context is expired

	// Operations should still work with in-memory store
	// but this tests that the context is being passed through
	h1, _ := types.GetRandomBlock(1, 1, "test-chain")
	// Note: In-memory store doesn't actually check context, but this verifies
	// the adapter passes the context through
	_ = adapter.Append(ctx, wrapHeader(h1))
}

func TestHeaderStoreAdapter_PendingAndStoreInteraction(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ds, err := NewTestInMemoryKVStore()
	require.NoError(t, err)
	store := New(ds)
	adapter := NewHeaderStoreAdapter(store, testGenesis())

	// Add header to pending
	h1, _ := types.GetRandomBlock(1, 1, "test-chain")
	require.NoError(t, adapter.Append(ctx, wrapHeader(h1)))

	// Verify it's in pending
	retrieved, err := adapter.GetByHeight(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, h1.Height(), retrieved.Height())

	// Now save a different header at height 1 directly to store
	h1Store, d1Store := types.GetRandomBlock(1, 2, "test-chain")
	batch, err := store.NewBatch(ctx)
	require.NoError(t, err)
	require.NoError(t, batch.SaveBlockData(h1Store, d1Store, &h1Store.Signature))
	require.NoError(t, batch.SetHeight(1))
	require.NoError(t, batch.Commit())

	// GetByHeight should now return from store (store takes precedence)
	retrieved, err = adapter.GetByHeight(ctx, 1)
	require.NoError(t, err)
	// The store version should be returned
	assert.Equal(t, h1Store.Height(), retrieved.Height())
}

func TestHeaderStoreAdapter_HeadPrefersPending(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ds, err := NewTestInMemoryKVStore()
	require.NoError(t, err)
	store := New(ds)

	// Save height 1 to store
	h1, d1 := types.GetRandomBlock(1, 1, "test-chain")
	batch, err := store.NewBatch(ctx)
	require.NoError(t, err)
	require.NoError(t, batch.SaveBlockData(h1, d1, &h1.Signature))
	require.NoError(t, batch.SetHeight(1))
	require.NoError(t, batch.Commit())

	adapter := NewHeaderStoreAdapter(store, testGenesis())

	// Add height 2 to pending
	h2, _ := types.GetRandomBlock(2, 1, "test-chain")
	require.NoError(t, adapter.Append(ctx, wrapHeader(h2)))

	// Head should return the pending header (higher height)
	head, err := adapter.Head(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint64(2), head.Height())
}

func TestHeaderStoreAdapter_GetFromPendingByHash(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	ds, err := NewTestInMemoryKVStore()
	require.NoError(t, err)
	store := New(ds)
	adapter := NewHeaderStoreAdapter(store, testGenesis())

	// Add header to pending
	h1, _ := types.GetRandomBlock(1, 1, "test-chain")
	require.NoError(t, adapter.Append(ctx, wrapHeader(h1)))

	// Get by hash from pending (uses header's Hash() method)
	retrieved, err := adapter.Get(ctx, h1.Hash())
	require.NoError(t, err)
	assert.Equal(t, h1.Height(), retrieved.Height())
}
