package block

import (
	"context"

	"cosmossdk.io/log"

	"github.com/rollkit/rollkit/pkg/store"
	"github.com/rollkit/rollkit/types"
)

// LastSubmittedHeaderHeightKey is the key used for persisting the last submitted header height in store.
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
	base *pendingBase[*types.SignedHeader]
}

func fetchSignedHeader(ctx context.Context, store store.Store, height uint64) (*types.SignedHeader, error) {
	header, _, err := store.GetBlockData(ctx, height)
	return header, err
}

// NewPendingHeaders returns a new PendingHeaders struct
func NewPendingHeaders(store store.Store, logger log.Logger) (*PendingHeaders, error) {
	base, err := newPendingBase(store, logger, LastSubmittedHeaderHeightKey, fetchSignedHeader)
	if err != nil {
		return nil, err
	}
	return &PendingHeaders{base: base}, nil
}

// GetPendingHeaders returns a sorted slice of pending headers.
func (ph *PendingHeaders) getPendingHeaders(ctx context.Context) ([]*types.SignedHeader, error) {
	return ph.base.getPending(ctx)
}

// GetLastSubmittedHeaderHeight returns the height of the last successfully submitted header.
func (ph *PendingHeaders) getLastSubmittedHeaderHeight() uint64 {
	return ph.base.lastHeight.Load()
}

func (ph *PendingHeaders) isEmpty() bool {
	return ph.base.isEmpty()
}

func (ph *PendingHeaders) numPendingHeaders() uint64 {
	return ph.base.numPending()
}

func (ph *PendingHeaders) setLastSubmittedHeaderHeight(ctx context.Context, newLastSubmittedHeaderHeight uint64) {
	ph.base.setLastSubmittedHeight(ctx, newLastSubmittedHeaderHeight)
}

func (ph *PendingHeaders) init() error {
	return ph.base.init()
}
