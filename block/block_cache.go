package block

import (
	"sync"

	"github.com/rollkit/rollkit/types"
)

type BlockCache struct {
	blocks     map[uint64]*types.Block
	hashes     map[string]bool
	daIncluded map[string]bool
	mtx        *sync.RWMutex
}

func NewBlockCache() *BlockCache {
	return &BlockCache{
		blocks:     make(map[uint64]*types.Block),
		hashes:     make(map[string]bool),
		daIncluded: make(map[string]bool),
		mtx:        new(sync.RWMutex),
	}
}

func (bc *BlockCache) getBlock(height uint64) (*types.Block, bool) {
	bc.mtx.Lock()
	defer bc.mtx.Unlock()
	block, ok := bc.blocks[height]
	return block, ok
}

func (bc *BlockCache) setBlock(height uint64, block *types.Block) {
	bc.mtx.Lock()
	defer bc.mtx.Unlock()
	bc.blocks[height] = block
}

func (bc *BlockCache) deleteBlock(height uint64) {
	bc.mtx.Lock()
	defer bc.mtx.Unlock()
	delete(bc.blocks, height)
}

func (bc *BlockCache) isSeen(hash string) bool {
	bc.mtx.Lock()
	defer bc.mtx.Unlock()
	return bc.hashes[hash]
}

func (bc *BlockCache) setSeen(hash string) {
	bc.mtx.Lock()
	defer bc.mtx.Unlock()
	bc.hashes[hash] = true
}

func (bc *BlockCache) isDAIncluded(hash string) bool {
	bc.mtx.RLock()
	defer bc.mtx.RUnlock()
	return bc.daIncluded[hash]
}

func (bc *BlockCache) setDAIncluded(hash string) {
	bc.mtx.Lock()
	defer bc.mtx.Unlock()
	bc.daIncluded[hash] = true
}
