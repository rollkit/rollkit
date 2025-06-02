package block

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync/atomic"

	"cosmossdk.io/log"
	ds "github.com/ipfs/go-datastore"

	"github.com/rollkit/rollkit/pkg/store"
	"github.com/rollkit/rollkit/types"
)

// LastSubmittedHeaderHeightKey is the key used for persisting the last submitted height in store.
const LastSubmittedHeaderHeightKey = "last-submitted-header-height"

// PendingHeaders maintains headers that need to be published to DA layer
//
// Important assertions:
// - headers are safely stored in database before submission to DA
// - headers are always pushed to DA in order (by height)
// - DA submission of multiple headers is atomic - it's impossible to submit only part of a batch
//
// lastSubmittedHeaderHeight is updated only after receiving confirmation from DA.
// Worst case scenario is when headers was successfully submitted to DA, but confirmation was not received (e.g. node was
// restarted, networking issue occurred). In this case headers are re-submitted to DA (it's extra cost).
// rollkit is able to skip duplicate headers so this shouldn't affect full nodes.
// TODO(tzdybal): we shouldn't try to push all pending headers at once; this should depend on max blob size
type PendingHeaders struct {
	logger log.Logger
	store  store.Store

	// lastSubmittedHeaderHeight holds information about last header successfully submitted to DA
	lastSubmittedHeaderHeight atomic.Uint64
}

// NewPendingHeaders returns a new PendingHeaders struct
func NewPendingHeaders(store store.Store, logger log.Logger) (*PendingHeaders, error) {
	ph := &PendingHeaders{
		store:  store,
		logger: logger,
	}
	if err := ph.init(); err != nil {
		return nil, err
	}
	return ph, nil
}

// GetPendingHeaders returns a sorted slice of pending headers.
func (ph *PendingHeaders) GetPendingHeaders() ([]*types.SignedHeader, error) {
	return ph.getPendingHeaders(context.Background())
}

// GetLastSubmittedHeaderHeight returns the height of the last successfully submitted header.
func (ph *PendingHeaders) GetLastSubmittedHeaderHeight() uint64 {
	return ph.lastSubmittedHeaderHeight.Load()
}

// getPendingHeaders returns a sorted slice of pending headers
// that need to be published to DA layer in order of header height
func (ph *PendingHeaders) getPendingHeaders(ctx context.Context) ([]*types.SignedHeader, error) {
	lastSubmitted := ph.lastSubmittedHeaderHeight.Load()
	height, err := ph.store.Height(ctx)
	if err != nil {
		return nil, err
	}

	if lastSubmitted == height {
		return nil, nil
	}
	if lastSubmitted > height {
		panic(fmt.Sprintf("height of last header submitted to DA (%d) is greater than height of last header (%d)",
			lastSubmitted, height))
	}

	headers := make([]*types.SignedHeader, 0, height-lastSubmitted)
	for i := lastSubmitted + 1; i <= height; i++ {
		header, _, err := ph.store.GetBlockData(ctx, i)
		if err != nil {
			// return as much as possible + error information
			return headers, err
		}
		headers = append(headers, header)
	}
	return headers, nil
}

func (ph *PendingHeaders) isEmpty() bool {
	height, err := ph.store.Height(context.Background())
	if err != nil {
		return false
	}
	return height == ph.lastSubmittedHeaderHeight.Load()
}

func (ph *PendingHeaders) numPendingHeaders() uint64 {
	height, err := ph.store.Height(context.Background())
	if err != nil {
		return 0
	}
	return height - ph.lastSubmittedHeaderHeight.Load()
}

func (ph *PendingHeaders) setLastSubmittedHeaderHeight(ctx context.Context, newLastSubmittedHeaderHeight uint64) {
	lsh := ph.lastSubmittedHeaderHeight.Load()

	if newLastSubmittedHeaderHeight > lsh && ph.lastSubmittedHeaderHeight.CompareAndSwap(lsh, newLastSubmittedHeaderHeight) {
		bz := make([]byte, 8)
		binary.LittleEndian.PutUint64(bz, newLastSubmittedHeaderHeight)
		err := ph.store.SetMetadata(ctx, LastSubmittedHeaderHeightKey, bz)
		if err != nil {
			// This indicates IO error in KV store. We can't do much about this.
			// After next successful DA submission, update will be re-attempted (with new value).
			// If store is not updated, after node restart some headers will be re-submitted to DA.
			ph.logger.Error("failed to store height of latest header submitted to DA", "err", err)
		}
	}
}

func (ph *PendingHeaders) init() error {
	raw, err := ph.store.GetMetadata(context.Background(), LastSubmittedHeaderHeightKey)
	if errors.Is(err, ds.ErrNotFound) {
		// LastSubmittedHeaderHeightKey was never used, it's special case not actual error
		// we don't need to modify lastSubmittedHeaderHeight
		return nil
	}
	if err != nil {
		return err
	}
	if len(raw) != 8 {
		return fmt.Errorf("invalid length of last submitted height: %d, expected 8", len(raw))
	}
	lsh := binary.LittleEndian.Uint64(raw)
	if lsh == 0 {
		// this is special case, we don't need to modify lastSubmittedHeaderHeight
		return nil
	}
	ph.lastSubmittedHeaderHeight.CompareAndSwap(0, lsh)
	return nil
}
