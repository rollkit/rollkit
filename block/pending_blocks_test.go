package block

import (
	"context"
	"sort"
	"testing"

	"github.com/rollkit/rollkit/store"

	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/types"
)

func TestGetPendingBlocks(t *testing.T) {
	require := require.New(t)
	pb := newPendingBlocks(t)
	for i := uint64(0); i < 5; i++ {
		pb.addPendingBlock(types.GetRandomBlock(i, 0))
	}
	blocks, _ := pb.getPendingBlocks()
	require.True(sort.SliceIsSorted(blocks, func(i, j int) bool {
		return blocks[i].Height() < blocks[j].Height()
	}))
}

func TestRemoveSubmittedBlocks(t *testing.T) {
	require := require.New(t)
	pb := newPendingBlocks(t)
	for i := uint64(0); i < 5; i++ {
		pb.addPendingBlock(types.GetRandomBlock(i, 0))
	}
	blocks, _ := pb.getPendingBlocks()
	pb.removeSubmittedBlocks(blocks)
	require.True(pb.isEmpty())
}

func TestRemoveSubsetOfBlocks(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()
	pb := newPendingBlocks(t)
	for i := uint64(1); i <= 5; i++ {
		block := types.GetRandomBlock(i, 0)
		pb.addPendingBlock(block)
		require.NoError(pb.store.SaveBlock(ctx, block, &types.Commit{}))
		pb.store.SetHeight(ctx, i)
	}
	// Remove blocks with height 1 and 2
	pb.removeSubmittedBlocks([]*types.Block{
		types.GetRandomBlock(1, 0),
		types.GetRandomBlock(2, 0),
	})
	remainingBlocks, err := pb.getPendingBlocks()
	require.NoError(err)
	require.Len(remainingBlocks, 3, "There should be 3 blocks remaining")
	for _, block := range remainingBlocks {
		require.Contains([]uint64{3, 4, 5}, block.Height(), "Only blocks with height 3, 4 and 5 should remain")
	}
}

func TestRemoveAllBlocksAndVerifyEmpty(t *testing.T) {
	require := require.New(t)
	pb := newPendingBlocks(t)
	for i := uint64(0); i < 5; i++ {
		pb.addPendingBlock(types.GetRandomBlock(i, 0))
	}
	// Remove all blocks
	blocks, err := pb.getPendingBlocks()
	require.NoError(err)
	pb.removeSubmittedBlocks(blocks)
	require.True(pb.isEmpty(), "PendingBlocks should be empty after removing all blocks")
}

func TestRemoveBlocksFromEmptyPendingBlocks(t *testing.T) {
	require := require.New(t)
	pb := newPendingBlocks(t)
	// Attempt to remove blocks from an empty PendingBlocks
	require.NotPanics(func() {
		pb.removeSubmittedBlocks([]*types.Block{
			types.GetRandomBlock(1, 0),
			types.GetRandomBlock(2, 0),
		})
	}, "Removing blocks from an empty PendingBlocks should not cause a panic")
}

func newPendingBlocks(t *testing.T) *PendingBlocks {
	kv, err := store.NewDefaultInMemoryKVStore()
	require.NoError(t, err)
	return NewPendingBlocks(store.New(kv))
}
