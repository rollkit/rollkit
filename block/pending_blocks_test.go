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
	}), true)
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
