package block

import (
	"context"
	"sync/atomic"

	"github.com/rollkit/rollkit/store"

	"github.com/rollkit/rollkit/types"
)

// PendingBlocks maintains blocks that need to be published to DA layer
//
// Important assertions:
// - blocks are safely stored in database before submission to DA
// - blocks are always pushed to DA in order (by height)
// - DA submission of multiple blocks is atomic - it's impossible to submit only part of a batch
//
// lastSubmittedHeight is updated only after receiving confirmation from DA.
// Worst case scenario is when blocks was successfully submitted to DA, but confirmation was not received (e.g. node was
// restarted, networking issue occurred). In this case blocks are re-submitted to DA (it's extra cost).
// rollkit is able to skip duplicate blocks so this shouldn't affect full nodes.
// TODO(tzdybal): batch size
type PendingBlocks struct {
	store store.Store

	// lastSubmittedHeight holds information about last block successfully submitted to DA
	lastSubmittedHeight atomic.Uint64
}

// NewPendingBlocks returns a new PendingBlocks struct
func NewPendingBlocks(store store.Store) *PendingBlocks {
	return &PendingBlocks{
		store: store,
		// TODO(tzdybal): lastSubmittedHeight from store
	}
}

// getPendingBlocks returns a sorted slice of pending blocks
// that need to be published to DA layer in order of block height
func (pb *PendingBlocks) getPendingBlocks() ([]*types.Block, error) {
	height := pb.store.Height()
	lastSubmitted := pb.lastSubmittedHeight.Load()

	// TODO(tzdybal) - lastSubmitted should never be > than height in final implementation
	if lastSubmitted >= height {
		return nil, nil
	}

	blocks := make([]*types.Block, 0, height-lastSubmitted)
	for i := lastSubmitted + 1; i <= height; i++ {
		block, err := pb.store.GetBlock(context.TODO(), i)
		if err != nil {
			// return as much as possible + error information
			return blocks, err
		}
		blocks = append(blocks, block)
	}
	return blocks, nil
}

func (pb *PendingBlocks) isEmpty() bool {
	return pb.store.Height() == pb.lastSubmittedHeight.Load()
}

func (pb *PendingBlocks) addPendingBlock(_ *types.Block) {
	// TODO(tzdybal): remove this method
}

// TODO(tzdybal): change signature (accept height)
func (pb *PendingBlocks) removeSubmittedBlocks(blocks []*types.Block) {
	if len(blocks) == 0 {
		return
	}
	height := blocks[len(blocks)-1].Height()
	lastSubmitted := pb.lastSubmittedHeight.Load()

	if height > lastSubmitted {
		pb.lastSubmittedHeight.CompareAndSwap(lastSubmitted, height)
	}
}
