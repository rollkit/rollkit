package store

import (
	"context"

	"github.com/rollkit/rollkit/types"
)

// HeaderStore is the interface for storing and retrieving block headers.
type HeaderStore interface {
	// Height returns height of the highest header saved in the Store.
	Height() uint64

	// SetHeight sets the height saved in the Store if it is higher than the existing height
	SetHeight(ctx context.Context, height uint64)

	// SaveHeader adds a block header to the store.
	// Stored height is updated if header height is greater than stored value.
	SaveHeader(ctx context.Context, header *types.SignedHeader) error

	// GetHeaderByHash returns header at given hash, or error if it's not found in Store.
	GetHeaderByHash(ctx context.Context, hash types.Hash) (*types.SignedHeader, error)

	// GetHeader returns header at given height, or error if it's not found in Store.
	GetHeader(ctx context.Context, height uint64) (*types.SignedHeader, error)

	// Close safely closes underlying data storage, to ensure that data is actually saved.
	Close() error
}

// DataStore is the interface for storing and retrieving block data.
type DataStore interface {
	// Height returns height of the highest data saved in the Store.
	Height() uint64

	// SetHeight sets the height saved in the Store if it is higher than the existing height
	SetHeight(ctx context.Context, height uint64)

	// SaveData adds block data to the store.
	// Stored height is updated if data height is greater than stored value.
	SaveData(ctx context.Context, data *types.Data) error

	// GetDataByHash returns data at given hash, or error if it's not found in Store.
	GetDataByHash(ctx context.Context, hash types.Hash) (*types.Data, error)

	// GetData returns data at given height, or error if it's not found in Store.
	GetData(ctx context.Context, height uint64) (*types.Data, error)

	// Close safely closes underlying data storage, to ensure that data is actually saved.
	Close() error
}
