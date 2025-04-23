package store

import (
	"context"
	"github.com/rollkit/rollkit/pkg/config"
	"github.com/rollkit/rollkit/types"
)

type DefaultPruningStore struct {
	Store

	Config config.PruningConfig
}

var _ PruningStore = &DefaultPruningStore{}

// NewDefaultPruningStore returns default pruning store.
func NewDefaultPruningStore(store Store, config config.PruningConfig) PruningStore {
	return &DefaultPruningStore{
		Store:  store,
		Config: config,
	}
}

// Close safely closes underlying data storage, to ensure that data is actually saved.
func (s *DefaultPruningStore) Close() error {
	return s.Store.Close()
}

// SetHeight sets the height saved in the Store if it is higher than the existing height
func (s *DefaultPruningStore) SetHeight(ctx context.Context, height uint64) error {
	return s.Store.SetHeight(ctx, height)
}

// Height returns height of the highest block saved in the Store.
func (s *DefaultPruningStore) Height(ctx context.Context) (uint64, error) {
	return s.Store.Height(ctx)
}

// SaveBlockData adds block header and data to the store along with corresponding signature.
// Stored height is updated if block height is greater than stored value.
func (s *DefaultPruningStore) SaveBlockData(ctx context.Context, header *types.SignedHeader, data *types.Data, signature *types.Signature) error {
	return s.Store.SaveBlockData(ctx, header, data, signature)
}

// DeleteBlockData deletes block at given height.
func (s *DefaultPruningStore) DeleteBlockData(ctx context.Context, height uint64) error {
	return s.Store.DeleteBlockData(ctx, height)
}

// GetBlockData returns block header and data at given height, or error if it's not found in Store.
func (s *DefaultPruningStore) GetBlockData(ctx context.Context, height uint64) (*types.SignedHeader, *types.Data, error) {
	return s.Store.GetBlockData(ctx, height)
}

// GetBlockByHash returns block with given block header hash, or error if it's not found in Store.
func (s *DefaultPruningStore) GetBlockByHash(ctx context.Context, hash []byte) (*types.SignedHeader, *types.Data, error) {
	return s.Store.GetBlockByHash(ctx, hash)
}

// GetSignatureByHash returns signature for a block at given height, or error if it's not found in Store.
func (s *DefaultPruningStore) GetSignatureByHash(ctx context.Context, hash []byte) (*types.Signature, error) {
	return s.Store.GetSignatureByHash(ctx, hash)
}

// GetSignature returns signature for a block with given block header hash, or error if it's not found in Store.
func (s *DefaultPruningStore) GetSignature(ctx context.Context, height uint64) (*types.Signature, error) {
	return s.Store.GetSignature(ctx, height)
}

// UpdateState updates state saved in Store. Only one State is stored.
// If there is no State in Store, state will be saved.
func (s *DefaultPruningStore) UpdateState(ctx context.Context, state types.State) error {
	return s.Store.UpdateState(ctx, state)
}

// GetState returns last state saved with UpdateState.
func (s *DefaultPruningStore) GetState(ctx context.Context) (types.State, error) {
	return s.Store.GetState(ctx)
}

// SetMetadata saves arbitrary value in the store.
//
// Metadata is separated from other data by using prefix in KV.
func (s *DefaultPruningStore) SetMetadata(ctx context.Context, key string, value []byte) error {
	return s.Store.SetMetadata(ctx, key, value)
}

// GetMetadata returns values stored for given key with SetMetadata.
func (s *DefaultPruningStore) GetMetadata(ctx context.Context, key string) ([]byte, error) {
	return s.Store.GetMetadata(ctx, key)
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
			return err
		}
	}

	return nil
}
