package store

import (
	"testing"

	"github.com/lazyledger/optimint/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBlockstoreHeight(t *testing.T) {
	assert := assert.New(t)

	bstore := NewBlockStore()
	assert.Equal(uint64(0), bstore.Height())

	err := bstore.SaveBlock(&types.Block{
		Header: types.Header{
			Height: 1,
		},
	})
	assert.NoError(err)
	assert.Equal(uint64(1), bstore.Height())
}

func TestBlockstoreLoad(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	bstore := NewBlockStore()

	err := bstore.SaveBlock(&types.Block{
		Header: types.Header{
			Height: 1,
		},
	})
	require.NoError(err)
	assert.Equal(uint64(1), bstore.Height())

	block, err := bstore.LoadBlock(1)
	require.NoError(err)
	require.NotNil(block)

	assert.Equal(uint64(1), block.Header.Height)
}
