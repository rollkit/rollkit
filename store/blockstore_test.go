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

	bstore.SaveBlock(&types.Block{
		Header: types.Header{
			Height: 1,
		},
	})

	assert.Equal(uint64(1), bstore.Height())
}

func TestBlockstoreLoad(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	bstore := NewBlockStore()

	bstore.SaveBlock(&types.Block{
		Header: types.Header{
			Height: 1,
		},
	})

	assert.Equal(uint64(1), bstore.Height())

	block := bstore.LoadBlock(1)
	require.NotNil(block)

	assert.Equal(uint64(1), block.Header.Height)
}
