package block

import (
	"context"
	"sort"
	"testing"

	test "github.com/rollkit/rollkit/test/log"

	"github.com/rollkit/rollkit/store"

	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/types"
)

func TestGetPendingBlocks(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()
	pb := newPendingBlocks(t)
	for i := uint64(0); i < 5; i++ {
		require.NoError(pb.store.SaveBlock(ctx, types.GetRandomBlock(i, 0), &types.Commit{}))
		pb.store.SetHeight(ctx, i)
	}
	blocks, _ := pb.getPendingBlocks(ctx)
	require.True(sort.SliceIsSorted(blocks, func(i, j int) bool {
		return blocks[i].Height() < blocks[j].Height()
	}))
}

func TestRemoveSubmittedBlocks(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()
	pb := newPendingBlocks(t)
	const nBlocks = 5
	for i := uint64(1); i <= nBlocks; i++ {
		require.NoError(pb.store.SaveBlock(ctx, types.GetRandomBlock(i, 0), &types.Commit{}))
		pb.store.SetHeight(ctx, i)
	}
	pb.setLastSubmittedHeight(nBlocks)
	require.True(pb.isEmpty())
}

func TestRemoveSubsetOfBlocks(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()
	pb := newPendingBlocks(t)
	for i := uint64(1); i <= 5; i++ {
		require.NoError(pb.store.SaveBlock(ctx, types.GetRandomBlock(i, 0), &types.Commit{}))
		pb.store.SetHeight(ctx, i)
	}
	// Remove blocks with height 1 and 2
	pb.setLastSubmittedHeight(2)
	remainingBlocks, err := pb.getPendingBlocks(ctx)
	require.NoError(err)
	require.Len(remainingBlocks, 3, "There should be 3 blocks remaining")
	for _, block := range remainingBlocks {
		require.Contains([]uint64{3, 4, 5}, block.Height(), "Only blocks with height 3, 4 and 5 should remain")
	}
}

func TestRemoveAllBlocksAndVerifyEmpty(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()
	pb := newPendingBlocks(t)
	for i := uint64(0); i < 5; i++ {
		require.NoError(pb.store.SaveBlock(ctx, types.GetRandomBlock(i, 0), &types.Commit{}))
		pb.store.SetHeight(ctx, i)
	}
	// Remove all blocks
	blocks, err := pb.getPendingBlocks(ctx)
	require.NoError(err)
	pb.setLastSubmittedHeight(blocks[len(blocks)-1].Height())
	require.True(pb.isEmpty(), "PendingBlocks should be empty after removing all blocks")
}

func TestRemoveBlocksFromEmptyPendingBlocks(t *testing.T) {
	require := require.New(t)
	pb := newPendingBlocks(t)
	// Attempt to remove blocks from an empty PendingBlocks
	require.NotPanics(func() {
		pb.setLastSubmittedHeight(2)
	}, "Removing blocks from an empty PendingBlocks should not cause a panic")
}

func newPendingBlocks(t *testing.T) *PendingBlocks {
	kv, err := store.NewDefaultInMemoryKVStore()
	require.NoError(t, err)
	pendingBlocks, err := NewPendingBlocks(store.New(kv), test.NewLogger(t))
	require.NoError(t, err)
	return pendingBlocks
}
