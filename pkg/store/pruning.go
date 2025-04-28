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

func (s *DefaultPruningStore) PruneBlockData(ctx context.Context) error {
	var (
		err error
	)

	// Skip if strategy is none.
	if s.Config.Strategy == config.PruningConfigStrategyNone {
		return nil
	}

	height, err := s.Height(ctx)
	if err != nil {
		return err
	}

	// Skip if it's a correct interval or latest height is less or equal than number of blocks need to keep.
	if height%s.Config.Interval != 0 || height < s.Config.KeepRecent {
		return nil
	}

	// Must keep at least 2 blocks(while strategy is everything).
	endHeight := height + 1 - s.Config.KeepRecent
	startHeight := uint64(0)
	if endHeight > s.Config.Interval {
		startHeight = endHeight - s.Config.Interval
	}

	for i := startHeight; i <= endHeight; i++ {
		err = s.DeleteBlockData(ctx, i)
		if err != nil {
			// Could ignore for errors like not found.
			// This err is a placeholder for warning if there is a logger in the Store.
			continue
		}
	}

	return nil
}
