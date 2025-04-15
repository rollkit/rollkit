package store

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"

	ds "github.com/ipfs/go-datastore"
	"google.golang.org/protobuf/proto"

	"github.com/rollkit/rollkit/types"
	pb "github.com/rollkit/rollkit/types/pb/rollkit/v1"
)

var (
	headerPrefix    = "h"
	dataPrefix      = "d"
	indexPrefix     = "i"
	signaturePrefix = "c"
	statePrefix     = "s"
	metaPrefix      = "m"
	heightPrefix    = "t"
)

// DefaultStore is a default store implmementation.
type DefaultStore struct {
	db ds.Batching
}

var _ Store = &DefaultStore{}

// New returns new, default store.
func New(ds ds.Batching) Store {
	return &DefaultStore{
		db: ds,
	}
}

// Close safely closes underlying data storage, to ensure that data is actually saved.
func (s *DefaultStore) Close() error {
	return s.db.Close()
}

// SetHeight sets the height saved in the Store. It allows setting a lower height.
func (s *DefaultStore) SetHeight(ctx context.Context, height uint64) error {
	// No need to check current height when setting, allow overwrite/rollback
	heightBytes := encodeHeight(height)
	return s.db.Put(ctx, ds.NewKey(getHeightKey()), heightBytes)
}

// Height returns height of the highest block saved in the Store.
func (s *DefaultStore) Height(ctx context.Context) (uint64, error) {
	heightBytes, err := s.db.Get(ctx, ds.NewKey(getHeightKey()))
	if errors.Is(err, ds.ErrNotFound) {
		return 0, nil
	}
	if err != nil {
		// Since we can't return an error due to interface constraints,
		// we log by returning 0 when there's an error reading from disk
		return 0, err
	}

	height, err := decodeHeight(heightBytes)
	if err != nil {
		return 0, err
	}
	return height, nil
}

// SaveBlockData adds block header and data to the store along with corresponding signature.
// Stored height is updated if block height is greater than stored value.
func (s *DefaultStore) SaveBlockData(ctx context.Context, header *types.SignedHeader, data *types.Data, signature *types.Signature) error {
	hash := header.Hash()
	height := header.Height()
	signatureHash := *signature
	headerBlob, err := header.MarshalBinary()
	if err != nil {
		return fmt.Errorf("failed to marshal Header to binary: %w", err)
	}
	dataBlob, err := data.MarshalBinary()
	if err != nil {
		return fmt.Errorf("failed to marshal Data to binary: %w", err)
	}

	batch, err := s.db.Batch(ctx)
	if err != nil {
		return fmt.Errorf("failed to create a new batch: %w", err)
	}

	if err := batch.Put(ctx, ds.NewKey(getHeaderKey(height)), headerBlob); err != nil {
		return fmt.Errorf("failed to put header blob in batch: %w", err)
	}
	if err := batch.Put(ctx, ds.NewKey(getDataKey(height)), dataBlob); err != nil {
		return fmt.Errorf("failed to put data blob in batch: %w", err)
	}
	if err := batch.Put(ctx, ds.NewKey(getSignatureKey(height)), signatureHash[:]); err != nil {
		return fmt.Errorf("failed to put signature of block blob in batch: %w", err)
	}
	if err := batch.Put(ctx, ds.NewKey(getIndexKey(hash)), encodeHeight(height)); err != nil {
		return fmt.Errorf("failed to put index key in batch: %w", err)
	}
	if err := batch.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit batch: %w", err)
	}

	// Update height only if the new block's height is greater
	currentHeight, err := s.Height(ctx)
	if err != nil && !errors.Is(err, ds.ErrNotFound) {
		return fmt.Errorf("failed to get current height after saving block: %w", err)
	}
	if height > currentHeight {
		if err := s.SetHeight(ctx, height); err != nil {
			return fmt.Errorf("failed to set new height after saving block: %w", err)
		}
	}

	return nil
}

// TODO: We unmarshal the header and data here, but then we re-marshal them to proto to hash or send them to DA, we should not unmarshal them here and allow the caller to handle them as needed.

// GetBlockData returns block header and data at given height, or error if it's not found in Store.
func (s *DefaultStore) GetBlockData(ctx context.Context, height uint64) (*types.SignedHeader, *types.Data, error) {
	headerBlob, err := s.db.Get(ctx, ds.NewKey(getHeaderKey(height)))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load block header at height %d: %w", height, err)
	}
	header := new(types.SignedHeader)
	err = header.UnmarshalBinary(headerBlob)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal block header at height %d: %w", height, err)
	}

	dataBlob, err := s.db.Get(ctx, ds.NewKey(getDataKey(height)))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load block data at height %d: %w", height, err)
	}
	data := new(types.Data)
	err = data.UnmarshalBinary(dataBlob)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal block data at height %d: %w", height, err)
	}
	return header, data, nil
}

