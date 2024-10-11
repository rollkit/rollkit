package block

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/types"
)

func TestBlockCache(t *testing.T) {
	chainId := "TestBlockCache"
	require := require.New(t)
	// Create new HeaderCache and DataCache and verify not nil
	hc := NewHeaderCache()
	require.NotNil(hc)

	dc := NewDataCache()
	require.NotNil(dc)

	// Test setBlock and getBlock
	height, nTxs := uint64(1), 2
	header, data := types.GetRandomBlock(height, nTxs, chainId)
	hc.setHeader(height, header)
	gotHeader := hc.getHeader(height)
	require.NotNil(gotHeader, "getHeader should return non-nil after setHeader")
	require.Equal(header, gotHeader)
	dc.setData(height, data)
	gotData := dc.getData(height)
	require.NotNil(gotData, "getData should return non-nil after setData")
	require.Equal(data, gotData)

	// Test overwriting a block
	header1, data1 := types.GetRandomBlock(height, nTxs, chainId)
	hc.setHeader(height, header1)
	gotHeader1 := hc.getHeader(height)
	require.NotNil(gotHeader1, "getHeader should return non-nil after overwriting a header")
	require.Equal(header1, gotHeader1)
	dc.setData(height, data1)
	gotData1 := dc.getData(height)
	require.NotNil(gotData1, "getData should return non-nil after overwriting a data")
	require.Equal(data1, gotData1)

	// Test deleteBlock
	hc.deleteHeader(height)
	h := hc.getHeader(height)
	require.Nil(h, "getHeader should return nil after deleteHeader")
	dc.deleteData(height)
	d := dc.getData(height)
	require.Nil(d, "getData should return nil after deleteData")

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
