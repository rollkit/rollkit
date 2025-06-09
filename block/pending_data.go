package block

import (
	"context"

	"cosmossdk.io/log"

	"github.com/rollkit/rollkit/pkg/store"
	"github.com/rollkit/rollkit/types"
)

// LastSubmittedDataHeightKey is the key used for persisting the last submitted data height in store.
const LastSubmittedDataHeightKey = "last-submitted-data-height"

// PendingData maintains Data that need to be published to DA layer
//
// Important assertions:
// - data is safely stored in database before submission to DA
// - data is always pushed to DA in order (by height)
// - DA submission of multiple data is atomic - it's impossible to submit only part of a batch
//
// lastSubmittedDataHeight is updated only after receiving confirmation from DA.
// Worst case scenario is when data was successfully submitted to DA, but confirmation was not received (e.g. node was
// restarted, networking issue occurred). In this case data is re-submitted to DA (it's extra cost).
// rollkit is able to skip duplicate data so this shouldn't affect full nodes.
// TODO: we shouldn't try to push all pending data at once; this should depend on max blob size
type PendingData struct {
	base *pendingBase[*types.Data]
}

func fetchData(ctx context.Context, store store.Store, height uint64) (*types.Data, error) {
	_, data, err := store.GetBlockData(ctx, height)
	return data, err
}

// NewPendingData returns a new PendingData struct
func NewPendingData(store store.Store, logger log.Logger) (*PendingData, error) {
	base, err := newPendingBase(store, logger, LastSubmittedDataHeightKey, fetchData)
	if err != nil {
		return nil, err
	}
	return &PendingData{base: base}, nil
}

// GetPendingData returns a sorted slice of pending Data.
func (pd *PendingData) getPendingData(ctx context.Context) ([]*types.Data, error) {
	return pd.base.getPending(ctx)
}

// GetLastSubmittedDataHeight returns the height of the last successfully submitted data.
func (pd *PendingData) getLastSubmittedDataHeight() uint64 {
	return pd.base.lastHeight.Load()
}

func (pd *PendingData) isEmpty() bool {
	return pd.base.isEmpty()
}

func (pd *PendingData) numPendingData() uint64 {
	return pd.base.numPending()
}

func (pd *PendingData) setLastSubmittedDataHeight(ctx context.Context, newLastSubmittedDataHeight uint64) {
	pd.base.setLastSubmittedHeight(ctx, newLastSubmittedDataHeight)
}

func (pd *PendingData) init() error {
	return pd.base.init()
}
