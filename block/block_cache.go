package block

import (
	"sync"

	"github.com/rollkit/rollkit/types"
)

type BlockCache struct {
	blocks      map[uint64]*types.Block
	blockStatus map[string]BlockStatus
	mtx         *sync.RWMutex
}

type BlockStatus int

const (
	StatusSeen BlockStatus = iota
	StatusHardConfirmed
)

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
	_, ok := bc.blockStatus[hash]
	return ok
}

func (bc *BlockCache) setSeen(hash string) {
	bc.mtx.Lock()
	defer bc.mtx.Unlock()
	bc.blockStatus[hash] = StatusSeen
}

func (bc *BlockCache) setHardConfirmed(hash string) {
	bc.mtx.Lock()
	defer bc.mtx.Unlock()
	bc.blockStatus[hash] = StatusHardConfirmed
}
