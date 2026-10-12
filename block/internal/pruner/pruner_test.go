package pruner

import (
	"context"
	"testing"
	"time"

	ds "github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/evstack/ev-node/pkg/config"
	"github.com/evstack/ev-node/pkg/store"
	"github.com/evstack/ev-node/types"
)

type execMetaAdapter struct {
	existing map[uint64]struct{}
}

func (e *execMetaAdapter) PruneExec(ctx context.Context, height uint64) error {
	for h := range e.existing {
		if h < height {
			delete(e.existing, h)
		}
	}

	return nil
}

func TestPrunerPruneMetadata(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	kv := dssync.MutexWrap(ds.NewMapDatastore())
	stateStore := store.New(kv)

	for height := uint64(1); height <= 5; height++ {
		batch, err := stateStore.NewBatch(ctx)
		require.NoError(t, err)
		require.NoError(t, batch.SetHeight(height))
		require.NoError(t, batch.UpdateState(types.State{LastBlockHeight: height}))
		require.NoError(t, batch.Commit())
	}

	execAdapter := &execMetaAdapter{existing: map[uint64]struct{}{1: {}, 2: {}, 3: {}}}
	cfg := config.PruningConfig{
		Mode:       config.PruningModeMetadata,
		Interval:   config.DurationWrapper{Duration: 1 * time.Second},
		KeepRecent: 1,
	}

	pruner := New(zerolog.New(zerolog.NewTestWriter(t)), stateStore, execAdapter, cfg, 100*time.Millisecond, "") // Empty DA address
	pruner.ctx = ctx
	require.NoError(t, pruner.pruneMetadata())

	_, err := stateStore.GetStateAtHeight(ctx, 3)
	require.ErrorIs(t, err, ds.ErrNotFound)

	// State 4 is kept so the oldest retained block (5) can be replayed.
	_, err = stateStore.GetStateAtHeight(ctx, 4)
	require.NoError(t, err)

	_, err = stateStore.GetStateAtHeight(ctx, 5)
	require.NoError(t, err)

	_, exists := execAdapter.existing[1]
	require.False(t, exists)
}

func TestPrunerPruneBlocksWithoutDA(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	kv := dssync.MutexWrap(ds.NewMapDatastore())
	stateStore := store.New(kv)

	// Create blocks without setting DAIncludedHeightKey (simulating node without DA)
	for height := uint64(1); height <= 100; height++ {
		header := &types.SignedHeader{Header: types.Header{BaseHeader: types.BaseHeader{Height: height}}}
		data := &types.Data{}
		sig := types.Signature([]byte{byte(height)})

		batch, err := stateStore.NewBatch(ctx)
		require.NoError(t, err)
		require.NoError(t, batch.SaveBlockData(header, data, &sig))
		require.NoError(t, batch.SetHeight(height))
		require.NoError(t, batch.UpdateState(types.State{LastBlockHeight: height}))
		require.NoError(t, batch.Commit())
	}

	execAdapter := &execMetaAdapter{existing: make(map[uint64]struct{})}
	for h := uint64(1); h <= 100; h++ {
		execAdapter.existing[h] = struct{}{}
	}

	// Test with empty DA address (DA disabled) - should prune successfully
	cfg := config.PruningConfig{
		Mode:       config.PruningModeAll,
		Interval:   config.DurationWrapper{Duration: 1 * time.Second},
		KeepRecent: 10,
	}

	pruner := New(zerolog.New(zerolog.NewTestWriter(t)), stateStore, execAdapter, cfg, 100*time.Millisecond, "") // Empty DA address = DA disabled
	pruner.ctx = ctx
	require.NoError(t, pruner.pruneBlocks())

	// The per-tick budget (at least maxPruneChunk) covers the whole range,
	// so everything up to height - KeepRecent = 90 is pruned in one tick.
	height, err := stateStore.Height(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(100), height)

	for h := uint64(1); h <= 90; h++ {
		_, _, err := stateStore.GetBlockData(ctx, h)
		require.Error(t, err, "expected block data at height %d to be pruned", h)
	}
	for h := uint64(91); h <= 100; h++ {
		_, _, err := stateStore.GetBlockData(ctx, h)
		require.NoError(t, err, "expected block data at height %d to be kept", h)
	}

	// Verify exec metadata was also pruned (strictly less than 90)
	for h := uint64(1); h < 90; h++ {
		_, exists := execAdapter.existing[h]
		require.False(t, exists, "expected exec metadata at height %d to be pruned", h)
	}

	// Verify state snapshots were pruned below 90; state 90 is kept to replay block 91.
	for h := uint64(1); h < 90; h++ {
		_, err := stateStore.GetStateAtHeight(ctx, h)
		require.ErrorIs(t, err, ds.ErrNotFound, "expected state at height %d to be pruned", h)
	}
	for h := uint64(90); h <= 100; h++ {
		_, err := stateStore.GetStateAtHeight(ctx, h)
		require.NoError(t, err, "expected state at height %d to be kept", h)
	}
}

