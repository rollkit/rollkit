package block

import (
	"sync"

	"github.com/rollkit/rollkit/types"
)

// PendingBlocks maintains blocks that need to be published to DA layer
type PendingBlocks struct {
	pendingBlocks map[uint64]*types.Block
	mtx           *sync.RWMutex
}

// NewPendingBlocks returns a new PendingBlocks struct
func NewPendingBlocks() *PendingBlocks {
	return &PendingBlocks{
		pendingBlocks: make(map[uint64]*types.Block),
		mtx:           new(sync.RWMutex),
	}
}

func (pb *PendingBlocks) getPendingBlocks() []*types.Block {
	pb.mtx.RLock()
	defer pb.mtx.RUnlock()
	blocks := make([]*types.Block, 0)
	for _, block := range pb.pendingBlocks {
		blocks = append(blocks, block)
	}
	return blocks
}

func (pb *PendingBlocks) isEmpty() bool {
	pendingBlocks := pb.getPendingBlocks()
	return len(pendingBlocks) == 0
}

func (pb *PendingBlocks) addPendingBlock(block *types.Block) {
	pb.mtx.Lock()
	defer pb.mtx.Unlock()
	pb.pendingBlocks[block.Height()] = block
}

func (pb *PendingBlocks) removeSubmittedBlocks(blocks []*types.Block) {
	pb.mtx.Lock()
	defer pb.mtx.Unlock()
	for _, block := range blocks {
		delete(pb.pendingBlocks, block.Height())
	}
}
