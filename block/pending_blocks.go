package block

import (
	"sort"
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

// getPendingBlocks returns a sorted slice of pending blocks
// that need to be published to DA layer in order of block height
func (pb *PendingBlocks) getPendingBlocks() []*types.Block {
	blocks := copyBlocks(pb)
	sort.Slice(blocks, func(i, j int) bool {
		return blocks[i].Height() < blocks[j].Height()
	})
	return blocks
}

// copyBlocks creates a copy of the pending blocks in a thread-safe manner.
// It returns a slice of pointers to the copied blocks.
func copyBlocks(pb *PendingBlocks) []*types.Block {
	pb.mtx.RLock()
	defer pb.mtx.RUnlock()
	blocks := make([]*types.Block, 0, len(pb.pendingBlocks))
	for _, block := range pb.pendingBlocks {
		blocks = append(blocks, block)
	}
	return blocks
}

func (pb *PendingBlocks) isEmpty() bool {
	pb.mtx.RLock()
	defer pb.mtx.RUnlock()
	return len(pb.pendingBlocks) == 0
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
