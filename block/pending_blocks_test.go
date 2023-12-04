package block

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/types"
)

func TestGetPendingBlocks(t *testing.T) {
	require := require.New(t)
	pb := NewPendingBlocks()
	for i := uint64(0); i < 5; i++ {
		pb.addPendingBlock(types.GetRandomBlock(i, 0))
	}
	blocks := pb.getPendingBlocks()
	require.True(sort.SliceIsSorted(blocks, func(i, j int) bool {
		return blocks[i].Height() < blocks[j].Height()
	}))
}

func TestRemoveSubmittedBlocks(t *testing.T) {
	require := require.New(t)
	pb := NewPendingBlocks()
	for i := uint64(0); i < 5; i++ {
		pb.addPendingBlock(types.GetRandomBlock(i, 0))
	}
	blocks := pb.getPendingBlocks()
	pb.removeSubmittedBlocks(blocks)
	require.True(pb.isEmpty())
}

func TestRemoveSubsetOfBlocks(t *testing.T) {
	require := require.New(t)
	pb := NewPendingBlocks()
	for i := uint64(0); i < 5; i++ {
		pb.addPendingBlock(types.GetRandomBlock(i, 0))
	}
	// Remove blocks with height 1 and 2
	pb.removeSubmittedBlocks([]*types.Block{
		types.GetRandomBlock(1, 0),
		types.GetRandomBlock(2, 0),
	})
	remainingBlocks := pb.getPendingBlocks()
	require.Len(remainingBlocks, 3, "There should be 3 blocks remaining")
	for _, block := range remainingBlocks {
		require.Contains([]uint64{0, 3, 4}, block.Height(), "Only blocks with height 0, 3, and 4 should remain")
	}
}

func TestRemoveAllBlocksAndVerifyEmpty(t *testing.T) {
	require := require.New(t)
	pb := NewPendingBlocks()
	for i := uint64(0); i < 5; i++ {
		pb.addPendingBlock(types.GetRandomBlock(i, 0))
	}
	// Remove all blocks
	pb.removeSubmittedBlocks(pb.getPendingBlocks())
	require.True(pb.isEmpty(), "PendingBlocks should be empty after removing all blocks")
}

func TestRemoveBlocksFromEmptyPendingBlocks(t *testing.T) {
	require := require.New(t)
	pb := NewPendingBlocks()
	// Attempt to remove blocks from an empty PendingBlocks
	require.NotPanics(func() {
		pb.removeSubmittedBlocks([]*types.Block{
			types.GetRandomBlock(1, 0),
			types.GetRandomBlock(2, 0),
		})
	}, "Removing blocks from an empty PendingBlocks should not cause a panic")
}