// GetBlockByHash returns block with given block header hash, or error if it's not found in Store.
func (s *DefaultStore) GetBlockByHash(ctx context.Context, hash []byte) (*types.SignedHeader, *types.Data, error) {
	height, err := s.getHeightByHash(ctx, hash)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load height from index for hash %s: %w", types.Hash(hash).String(), err)
	}
	return s.GetBlockData(ctx, height)
}

// getHeightByHash returns height for a block with given block header hash.
func (s *DefaultStore) getHeightByHash(ctx context.Context, hash []byte) (uint64, error) {
	heightBytes, err := s.db.Get(ctx, ds.NewKey(getIndexKey(hash)))
	if err != nil {
		return 0, fmt.Errorf("failed to get height for hash %s: %w", types.Hash(hash).String(), err)
	}
	height, err := decodeHeight(heightBytes)
	if err != nil {
		return 0, fmt.Errorf("failed to decode height for hash %s: %w", types.Hash(hash).String(), err)
	}
	return height, nil
}

// GetSignatureByHash returns signature for a block at given height, or error if it's not found in Store.
func (s *DefaultStore) GetSignatureByHash(ctx context.Context, hash []byte) (*types.Signature, error) {
	height, err := s.getHeightByHash(ctx, hash)
	if err != nil {
		return nil, fmt.Errorf("failed to load height from index for hash %s: %w", types.Hash(hash).String(), err)
	}
	return s.GetSignature(ctx, height)
}

// GetSignature returns signature for a block with given block header hash, or error if it's not found in Store.
func (s *DefaultStore) GetSignature(ctx context.Context, height uint64) (*types.Signature, error) {
	signatureData, err := s.db.Get(ctx, ds.NewKey(getSignatureKey(height)))
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve signature from height %d: %w", height, err)
	}
	signature := types.Signature(signatureData)
	return &signature, nil
}

// UpdateState updates state saved in Store. Only one State is stored.
// If there is no State in Store, state will be saved.
func (s *DefaultStore) UpdateState(ctx context.Context, state types.State) error {
	pbState, err := state.ToProto()
	if err != nil {
		return fmt.Errorf("failed to convert state to proto: %w", err)
	}
	data, err := proto.Marshal(pbState)
	if err != nil {
		return fmt.Errorf("failed to marshal state proto: %w", err)
	}
	return s.db.Put(ctx, ds.NewKey(getStateKey()), data)
}

// GetState returns last state saved with UpdateState.
func (s *DefaultStore) GetState(ctx context.Context) (types.State, error) {
	blob, err := s.db.Get(ctx, ds.NewKey(getStateKey()))
	if err != nil {
		return types.State{}, fmt.Errorf("failed to retrieve state blob: %w", err)
	}
	var pbState pb.State
	err = proto.Unmarshal(blob, &pbState)
	if err != nil {
		return types.State{}, fmt.Errorf("failed to unmarshal state proto: %w", err)
	}

	var state types.State
	err = state.FromProto(&pbState)
	if err != nil {
		return types.State{}, fmt.Errorf("failed to convert state from proto: %w", err)
	}
	return state, nil
}

// SetMetadata saves arbitrary value in the store.
//
// Metadata is separated from other data by using prefix in KV.
func (s *DefaultStore) SetMetadata(ctx context.Context, key string, value []byte) error {
	err := s.db.Put(ctx, ds.NewKey(getMetaKey(key)), value)
	if err != nil {
		return fmt.Errorf("failed to set metadata for key '%s': %w", key, err)
	}
	return nil
}

// GetMetadata returns values stored for given key with SetMetadata.
func (s *DefaultStore) GetMetadata(ctx context.Context, key string) ([]byte, error) {
	data, err := s.db.Get(ctx, ds.NewKey(getMetaKey(key)))
	if err != nil {
		return nil, fmt.Errorf("failed to get metadata for key '%s': %w", key, err)
	}
	return data, nil
}

