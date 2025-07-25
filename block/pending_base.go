package block

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync/atomic"

	ds "github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log/v2"

	"github.com/evstack/ev-node/pkg/store"
)

// pendingBase is a generic struct for tracking items (headers, data, etc.)
// that need to be published to the DA layer in order. It handles persistence
// of the last submitted height and provides methods for retrieving pending items.
type pendingBase[T any] struct {
	logger     logging.EventLogger
	store      store.Store
	metaKey    string
	fetch      func(ctx context.Context, store store.Store, height uint64) (T, error)
	lastHeight atomic.Uint64
}

// newPendingBase constructs a new pendingBase for a given type.
func newPendingBase[T any](store store.Store, logger logging.EventLogger, metaKey string, fetch func(ctx context.Context, store store.Store, height uint64) (T, error)) (*pendingBase[T], error) {
	pb := &pendingBase[T]{
		store:   store,
		logger:  logger,
		metaKey: metaKey,
		fetch:   fetch,
	}
	if err := pb.init(); err != nil {
		return nil, err
	}
	return pb, nil
}

// getPending returns a sorted slice of pending items of type T.
func (pb *pendingBase[T]) getPending(ctx context.Context) ([]T, error) {
	lastSubmitted := pb.lastHeight.Load()
	height, err := pb.store.Height(ctx)
	if err != nil {
		return nil, err
	}
	if lastSubmitted == height {
		return nil, nil
	}
	if lastSubmitted > height {
		return nil, fmt.Errorf("height of last submitted item (%d) is greater than height of last item (%d)", lastSubmitted, height)
	}
	pending := make([]T, 0, height-lastSubmitted)
	for i := lastSubmitted + 1; i <= height; i++ {
		item, err := pb.fetch(ctx, pb.store, i)
		if err != nil {
			return pending, err
		}
		pending = append(pending, item)
	}
	return pending, nil
}

func (pb *pendingBase[T]) isEmpty() bool {
	height, err := pb.store.Height(context.Background())
	if err != nil {
		pb.logger.Error("failed to get height in isEmpty", "err", err)
		return false
	}
	return height == pb.lastHeight.Load()
}

func (pb *pendingBase[T]) numPending() uint64 {
	height, err := pb.store.Height(context.Background())
	if err != nil {
		pb.logger.Error("failed to get height in numPending", "err", err)
		return 0
	}
	return height - pb.lastHeight.Load()
}

func (pb *pendingBase[T]) setLastSubmittedHeight(ctx context.Context, newLastSubmittedHeight uint64) {
	lsh := pb.lastHeight.Load()
	if newLastSubmittedHeight > lsh && pb.lastHeight.CompareAndSwap(lsh, newLastSubmittedHeight) {
		bz := make([]byte, 8)
		binary.LittleEndian.PutUint64(bz, newLastSubmittedHeight)
		err := pb.store.SetMetadata(ctx, pb.metaKey, bz)
		if err != nil {
			pb.logger.Error("failed to store height of latest item submitted to DA", "err", err)
		}
	}
}

func (pb *pendingBase[T]) init() error {
	raw, err := pb.store.GetMetadata(context.Background(), pb.metaKey)
	if errors.Is(err, ds.ErrNotFound) {
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
		return nil
	}
	pb.lastHeight.CompareAndSwap(0, lsh)
	return nil
}
