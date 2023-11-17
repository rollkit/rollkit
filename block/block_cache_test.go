package block

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/types"
)

func TestBlockCache(t *testing.T) {
	// Create new BlockCache and verify not nil
	bc := NewBlockCache()
	require.NotNil(t, bc)

	// Test setBlock and getBlock
	height, nTxs := uint64(1), 2
	block := types.GetRandomBlock(height, nTxs)
	bc.setBlock(height, block)
	gotBlock, ok := bc.getBlock(height)
	require.True(t, ok, "getBlock should return true after setBlock")
	require.Equal(t, block, gotBlock)

	// Test overwriting a block
	block1 := types.GetRandomBlock(height, nTxs)
	bc.setBlock(height, block1)
	gotBlock1, ok1 := bc.getBlock(height)
	require.True(t, ok1, "getBlock should return true after overwriting a block")
	require.Equal(t, block1, gotBlock1)

	// Test deleteBlock
	bc.deleteBlock(height)
	_, ok = bc.getBlock(height)
	require.False(t, ok, "getBlock should return false after deleteBlock")

	// Test isSeen and setSeen
	require.False(t, bc.isSeen("hash"), "isSeen should return false for unseen hash")
	bc.setSeen("hash")
	require.True(t, bc.isSeen("hash"), "isSeen should return true for seen hash")

	// Test setDAIncluded
	require.False(t, bc.isDAIncluded("hash"), "DAIncluded should be false for unseen hash")
	bc.setDAIncluded("hash")
	require.True(t, bc.isDAIncluded("hash"), "DAIncluded should be true for seen hash")
}
