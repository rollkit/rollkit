package block

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync/atomic"

	ds "github.com/ipfs/go-datastore"

	"github.com/rollkit/rollkit/store"
	"github.com/rollkit/rollkit/third_party/log"
	"github.com/rollkit/rollkit/types"
)

// LastSubmittedHeightKey is the key used for persisting the last submitted height in store.
const LastSubmittedHeightKey = "last submitted"

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
// TODO(tzdybal): we shouldn't try to push all pending blocks at once; this should depend on max blob size
type PendingBlocks struct {
	store  store.Store
	logger log.Logger

	// lastSubmittedHeight holds information about last block successfully submitted to DA
	lastSubmittedHeight atomic.Uint64
}

// NewPendingBlocks returns a new PendingBlocks struct
func NewPendingBlocks(store store.Store, logger log.Logger) (*PendingBlocks, error) {
	pb := &PendingBlocks{
		store:  store,
		logger: logger,
	}
	if err := pb.init(); err != nil {
		return nil, err
	}
	return pb, nil
}

// getPendingBlocks returns a sorted slice of pending blocks
// that need to be published to DA layer in order of block height
func (pb *PendingBlocks) getPendingBlocks(ctx context.Context) ([]*types.Block, error) {
	lastSubmitted := pb.lastSubmittedHeight.Load()
	height := pb.store.Height()

	if lastSubmitted == height {
		return nil, nil
	}
	if lastSubmitted > height {
		panic(fmt.Sprintf("height of last block submitted to DA (%d) is greater than height of last block (%d)",
			lastSubmitted, height))
	}

	blocks := make([]*types.Block, 0, height-lastSubmitted)
	for i := lastSubmitted + 1; i <= height; i++ {
		block, err := pb.store.GetBlock(ctx, i)
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

func (pb *PendingBlocks) numPendingBlocks() uint64 {
	return pb.store.Height() - pb.lastSubmittedHeight.Load()
}

func (pb *PendingBlocks) setLastSubmittedHeight(ctx context.Context, newLastSubmittedHeight uint64) {
	lsh := pb.lastSubmittedHeight.Load()

	if newLastSubmittedHeight > lsh && pb.lastSubmittedHeight.CompareAndSwap(lsh, newLastSubmittedHeight) {
		err := pb.store.SetMetadata(ctx, LastSubmittedHeightKey, []byte(strconv.FormatUint(newLastSubmittedHeight, 10)))
		if err != nil {
			// This indicates IO error in KV store. We can't do much about this.
			// After next successful DA submission, update will be re-attempted (with new value).
			// If store is not updated, after node restart some blocks will be re-submitted to DA.
			pb.logger.Error("failed to store height of latest block submitted to DA", "err", err)
		}
	}
}

func (pb *PendingBlocks) init() error {
	raw, err := pb.store.GetMetadata(context.Background(), LastSubmittedHeightKey)
	if errors.Is(err, ds.ErrNotFound) {
		// LastSubmittedHeightKey was never used, it's special case not actual error
		// we don't need to modify lastSubmittedHeight
		return nil
	}
	if err != nil {
		return err
	}
	lsh, err := strconv.ParseUint(string(raw), 10, 64)
	if err != nil {
		return err
	}
	pb.lastSubmittedHeight.CompareAndSwap(0, lsh)
	return nil
}
