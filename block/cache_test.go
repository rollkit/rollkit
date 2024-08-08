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
	require.True(ok, "getHeader should return true after setHeader")
	require.Equal(header, gotHeader)
	dc.setData(height, data)
	gotData, ok := dc.getData(height)
	require.True(ok, "getData should return true after setData")
	require.Equal(data, gotData)

	// Test overwriting a block
	header1, data1 := types.GetRandomBlock(height, nTxs)
	hc.setHeader(height, header1)
	gotHeader1, ok1 := hc.getHeader(height)
	require.True(ok1, "getHeader should return true after overwriting a header")
	require.Equal(header1, gotHeader1)
	dc.setData(height, data1)
	gotData1, ok1 := hc.getHeader(height)
	require.True(ok1, "getData should return true after overwriting a data")
	require.Equal(data1, gotData1)

	// Test deleteBlock
	hc.deleteHeader(height)
	_, ok = hc.getHeader(height)
	require.False(ok, "getHeader should return false after deleteHeader")
	dc.deleteData(height)
	_, ok = dc.getData(height)
	require.False(ok, "getData should return false after deleteData")

	// Test isSeen and setSeen
	require.False(hc.isSeen("hash"), "isSeen should return false for unseen hash")
	hc.setSeen("hash")
	require.True(hc.isSeen("hash"), "isSeen should return true for seen hash")
	require.False(dc.isSeen("hash"), "isSeen should return false for unseen hash")
	dc.setSeen("hash")
	require.True(dc.isSeen("hash"), "isSeen should return true for seen hash")

	// Test setDAIncluded
	require.False(hc.isDAIncluded("hash"), "DAIncluded should be false for unseen hash")
	hc.setDAIncluded("hash")
	require.True(hc.isDAIncluded("hash"), "DAIncluded should be true for seen hash")
	require.False(dc.isDAIncluded("hash"), "DAIncluded should be false for unseen hash")
	dc.setDAIncluded("hash")
	require.True(dc.isDAIncluded("hash"), "DAIncluded should be true for seen hash")
}
