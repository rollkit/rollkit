package store

import (
	"context"
	"fmt"
	"sync/atomic"

	ds "github.com/ipfs/go-datastore"

	"github.com/rollkit/rollkit/types"
)

// DefaultHeaderStore is a default implementation of the HeaderStore interface.
type DefaultHeaderStore struct {
	db     ds.Batching
	height atomic.Uint64
}

var _ HeaderStore = &DefaultHeaderStore{}

// NewHeaderStore returns a new DefaultHeaderStore.
func NewHeaderStore(ds ds.Batching) HeaderStore {
	return &DefaultHeaderStore{
		db: ds,
	}
}

// Close safely closes underlying data storage, to ensure that data is actually saved.
func (s *DefaultHeaderStore) Close() error {
	return s.db.Close()
}

// SetHeight sets the height saved in the Store if it is higher than the existing height
func (s *DefaultHeaderStore) SetHeight(ctx context.Context, height uint64) {
	for {
		storeHeight := s.height.Load()
		if height <= storeHeight {
			break
		}
		if s.height.CompareAndSwap(storeHeight, height) {
			break
		}
	}
}

// Height returns height of the highest header saved in the Store.
func (s *DefaultHeaderStore) Height() uint64 {
	return s.height.Load()
}

// SaveHeader adds a block header to the store.
// Stored height is updated if header height is greater than stored value.
func (s *DefaultHeaderStore) SaveHeader(ctx context.Context, header *types.SignedHeader) error {
	hash := header.Hash()
	height := header.Height()
	headerBlob, err := header.MarshalBinary()
	if err != nil {
		return fmt.Errorf("failed to marshal Header to binary: %w", err)
	}

	batch, err := s.db.Batch(ctx)
	if err != nil {
		return fmt.Errorf("failed to create a new batch: %w", err)
	}

	if err := batch.Put(ctx, ds.NewKey(getHeaderKey(height)), headerBlob); err != nil {
		return fmt.Errorf("failed to put header blob in batch: %w", err)
	}
	if err := batch.Put(ctx, ds.NewKey(getIndexKey(hash)), encodeHeight(height)); err != nil {
		return fmt.Errorf("failed to put index key in batch: %w", err)
	}
	if err := batch.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit batch: %w", err)
	}

	s.SetHeight(ctx, height)
	return nil
}

// GetHeaderByHash returns header at given hash, or error if it's not found in Store.
func (s *DefaultHeaderStore) GetHeaderByHash(ctx context.Context, hash types.Hash) (*types.SignedHeader, error) {
	heightBytes, err := s.db.Get(ctx, ds.NewKey(getIndexKey(hash)))
	if err != nil {
		return nil, fmt.Errorf("failed to load hash from index: %w", err)
	}
	height, err := decodeHeight(heightBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to decode height: %w", err)
	}
	return s.GetHeader(ctx, height)
}

// GetHeader returns header at given height, or error if it's not found in Store.
func (s *DefaultHeaderStore) GetHeader(ctx context.Context, height uint64) (*types.SignedHeader, error) {
	headerBlob, err := s.db.Get(ctx, ds.NewKey(getHeaderKey(height)))
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve header from height %v: %w", height, err)
	}
	var header types.SignedHeader
	err = header.UnmarshalBinary(headerBlob)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal header: %w", err)
	}
	return &header, nil
}

// DefaultDataStore is a default implementation of the DataStore interface.
type DefaultDataStore struct {
	db     ds.Batching
	height atomic.Uint64
}

var _ DataStore = &DefaultDataStore{}

// NewDataStore returns a new DefaultDataStore.
func NewDataStore(ds ds.Batching) DataStore {
	return &DefaultDataStore{
		db: ds,
	}
}

// Close safely closes underlying data storage, to ensure that data is actually saved.
func (s *DefaultDataStore) Close() error {
	return s.db.Close()
}

// SetHeight sets the height saved in the Store if it is higher than the existing height
func (s *DefaultDataStore) SetHeight(ctx context.Context, height uint64) {
	for {
		storeHeight := s.height.Load()
		if height <= storeHeight {
			break
		}
		if s.height.CompareAndSwap(storeHeight, height) {
			break
		}
	}
}

// Height returns height of the highest data saved in the Store.
func (s *DefaultDataStore) Height() uint64 {
	return s.height.Load()
}

// SaveData adds block data to the store.
// Stored height is updated if data height is greater than stored value.
func (s *DefaultDataStore) SaveData(ctx context.Context, data *types.Data) error {
	hash := data.Hash()
	height := data.Metadata.Height
	dataBlob, err := data.MarshalBinary()
	if err != nil {
		return fmt.Errorf("failed to marshal Data to binary: %w", err)
	}

	batch, err := s.db.Batch(ctx)
	if err != nil {
		return fmt.Errorf("failed to create a new batch: %w", err)
	}

	if err := batch.Put(ctx, ds.NewKey(getDataKey(height)), dataBlob); err != nil {
		return fmt.Errorf("failed to put data blob in batch: %w", err)
	}
	if err := batch.Put(ctx, ds.NewKey(getIndexKey(hash)), encodeHeight(height)); err != nil {
		return fmt.Errorf("failed to put index key in batch: %w", err)
	}
	if err := batch.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit batch: %w", err)
	}

	s.SetHeight(ctx, height)
	return nil
}

// GetDataByHash returns data at given hash, or error if it's not found in Store.
func (s *DefaultDataStore) GetDataByHash(ctx context.Context, hash types.Hash) (*types.Data, error) {
	heightBytes, err := s.db.Get(ctx, ds.NewKey(getIndexKey(hash)))
	if err != nil {
		return nil, fmt.Errorf("failed to load hash from index: %w", err)
	}
	height, err := decodeHeight(heightBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to decode height: %w", err)
	}
	return s.GetData(ctx, height)
}

// GetData returns data at given height, or error if it's not found in Store.
func (s *DefaultDataStore) GetData(ctx context.Context, height uint64) (*types.Data, error) {
	dataBlob, err := s.db.Get(ctx, ds.NewKey(getDataKey(height)))
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve data from height %v: %w", height, err)
	}
	var data types.Data
	err = data.UnmarshalBinary(dataBlob)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal data: %w", err)
	}
	return &data, nil
}
