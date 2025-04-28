package store

import (
	"context"

	"github.com/rollkit/rollkit/types"
)

// Store is minimal interface for storing and retrieving blocks, commits and state.
type Store interface {
	// Height returns height of the highest block in store.
	Height(ctx context.Context) (uint64, error)

	// SetHeight sets the height saved in the Store if it is higher than the existing height.
	SetHeight(ctx context.Context, height uint64) error

	// SaveBlockData saves block along with its seen signature (which will be included in the next block).
	SaveBlockData(ctx context.Context, header *types.SignedHeader, data *types.Data, signature *types.Signature) error
	// DeleteBlockData deletes block at given height.
	DeleteBlockData(ctx context.Context, height uint64) error

	// GetBlockData returns block at given height, or error if it's not found in Store.
	GetBlockData(ctx context.Context, height uint64) (*types.SignedHeader, *types.Data, error)
	// GetBlockByHash returns block with given block header hash, or error if it's not found in Store.
	GetBlockByHash(ctx context.Context, hash []byte) (*types.SignedHeader, *types.Data, error)

	// GetSignature returns signature for a block at given height, or error if it's not found in Store.
	GetSignature(ctx context.Context, height uint64) (*types.Signature, error)
	// GetSignatureByHash returns signature for a block with given block header hash, or error if it's not found in Store.
	GetSignatureByHash(ctx context.Context, hash []byte) (*types.Signature, error)

	// UpdateState updates state saved in Store. Only one State is stored.
	// If there is no State in Store, state will be saved.
	UpdateState(ctx context.Context, state types.State) error
	// GetState returns last state saved with UpdateState.
	GetState(ctx context.Context) (types.State, error)

	// SetMetadata saves arbitrary value in the store.
	//
	// This method enables rollkit to safely persist any information.
	SetMetadata(ctx context.Context, key string, value []byte) error

	// GetMetadata returns values stored for given key with SetMetadata.
	GetMetadata(ctx context.Context, key string) ([]byte, error)

	// Close safely closes underlying data storage, to ensure that data is actually saved.
	Close() error
}

type PruningStore interface {
	Store

	PruneBlockData(ctx context.Context) error
}
