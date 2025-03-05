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

// PendingHeaders maintains headers that need to be published to DA layer
//
// Important assertions:
// - headers are safely stored in database before submission to DA
// - headers are always pushed to DA in order (by height)
// - DA submission of multiple headers is atomic - it's impossible to submit only part of a batch
//
// lastSubmittedHeight is updated only after receiving confirmation from DA.
// Worst case scenario is when headers was successfully submitted to DA, but confirmation was not received (e.g. node was
// restarted, networking issue occurred). In this case headers are re-submitted to DA (it's extra cost).
// rollkit is able to skip duplicate headers so this shouldn't affect full nodes.
// TODO(tzdybal): we shouldn't try to push all pending headers at once; this should depend on max blob size
type PendingHeaders struct {
	logger log.Logger
	store  store.Store

	// lastSubmittedHeight holds information about last header successfully submitted to DA
	lastSubmittedHeight atomic.Uint64
}

// NewPendingHeaders returns a new PendingHeaders struct
func NewPendingHeaders(store store.Store, logger log.Logger) (*PendingHeaders, error) {
	pb := &PendingHeaders{
		store:  store,
		logger: logger,
	}
	if err := pb.init(); err != nil {
		return nil, err
	}
	return pb, nil
}

// GetPendingHeaders returns a sorted slice of pending headers.
func (pb *PendingHeaders) GetPendingHeaders() ([]*types.SignedHeader, error) {
	return pb.getPendingHeaders(context.Background())
}

// GetLastSubmittedHeight returns the height of the last successfully submitted header.
func (pb *PendingHeaders) GetLastSubmittedHeight() uint64 {
	return pb.lastSubmittedHeight.Load()
}

// getPendingHeaders returns a sorted slice of pending headers
// that need to be published to DA layer in order of header height
func (pb *PendingHeaders) getPendingHeaders(ctx context.Context) ([]*types.SignedHeader, error) {
	lastSubmitted := pb.lastSubmittedHeight.Load()
	height := pb.store.Height()

	if lastSubmitted == height {
		return nil, nil
	}
	if lastSubmitted > height {
		panic(fmt.Sprintf("height of last header submitted to DA (%d) is greater than height of last header (%d)",
			lastSubmitted, height))
	}

	headers := make([]*types.SignedHeader, 0, height-lastSubmitted)
	for i := lastSubmitted + 1; i <= height; i++ {
		header, _, err := pb.store.GetBlockData(ctx, i)
		if err != nil {
			// return as much as possible + error information
			return headers, err
		}
		headers = append(headers, header)
	}
	return headers, nil
}

func (pb *PendingHeaders) isEmpty() bool {
	return pb.store.Height() == pb.lastSubmittedHeight.Load()
}

func (pb *PendingHeaders) numPendingHeaders() uint64 {
	return pb.store.Height() - pb.lastSubmittedHeight.Load()
}

func (pb *PendingHeaders) setLastSubmittedHeight(ctx context.Context, newLastSubmittedHeight uint64) {
	lsh := pb.lastSubmittedHeight.Load()

	if newLastSubmittedHeight > lsh && pb.lastSubmittedHeight.CompareAndSwap(lsh, newLastSubmittedHeight) {
		err := pb.store.SetMetadata(ctx, LastSubmittedHeightKey, []byte(strconv.FormatUint(newLastSubmittedHeight, 10)))
		if err != nil {
			// This indicates IO error in KV store. We can't do much about this.
			// After next successful DA submission, update will be re-attempted (with new value).
			// If store is not updated, after node restart some headers will be re-submitted to DA.
			pb.logger.Error("failed to store height of latest header submitted to DA", "err", err)
		}
	}
}

func (pb *PendingHeaders) init() error {
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
