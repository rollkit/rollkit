package block

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/types"
)

func TestBlockCache(t *testing.T) {
	require := require.New(t)
	// Create new HeaderCache and DataCache and verify not nil
	hc := NewHeaderCache()
	require.NotNil(hc)

	dc := NewDataCache()
	require.NotNil(dc)

	// Test setBlock and getBlock
	height, nTxs := uint64(1), 2
	header, data := types.GetRandomBlock(height, nTxs)
	hc.setHeader(height, header)
	gotHeader, ok := hc.getHeader(height)
	require.True(ok, "getBlock should return true after setBlock")
	require.Equal(header, gotHeader)

	// Test overwriting a block
	header1, data1 := types.GetRandomBlock(height, nTxs)
	hc.setHeader(height, header1)
	gotHeader1, ok1 := hc.getHeader(height)
	require.True(ok1, "getBlock should return true after overwriting a block")
	require.Equal(header1, gotHeader1)

	// Test deleteBlock
	hc.deleteHeader(height)
	_, ok = hc.getHeader(height)
	require.False(ok, "getBlock should return false after deleteBlock")

	// Test isSeen and setSeen
	require.False(hc.isSeen("hash"), "isSeen should return false for unseen hash")
	hc.setSeen("hash")
	require.True(hc.isSeen("hash"), "isSeen should return true for seen hash")

	// Test setDAIncluded
	require.False(hc.isDAIncluded("hash"), "DAIncluded should be false for unseen hash")
	hc.setDAIncluded("hash")
	require.True(hc.isDAIncluded("hash"), "DAIncluded should be true for seen hash")
}
