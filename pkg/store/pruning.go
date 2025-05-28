package store

import (
	"context"

	ds "github.com/ipfs/go-datastore"

	"github.com/rollkit/rollkit/pkg/config"
)

type DefaultPruningStore struct {
	Store

	Config config.PruningConfig
}

var _ PruningStore = &DefaultPruningStore{}

// NewDefaultPruningStore returns default pruning store.
func NewDefaultPruningStore(ds ds.Batching, config config.PruningConfig) PruningStore {
	return &DefaultPruningStore{
		Store:  &DefaultStore{db: ds},
		Config: config,
	}
}

func (s *DefaultPruningStore) PruneBlockData(ctx context.Context, height uint64) error {
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
