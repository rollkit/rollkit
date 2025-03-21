package block

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/types"
)

func TestBlockCache(t *testing.T) {
	chainID := "TestBlockCache"
	require := require.New(t)
	// Create new HeaderCache and DataCache and verify not nil
	hc := NewHeaderCache()
	require.NotNil(hc)

	dc := NewDataCache()
	require.NotNil(dc)

	// Test setBlock and getBlock
	height, nTxs := uint64(1), 2
	header, data := types.GetRandomBlock(height, nTxs, chainID)
	hc.SetItem(height, header)
	gotHeader := hc.GetItem(height)
	require.NotNil(gotHeader, "getHeader should return non-nil after setHeader")
	require.Equal(header, gotHeader)
	dc.SetItem(height, data)
	gotData := dc.GetItem(height)
	require.NotNil(gotData, "getData should return non-nil after setData")
	require.Equal(data, gotData)

	// Test overwriting a block
	header1, data1 := types.GetRandomBlock(height, nTxs, chainID)
	hc.SetItem(height, header1)
	gotHeader1 := hc.GetItem(height)
	require.NotNil(gotHeader1, "getHeader should return non-nil after overwriting a header")
	require.Equal(header1, gotHeader1)
	dc.SetItem(height, data1)
	gotData1 := dc.GetItem(height)
	require.NotNil(gotData1, "getData should return non-nil after overwriting a data")
	require.Equal(data1, gotData1)

	// Test deleteBlock
	hc.DeleteItem(height)
	h := hc.GetItem(height)
	require.Nil(h, "getHeader should return nil after deleteHeader")
	dc.DeleteItem(height)
	d := dc.GetItem(height)
	require.Nil(d, "getData should return nil after deleteData")

	// Test isSeen and setSeen
	require.False(hc.IsSeen("hash"), "isSeen should return false for unseen hash")
	hc.SetSeen("hash")
	require.True(hc.IsSeen("hash"), "isSeen should return true for seen hash")
	require.False(dc.IsSeen("hash"), "isSeen should return false for unseen hash")
	dc.SetSeen("hash")
	require.True(dc.IsSeen("hash"), "isSeen should return true for seen hash")

	// Test setDAIncluded
	require.False(hc.IsDAIncluded("hash"), "DAIncluded should be false for unseen hash")
	hc.SetDAIncluded("hash")
	require.True(hc.IsDAIncluded("hash"), "DAIncluded should be true for seen hash")
	require.False(dc.IsDAIncluded("hash"), "DAIncluded should be false for unseen hash")
	dc.SetDAIncluded("hash")
	require.True(dc.IsDAIncluded("hash"), "DAIncluded should be true for seen hash")
}
