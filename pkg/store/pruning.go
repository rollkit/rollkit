package store

import (
	"context"
	"sync"

	ds "github.com/ipfs/go-datastore"

	"github.com/rollkit/rollkit/pkg/config"
	"github.com/rollkit/rollkit/types"
)

// DefaultPruningStore is for a store that supports pruning of block data.
type DefaultPruningStore struct {
	Store

	config                config.PruningConfig
	latestBlockDataHeight uint64
	mu                    sync.Mutex
}

var _ PruningStore = &DefaultPruningStore{}

func NewDefaultPruningStore(ds ds.Batching, config config.PruningConfig) PruningStore {
	return &DefaultPruningStore{
		Store: &DefaultStore{db: ds},

		config: config,
	}
}

// SaveBlockData saves the block data and updates the latest block data height.
func (s *DefaultPruningStore) SaveBlockData(ctx context.Context, header *types.SignedHeader, data *types.Data, signature *types.Signature) error {
	err := s.Store.SaveBlockData(ctx, header, data, signature)
	if err == nil {
		s.mu.Lock()
		s.latestBlockDataHeight = header.Height()
		s.mu.Unlock()
	}

	return err
}

func (s *DefaultPruningStore) PruneBlockData(ctx context.Context) error {
	// Skip if strategy is none.
	if s.config.Strategy == config.PruningConfigStrategyNone {
		return nil
	}

	// Read latest height after calling SaveBlockData. There is a delay between SetHeight and SaveBlockData.
	s.mu.Lock()
	height := s.latestBlockDataHeight
	s.mu.Unlock()

	// Skip if not the correct interval or latest height is less or equal than number of blocks need to keep.
	if height%s.config.Interval != 0 || height < s.config.KeepRecent {
		return nil
	}

	// Must keep at least 2 blocks(while strategy is everything).
	endHeight := height - 1 - s.config.KeepRecent
	startHeight := uint64(0)
	if endHeight > s.config.Interval {
		startHeight = endHeight - s.config.Interval
	}

	for i := startHeight; i < endHeight; i++ {
		// Could ignore for errors like not found.
		_ = s.DeleteBlockData(ctx, i)
	}

	return nil
}
