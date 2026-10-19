package cache

import (
	"context"
	"errors"
	"fmt"

	"github.com/rs/zerolog"

	"github.com/evstack/ev-node/pkg/store"
	"github.com/evstack/ev-node/types"
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
// evolve is able to skip duplicate data so this shouldn't affect full nodes.
// Note: Submission of pending data to DA should account for the DA max blob size.
type PendingData struct {
	base *pendingBase[*types.Data]
}

var errInFlightData = errors.New("inflight data")

func fetchData(ctx context.Context, store store.Store, height uint64) (*types.Data, error) {
	_, data, err := store.GetBlockData(ctx, height)
	if err != nil {
		return nil, err
	}
	// in the executor, WIP data is temporarily stored. skip them until the process is completed
	if data.Height() == 0 {
		return nil, errInFlightData
	}
	return data, err
}

// NewPendingData returns a new PendingData struct
func NewPendingData(store store.Store, logger zerolog.Logger) (*PendingData, error) {
	base, err := newPendingBase(store, logger, LastSubmittedDataHeightKey, fetchData)
	if err != nil {
		return nil, err
	}
	return &PendingData{base: base}, nil
}

// GetPendingData returns a sorted slice of pending Data along with their marshalled bytes.
func (pd *PendingData) GetPendingData(ctx context.Context) ([]*types.Data, [][]byte, error) {
	dataList, err := pd.base.getPending(ctx)
	if err != nil {
		return nil, nil, err
	}

	if len(dataList) == 0 {
		return nil, nil, nil
	}

	marshalled := make([][]byte, len(dataList))
	for i, data := range dataList {
		dataBytes, err := data.MarshalBinary()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to marshal data at height %d: %w", data.Height(), err)
		}
		marshalled[i] = dataBytes
	}

	return dataList, marshalled, nil
}

func (pd *PendingData) NumPendingData() uint64 {
	pd.advancePastEmptyData(context.Background())
	return pd.base.numPending()
}

func (pd *PendingData) SetLastSubmittedDataHeight(ctx context.Context, newLastSubmittedDataHeight uint64) {
	pd.base.setLastSubmittedHeight(ctx, newLastSubmittedDataHeight)
}

// advancePastEmptyData advances lastSubmittedDataHeight past any consecutive empty data blocks.
// This ensures that NumPendingData doesn't count empty data that won't be published to DA.
func (pd *PendingData) advancePastEmptyData(ctx context.Context) {
	storeHeight, err := pd.base.store.Height(ctx)
	if err != nil {
		return
	}

	currentHeight := pd.base.getLastSubmittedHeight()

	for height := currentHeight + 1; height <= storeHeight; height++ {
		data, err := fetchData(ctx, pd.base.store, height)
		if err != nil {
			// Can't fetch data (might be in-flight or error), stop advancing
			return
		}

		if len(data.Txs) > 0 {
			// Found non-empty data, stop advancing
			return
		}

		// Empty data, advance past it
		pd.base.setLastSubmittedHeight(ctx, height)
	}
}

func (pd *PendingData) GetLastSubmittedDataHeight() uint64 {
	return pd.base.getLastSubmittedHeight()
}