// DeleteBlock removes the block header, data, signature, and index entry for the given height.
func (s *DefaultStore) DeleteBlock(ctx context.Context, height uint64) error {
	// First, get the header to find the hash for the index key
	headerBlob, err := s.db.Get(ctx, ds.NewKey(getHeaderKey(height)))
	if errors.Is(err, ds.ErrNotFound) {
		// If header doesn't exist, assume block is already deleted or never existed.
		// Return nil for idempotency.
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get header for deletion at height %d: %w", height, err)
	}

	header := new(types.SignedHeader)
	err = header.UnmarshalBinary(headerBlob)
	if err != nil {
		// If we can't unmarshal, we can't get the hash to delete the index.
		return fmt.Errorf("failed to unmarshal header for deletion at height %d: %w", height, err)
	}
	hash := header.Hash()

	batch, err := s.db.Batch(ctx)
	if err != nil {
		return fmt.Errorf("failed to create batch for deletion at height %d: %w", height, err)
	}

	// Delete header, data, signature, and index
	// Ignore ErrNotFound for individual deletes to ensure idempotency
	if err := batch.Delete(ctx, ds.NewKey(getHeaderKey(height))); err != nil && !errors.Is(err, ds.ErrNotFound) {
		return fmt.Errorf("failed to delete header in batch at height %d: %w", height, err)
	}
	if err := batch.Delete(ctx, ds.NewKey(getDataKey(height))); err != nil && !errors.Is(err, ds.ErrNotFound) {
		return fmt.Errorf("failed to delete data in batch at height %d: %w", height, err)
	}
	if err := batch.Delete(ctx, ds.NewKey(getSignatureKey(height))); err != nil && !errors.Is(err, ds.ErrNotFound) {
		return fmt.Errorf("failed to delete signature in batch at height %d: %w", height, err)
	}
	if err := batch.Delete(ctx, ds.NewKey(getIndexKey(hash))); err != nil && !errors.Is(err, ds.ErrNotFound) {
		return fmt.Errorf("failed to delete index key in batch for hash %s (height %d): %w", hash.String(), height, err)
	}

	if err := batch.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit deletion batch at height %d: %w", height, err)
	}

	return nil
}

// Rollback deletes blocks from the store from currentHeight down to targetHeight + 1.
// It updates the store's height and relevant metadata keys.
// currentHeight is the current highest block height in the store.
// targetHeight is the height to roll back to (the last height to keep).
func (s *DefaultStore) Rollback(ctx context.Context, currentHeight, targetHeight uint64) error {
	if targetHeight >= currentHeight {
		return fmt.Errorf("target height (%d) must be less than current height (%d)", targetHeight, currentHeight)
	}

	// Delete blocks from currentHeight down to targetHeight + 1
	for h := currentHeight; h > targetHeight; h-- {
		if err := s.DeleteBlock(ctx, h); err != nil {
			// If a block delete fails, stop the process to avoid inconsistent state.
			return fmt.Errorf("failed to delete block at height %d: %w. Rollback partially completed. Store might be in an inconsistent state", h, err)
		}
	}

	// Update the main height record
	if err := s.SetHeight(ctx, targetHeight); err != nil {
		return fmt.Errorf("failed to set final store height to %d: %w", targetHeight, err)
	}

	// Update metadata heights if necessary
	keysToUpdate := []string{LastSubmittedHeightKey, DAIncludedHeightKey}
	for _, key := range keysToUpdate {
		metaBytes, err := s.GetMetadata(ctx, key)
		if err != nil {
			if errors.Is(err, ds.ErrNotFound) {
				continue // Key doesn't exist, nothing to update
			}
			// Log or wrap error? For now, let's return it as it indicates a potential issue reading state.
			// Alternatively, could log and continue if metadata update failure is acceptable.
			return fmt.Errorf("failed to get metadata for key '%s' during rollback: %w", key, err)
		}

		var metaHeight uint64
		// LastSubmittedHeightKey stores height as string, DAIncludedHeightKey as binary
		if key == LastSubmittedHeightKey {
			metaHeight, err = strconv.ParseUint(string(metaBytes), 10, 64)
			if err != nil {
				// Log or return? Let's return for consistency.
				return fmt.Errorf("failed to parse metadata height for key '%s' during rollback: %w", key, err)
			}
		} else if key == DAIncludedHeightKey {
			if len(metaBytes) == 8 { // Ensure it's 8 bytes before decoding
				metaHeight = binary.BigEndian.Uint64(metaBytes)
			} else {
				return fmt.Errorf("invalid metadata format for key '%s' during rollback", key)
			}
		}

		if metaHeight > targetHeight {
			var newValue []byte
			if key == LastSubmittedHeightKey {
				newValue = []byte(strconv.FormatUint(targetHeight, 10))
			} else { // DAIncludedHeightKey
				newValue = make([]byte, 8)
				binary.BigEndian.PutUint64(newValue, targetHeight)
			}
			if err := s.SetMetadata(ctx, key, newValue); err != nil {
				// Return error as failing to update metadata leaves state inconsistent.
				return fmt.Errorf("failed to update metadata for key '%s' during rollback: %w", key, err)
			}
		}
	}

	return nil
}

const heightLength = 8

func encodeHeight(height uint64) []byte {
	heightBytes := make([]byte, heightLength)
	binary.BigEndian.PutUint64(heightBytes, height)
	return heightBytes
}

func decodeHeight(heightBytes []byte) (uint64, error) {
	if len(heightBytes) != heightLength {
		return 0, fmt.Errorf("invalid height length: %d (expected %d)", len(heightBytes), heightLength)
	}
	return binary.BigEndian.Uint64(heightBytes), nil
}