func TestPrunerPruneMetadataAfterBlockPruning(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	kv := dssync.MutexWrap(ds.NewMapDatastore())
	stateStore := store.New(kv)

	for height := uint64(1); height <= 20; height++ {
		batch, err := stateStore.NewBatch(ctx)
		require.NoError(t, err)
		require.NoError(t, batch.SetHeight(height))
		require.NoError(t, batch.UpdateState(types.State{LastBlockHeight: height}))
		require.NoError(t, batch.Commit())
	}

	// Simulate a node that previously pruned blocks without pruning states.
	require.NoError(t, stateStore.SetMetadata(ctx, store.LastPrunedBlockHeightKey, []byte{10, 0, 0, 0, 0, 0, 0, 0}))

	cfg := config.PruningConfig{
		Mode:       config.PruningModeMetadata,
		Interval:   config.DurationWrapper{Duration: 1 * time.Second},
		KeepRecent: 5,
	}

	pruner := New(zerolog.New(zerolog.NewTestWriter(t)), stateStore, nil, cfg, 100*time.Millisecond, "")
	pruner.ctx = ctx
	require.NoError(t, pruner.pruneMetadata())

	for h := uint64(1); h <= 14; h++ {
		_, err := stateStore.GetStateAtHeight(ctx, h)
		require.ErrorIs(t, err, ds.ErrNotFound, "expected state at height %d to be pruned", h)
	}
	for h := uint64(15); h <= 20; h++ {
		_, err := stateStore.GetStateAtHeight(ctx, h)
		require.NoError(t, err, "expected state at height %d to be kept", h)
	}
}

func TestPrunerPruneStatesBudgetAndChunks(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	kv := dssync.MutexWrap(ds.NewMapDatastore())
	stateStore := store.New(kv)

	const total = 30_000
	batch, err := stateStore.NewBatch(ctx)
	require.NoError(t, err)
	for height := uint64(1); height <= total; height++ {
		require.NoError(t, batch.UpdateState(types.State{LastBlockHeight: height}))
	}
	require.NoError(t, batch.SetHeight(total))
	require.NoError(t, batch.Commit())

	// 1ms blocks with a 6s interval: 6,000 blocks per interval, budget 24,000,
	// so the first tick spans three store chunks (10k, 10k, 4k).
	cfg := config.PruningConfig{
		Mode:       config.PruningModeMetadata,
		Interval:   config.DurationWrapper{Duration: 6 * time.Second},
		KeepRecent: 1,
	}
	pruner := New(zerolog.New(zerolog.NewTestWriter(t)), stateStore, nil, cfg, time.Millisecond, "")
	pruner.ctx = ctx
	require.Equal(t, uint64(24_000), pruner.pruneBudget())

	require.NoError(t, pruner.pruneMetadata())
	lastPruned, err := pruner.getLastPrunedStateHeight(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(24_000), lastPruned)
	_, err = stateStore.GetStateAtHeight(ctx, 24_000)
	require.ErrorIs(t, err, ds.ErrNotFound)
	_, err = stateStore.GetStateAtHeight(ctx, 24_001)
	require.NoError(t, err)

	// Second tick drains the rest, keeping the state for the oldest retained block.
	require.NoError(t, pruner.pruneMetadata())
	lastPruned, err = pruner.getLastPrunedStateHeight(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(total-2), lastPruned)
	_, err = stateStore.GetStateAtHeight(ctx, total-1)
	require.NoError(t, err)
}

func TestPrunerPruneBlocksWithDAEnabled(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	kv := dssync.MutexWrap(ds.NewMapDatastore())
	stateStore := store.New(kv)

	// Create blocks without setting DAIncludedHeightKey
	for height := uint64(1); height <= 100; height++ {
		header := &types.SignedHeader{Header: types.Header{BaseHeader: types.BaseHeader{Height: height}}}
		data := &types.Data{}
		sig := types.Signature([]byte{byte(height)})

		batch, err := stateStore.NewBatch(ctx)
		require.NoError(t, err)
		require.NoError(t, batch.SaveBlockData(header, data, &sig))
		require.NoError(t, batch.SetHeight(height))
		require.NoError(t, batch.UpdateState(types.State{LastBlockHeight: height}))
		require.NoError(t, batch.Commit())
	}

	// Test with DA address provided (DA enabled) - should skip pruning when DA height is not available
	cfg := config.PruningConfig{
		Mode:       config.PruningModeAll,
		Interval:   config.DurationWrapper{Duration: 1 * time.Second},
		KeepRecent: 10,
	}

	pruner := New(zerolog.New(zerolog.NewTestWriter(t)), stateStore, nil, cfg, 100*time.Millisecond, "localhost:1234") // DA enabled
	pruner.ctx = ctx
	// Should return nil (skip pruning) since DA height is not available
	require.NoError(t, pruner.pruneBlocks())

	// Verify no blocks were pruned (all blocks should still be retrievable)
	for h := uint64(1); h <= 100; h++ {
		_, _, err := stateStore.GetBlockData(ctx, h)
		require.NoError(t, err, "expected block data at height %d to still exist (no pruning should have happened)", h)
	}
}
