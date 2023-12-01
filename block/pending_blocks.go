package block

import (
	"sync"

	"github.com/rollkit/rollkit/types"
)

// PendingBlocks maintains blocks that need to be published to DA layer
type PendingBlocks struct {
	pendingBlocks []*types.Block
	mtx           *sync.RWMutex
}

// NewPendingBlocks returns a new PendingBlocks struct.
func NewPendingBlocks() *PendingBlocks {
	return &PendingBlocks{
		pendingBlocks: make([]*types.Block, 0),
		mtx:           new(sync.RWMutex),
	}
}

func (pb *PendingBlocks) getPendingBlocks() []*types.Block {
	pb.mtx.RLock()
	defer pb.mtx.RUnlock()
	return pb.pendingBlocks
}

func (pb *PendingBlocks) isEmpty() bool {
	pendingBlocks := pb.getPendingBlocks()
	return len(pendingBlocks) == 0
}

func (pb *PendingBlocks) addPendingBlock(block *types.Block) {
	pb.mtx.Lock()
	defer pb.mtx.Unlock()
	pb.pendingBlocks = append(pb.pendingBlocks, block)
}

func (pb *PendingBlocks) resetPendingBlocks() {
	pb.mtx.Lock()
	defer pb.mtx.Unlock()
	pb.pendingBlocks = make([]*types.Block, 0)
}
