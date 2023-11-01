package block

import (
	"testing"

	"github.com/rollkit/rollkit/types"
	"github.com/stretchr/testify/require"
)

// TODO: refactor this helper as it is defined in multiple places
func getRandomBlock(height uint64, nTxs int) *types.Block {
	block := &types.Block{
		SignedHeader: types.SignedHeader{
			Header: types.Header{
				BaseHeader: types.BaseHeader{
					Height: height,
				},
				AggregatorsHash: make([]byte, 32),
			}},
		Data: types.Data{
			Txs: make(types.Txs, nTxs),
			IntermediateStateRoots: types.IntermediateStateRoots{
				RawRootsList: make([][]byte, nTxs),
			},
		},
	}

	for i := 0; i < nTxs; i++ {
		block.Data.Txs[i] = types.GetRandomTx()
		block.Data.IntermediateStateRoots.RawRootsList[i] = types.GetRandomBytes(32)
	}

	return block
}

func TestBlockCache(t *testing.T) {
	require := require.New(t)
	// Create new BlockCache and verify not nil
	bc := NewBlockCache()
	require.NotNil(bc)

	// Test setBlock and getBlock
	height := uint64(1)
	block := getRandomBlock(height, 2)
	bc.setBlock(height, block)
	gotBlock, ok := bc.getBlock(height)
	require.True(ok, "getBlock should return true after setBlock")
	require.Equal(block, gotBlock)

	// Test overwriting a block
	block1 := getRandomBlock(height, 2)
	bc.setBlock(height, block1)
	gotBlock1, ok1 := bc.getBlock(height)
	require.True(ok1, "getBlock hsould return true after overwriting a block")
	require.Equal(block1, gotBlock1)

	// Test deleteBlock
	bc.deleteBlock(height)
	_, ok = bc.getBlock(height)
	require.False(ok, "getBlock should return false after deleteBlock")

	// Test isSeen and setSeen
	require.False(bc.isSeen("hash"), "isSeen should return false for unseen hash")
	bc.setSeen("hash")
	require.True(bc.isSeen("hash"), "isSeen should return true for seen hash")

	// Test setHardConfirmed
	require.False(bc.hardConfirmations["hash"], "hardConfirmations should be false for unseen hash")
	bc.setHardConfirmed("hash")
	require.True(bc.hardConfirmations["hash"], "hardConfirmations should be true for seen hash")
}
