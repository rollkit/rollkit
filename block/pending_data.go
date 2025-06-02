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

// LastSubmittedDataHeightKey is the key used for persisting the last submitted data height in store.
const LastSubmittedDataHeightKey = "last-submitted-data-height"

// PendingData maintains SignedData that need to be published to DA layer
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
	logger log.Logger
	store  store.Store

	// lastSubmittedDataHeight holds information about last data successfully submitted to DA
	lastSubmittedDataHeight atomic.Uint64
}

// NewPendingData returns a new PendingData struct
func NewPendingData(store store.Store, logger log.Logger) (*PendingData, error) {
	pd := &PendingData{
		store:  store,
		logger: logger,
	}
	if err := pd.init(); err != nil {
		return nil, err
	}
	return pd, nil
}

// GetPendingData returns a sorted slice of pending SignedData.
func (pd *PendingData) GetPendingData() ([]*types.SignedData, error) {
	return pd.getPendingData(context.Background())
}

// GetLastSubmittedDataHeight returns the height of the last successfully submitted data.
func (pd *PendingData) GetLastSubmittedDataHeight() uint64 {
	return pd.lastSubmittedDataHeight.Load()
}

// getPendingData returns a sorted slice of pending SignedData
// that need to be published to DA layer in order of data height
func (pd *PendingData) getPendingData(ctx context.Context) ([]*types.SignedData, error) {
	lastSubmitted := pd.lastSubmittedDataHeight.Load()
	height, err := pd.store.Height(ctx)
	if err != nil {
		return nil, err
	}

	if lastSubmitted == height {
		return nil, nil
	}
	if lastSubmitted > height {
		panic(fmt.Sprintf("height of last data submitted to DA (%d) is greater than height of last data (%d)",
			lastSubmitted, height))
	}

	pending := make([]*types.SignedData, 0, height-lastSubmitted)
	for i := lastSubmitted + 1; i <= height; i++ {
		_, data, err := pd.store.GetBlockData(ctx, i)
		if err != nil {
			// return as much as possible + error information
			return pending, err
		}
		if data == nil {
			continue
		}
		// If you store SignedData directly, replace this with direct retrieval
		signedData := &types.SignedData{
			Data: *data,
			// Signature and Signer are not stored in Data, so this may need to be extended if you store them elsewhere
		}
		pending = append(pending, signedData)
	}
	return pending, nil
}

func (pd *PendingData) isEmpty() bool {
	height, err := pd.store.Height(context.Background())
	if err != nil {
		return false
	}
	return height == pd.lastSubmittedDataHeight.Load()
}

func (pd *PendingData) numPendingData() uint64 {
	height, err := pd.store.Height(context.Background())
	if err != nil {
		return 0
	}
	return height - pd.lastSubmittedDataHeight.Load()
}

func (pd *PendingData) setLastSubmittedDataHeight(ctx context.Context, newLastSubmittedDataHeight uint64) {
	lsd := pd.lastSubmittedDataHeight.Load()

	if newLastSubmittedDataHeight > lsd && pd.lastSubmittedDataHeight.CompareAndSwap(lsd, newLastSubmittedDataHeight) {
		bz := make([]byte, 8)
		binary.LittleEndian.PutUint64(bz, newLastSubmittedDataHeight)
		err := pd.store.SetMetadata(ctx, LastSubmittedDataHeightKey, bz)
		if err != nil {
			// This indicates IO error in KV store. We can't do much about this.
			// After next successful DA submission, update will be re-attempted (with new value).
			// If store is not updated, after node restart some data will be re-submitted to DA.
			pd.logger.Error("failed to store height of latest data submitted to DA", "err", err)
		}
	}
}

func (pd *PendingData) init() error {
	raw, err := pd.store.GetMetadata(context.Background(), LastSubmittedDataHeightKey)
	if errors.Is(err, ds.ErrNotFound) {
		// LastSubmittedDataHeightKey was never used, it's special case not actual error
		// we don't need to modify lastSubmittedDataHeight
		return nil
	}
	if err != nil {
		return err
	}
	if len(raw) != 8 {
		return fmt.Errorf("invalid length of last submitted data height: %d, expected 8", len(raw))
	}
	lsd := binary.LittleEndian.Uint64(raw)
	if lsd == 0 {
		// this is special case, we don't need to modify lastSubmittedDataHeight
		return nil
	}
	pd.lastSubmittedDataHeight.CompareAndSwap(0, lsd)
	return nil
}
