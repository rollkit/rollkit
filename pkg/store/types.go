package store

import (
	"context"
	"errors"

	ds "github.com/ipfs/go-datastore"

	"github.com/evstack/ev-node/types"
)

// ErrNotFound is returned when a key is not present in the store.
var ErrNotFound = ds.ErrNotFound

// IsNotFound reports whether err (or any error in its chain) is a not-found error.
func IsNotFound(err error) bool { return errors.Is(err, ds.ErrNotFound) }

// Batch provides atomic operations for the store
type Batch interface {
	// SaveBlockData atomically saves the block header, data, and signature
	SaveBlockData(header *types.SignedHeader, data *types.Data, signature *types.Signature) error

	// SaveBlockDataFromBytes is like SaveBlockData but accepts pre-serialized header and data bytes,
	// avoiding re-marshalling when the caller already has the binary blobs.
	SaveBlockDataFromBytes(header *types.SignedHeader, headerBlob, dataBlob []byte, signature *types.Signature) error

	// SetHeight sets the height in the batch
	SetHeight(height uint64) error

	// UpdateState updates the state in the batch
	UpdateState(state types.State) error

	// Commit commits all batch operations atomically
	Commit() error

	// Put adds a put operation to the batch (used internally for rollback)
	Put(key ds.Key, value []byte) error

	// Delete adds a delete operation to the batch (used internally for rollback)
	Delete(key ds.Key) error
}

// Store is minimal interface for storing and retrieving blocks, commits and state.
type Store interface {
	Reader
	Metadata
	Rollback
	Pruner

	// Close safely closes underlying data storage, to ensure that data is actually saved.
	Close() error

	// NewBatch creates a new batch for atomic operations.
	NewBatch(ctx context.Context) (Batch, error)
}

// MetadataKV is a key-value pair for batched metadata operations.
type MetadataKV struct {
	Key   string
	Value []byte
}

type Metadata interface {
	// SetMetadata saves arbitrary value in the store.
	//
	// This method enables evolve to safely persist any information.
	SetMetadata(ctx context.Context, key string, value []byte) error

	// DeleteMetadata removes a metadata key from the store.
	DeleteMetadata(ctx context.Context, key string) error

	// BatchMetadata writes and deletes metadata keys in a single Badger
	// WriteBatch transaction, reducing contention on the write pipeline.
	BatchMetadata(ctx context.Context, puts []MetadataKV, deletes []string) error
}

type Reader interface {
	// Height returns height of the highest block in store.
	Height(ctx context.Context) (uint64, error)

	// GetBlockData returns block at given height, or error if it's not found in Store.
	GetBlockData(ctx context.Context, height uint64) (*types.SignedHeader, *types.Data, error)
	// GetBlockByHash returns block with given block header hash, or error if it's not found in Store.
	GetBlockByHash(ctx context.Context, hash []byte) (*types.SignedHeader, *types.Data, error)
	// GetSignature returns signature for a block at given height, or error if it's not found in Store.
	GetSignature(ctx context.Context, height uint64) (*types.Signature, error)
	// GetSignatureByHash returns signature for a block with given block header hash, or error if it's not found in Store.
	GetSignatureByHash(ctx context.Context, hash []byte) (*types.Signature, error)
	// GetHeader returns the header at given height, or error if it's not found in Store.
	GetHeader(ctx context.Context, height uint64) (*types.SignedHeader, error)

	// GetState returns last state saved with UpdateState.
	GetState(ctx context.Context) (types.State, error)
	// GetStateAtHeight returns state saved at given height, or error if it's not found in Store.
	GetStateAtHeight(ctx context.Context, height uint64) (types.State, error)

	// GetMetadata returns values stored for given key with SetMetadata.
	GetMetadata(ctx context.Context, key string) ([]byte, error)
}

type Rollback interface {
	// Rollback deletes x height from the ev-node store.
	// Aggregator is used to determine if the rollback is performed on the aggregator node.
	Rollback(ctx context.Context, height uint64, aggregator bool) error
}

// Pruner provides long-term, height-based pruning of historical block data.
type Pruner interface {
	// PruneBlocks removes block data (header, data, signature, and hash index)
	// up to and including the given height from the store, without modifying
	// state snapshots or the current chain height.
	PruneBlocks(ctx context.Context, height uint64) error
	// PruneStates removes per-height state snapshots up to and including the
	// given height, tracking progress so repeated calls resume where they left off.
	PruneStates(ctx context.Context, height uint64) error
	// DeleteStateAtHeight removes the state at the given height from the store.
	// It does not affect the current state or any states at other heights, allowing for targeted pruning of historical state snapshots.
	DeleteStateAtHeight(ctx context.Context, height uint64) error
}
