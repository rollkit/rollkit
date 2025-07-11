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

	// GetBlockData returns block at given height, or error if it's not found in Store.
	GetBlockData(ctx context.Context, height uint64) (*types.SignedHeader, *types.Data, error)
	// GetBlockByHash returns block with given block header hash, or error if it's not found in Store.
	GetBlockByHash(ctx context.Context, hash []byte) (*types.SignedHeader, *types.Data, error)

	// GetHeader returns the header at the given height or error if it's not found in Store.
	GetHeader(ctx context.Context, height uint64) (*types.SignedHeader, error)

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

	// RollbackToHeight reverts the store state to the specified height.
	// This removes all blocks and state data at heights greater than the target height.
	// Requirements:
	// - Must be atomic - either fully succeeds or leaves state unchanged
	// - Must validate that target height exists and is less than current height
	// - Must update store height to the target height
	// - Must preserve all data at or below the target height
	//
	// Parameters:
	// - ctx: Context for timeout/cancellation control
	// - targetHeight: Height to rollback to (must be >= 1 and < current height)
	//
	// Returns:
	// - error: Any errors during rollback operation
	RollbackToHeight(ctx context.Context, targetHeight uint64) error

	// Close safely closes underlying data storage, to ensure that data is actually saved.
	Close() error
}
