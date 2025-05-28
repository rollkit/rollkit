package store

import (
	"context"
	"time"

	ds "github.com/ipfs/go-datastore"

	"github.com/rollkit/rollkit/pkg/config"
	"github.com/rollkit/rollkit/types"
)

const DefaultFlushInterval = 1 * time.Second

type defaultPruningStore struct {
	Store

	Config config.PruningConfig
}

var _ PruningStore = &defaultPruningStore{}

// newDefaultPruningStore returns default pruning store.
func newDefaultPruningStore(ds ds.Batching, config config.PruningConfig) PruningStore {
	return &defaultPruningStore{
		Store:  &DefaultStore{db: ds},
		Config: config,
	}
}

func (s *defaultPruningStore) PruneBlockData(ctx context.Context, height uint64) error {
	// Skip if strategy is none.
	if s.Config.Strategy == config.PruningConfigStrategyNone {
		return nil
	}

	// Skip if not the correct interval or latest height is less or equal than number of blocks need to keep.
	if height%s.Config.Interval != 0 || height < s.Config.KeepRecent {
		return nil
	}

	// Must keep at least 2 blocks(while strategy is everything).
	endHeight := height + 1 - s.Config.KeepRecent
	startHeight := uint64(0)
	if endHeight > s.Config.Interval {
		startHeight = endHeight - s.Config.Interval
	}

	for i := startHeight; i < endHeight; i++ {
		// Could ignore for errors like not found.
		_ = s.DeleteBlockData(ctx, i)
	}

	return nil
}

type AsyncPruningStore struct {
	PruningStore

	latestBlockDataHeight uint64
	flushInterval         time.Duration
}

var _ PruningStore = &AsyncPruningStore{}

func NewAsyncPruningStore(ds ds.Batching, config config.PruningConfig) *AsyncPruningStore {
	pruningStore := newDefaultPruningStore(ds, config)

	// todo: initialize latestBlockDataHeight from the store
	return &AsyncPruningStore{
		PruningStore: pruningStore,

		flushInterval: DefaultFlushInterval,
	}
}

// SaveBlockData saves the block data and updates the latest block data height.
func (s *AsyncPruningStore) SaveBlockData(ctx context.Context, header *types.SignedHeader, data *types.Data, signature *types.Signature) error {
	err := s.PruningStore.SaveBlockData(ctx, header, data, signature)
	if err == nil {
		s.latestBlockDataHeight = header.Height()
	}

	return err
}

// Start begins the pruning process at the specified interval.
func (s *AsyncPruningStore) Start(ctx context.Context) {
	// todo: use ctx for cancellation
	ticker := time.NewTicker(s.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Currently PruneBlockData only returns nil.
			_ = s.PruneBlockData(ctx, s.latestBlockDataHeight)
		}
	}
}
