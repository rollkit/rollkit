package block

import (
	"sync"

	"github.com/rollkit/rollkit/types"
)

// BlockCache maintains blocks that are seen and hard confirmed
type BlockCache struct {
	blocks     *sync.Map
	hashes     *sync.Map
	daIncluded *sync.Map
}

// NewBlockCache returns a new BlockCache struct
func NewBlockCache() *BlockCache {
	return &BlockCache{
		blocks:     new(sync.Map),
		hashes:     new(sync.Map),
		daIncluded: new(sync.Map),
	}
}

func (bc *BlockCache) getBlock(height uint64) (*types.Block, bool) {
	block, ok := bc.blocks.Load(height)
	if !ok {
		return nil, false
	}
	return block.(*types.Block), true
}

func (bc *BlockCache) setBlock(height uint64, block *types.Block) {
	if block != nil {
		bc.blocks.Store(height, block)
	}
}

func (bc *BlockCache) deleteBlock(height uint64) {
	bc.blocks.Delete(height)
}

func (bc *BlockCache) isSeen(hash string) bool {
	seen, ok := bc.hashes.Load(hash)
	if !ok {
		return false
	}
	return seen.(bool)
}

func (bc *BlockCache) setSeen(hash string) {
	bc.hashes.Store(hash, true)
}

func (bc *BlockCache) isDAIncluded(hash string) bool {
	daIncluded, ok := bc.daIncluded.Load(hash)
	if !ok {
		return false
	}
	return daIncluded.(bool)
}

func (bc *BlockCache) setDAIncluded(hash string) {
	bc.daIncluded.Store(hash, true)
}
