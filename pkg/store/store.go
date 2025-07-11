package store

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"

	ds "github.com/ipfs/go-datastore"
	"google.golang.org/protobuf/proto"

	"github.com/rollkit/rollkit/types"
	pb "github.com/rollkit/rollkit/types/pb/rollkit/v1"
)

// DefaultStore is a default store implementation.
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

// SetHeight sets the height saved in the Store if it is higher than the existing height
func (s *DefaultStore) SetHeight(ctx context.Context, height uint64) error {
	currentHeight, err := s.Height(ctx)
	if err != nil {
		return err
	}
	if height <= currentHeight {
		return nil
	}

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

	return nil
}

// GetBlockData returns block header and data at given height, or error if it's not found in Store.
func (s *DefaultStore) GetBlockData(ctx context.Context, height uint64) (*types.SignedHeader, *types.Data, error) {
	header, err := s.GetHeader(ctx, height)
	if err != nil {
		return nil, nil, err
	}
	dataBlob, err := s.db.Get(ctx, ds.NewKey(getDataKey(height)))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load block data: %w", err)
	}
	data := new(types.Data)
	err = data.UnmarshalBinary(dataBlob)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal block data: %w", err)
	}
	return header, data, nil
}

// GetBlockByHash returns block with given block header hash, or error if it's not found in Store.
func (s *DefaultStore) GetBlockByHash(ctx context.Context, hash []byte) (*types.SignedHeader, *types.Data, error) {
	height, err := s.getHeightByHash(ctx, hash)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load height from index %w", err)
	}
	return s.GetBlockData(ctx, height)
}

// getHeightByHash returns height for a block with given block header hash.
func (s *DefaultStore) getHeightByHash(ctx context.Context, hash []byte) (uint64, error) {
	heightBytes, err := s.db.Get(ctx, ds.NewKey(getIndexKey(hash)))
	if err != nil {
		return 0, fmt.Errorf("failed to get height for hash %v: %w", hash, err)
	}
	height, err := decodeHeight(heightBytes)
	if err != nil {
		return 0, fmt.Errorf("failed to decode height: %w", err)
	}
	return height, nil
}

// GetSignatureByHash returns signature for a block at given height, or error if it's not found in Store.
func (s *DefaultStore) GetSignatureByHash(ctx context.Context, hash []byte) (*types.Signature, error) {
	height, err := s.getHeightByHash(ctx, hash)
	if err != nil {
		return nil, fmt.Errorf("failed to load hash from index: %w", err)
	}
	return s.GetSignature(ctx, height)
}

// GetHeader returns the header at the given height or error if it's not found in Store.
func (s *DefaultStore) GetHeader(ctx context.Context, height uint64) (*types.SignedHeader, error) {
	headerBlob, err := s.db.Get(ctx, ds.NewKey(getHeaderKey(height)))
	if err != nil {
		return nil, fmt.Errorf("load block header: %w", err)
	}
	header := new(types.SignedHeader)
	if err = header.UnmarshalBinary(headerBlob); err != nil {
		return nil, fmt.Errorf("unmarshal block header: %w", err)
	}
	return header, nil
}

// GetSignature returns signature for a block with given block header hash, or error if it's not found in Store.
func (s *DefaultStore) GetSignature(ctx context.Context, height uint64) (*types.Signature, error) {
	signatureData, err := s.db.Get(ctx, ds.NewKey(getSignatureKey(height)))
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve signature from height %v: %w", height, err)
	}
	signature := types.Signature(signatureData)
	return &signature, nil
}

// UpdateState updates state saved in Store. Only one State is stored.
// If there is no State in Store, state will be saved.
func (s *DefaultStore) UpdateState(ctx context.Context, state types.State) error {
	pbState, err := state.ToProto()
	if err != nil {
		return fmt.Errorf("failed to convert type state to protobuf type: %w", err)
	}
	data, err := proto.Marshal(pbState)
	if err != nil {
		return fmt.Errorf("failed to marshal state to protobuf: %w", err)
	}
	return s.db.Put(ctx, ds.NewKey(getStateKey()), data)
}

// GetState returns last state saved with UpdateState.
func (s *DefaultStore) GetState(ctx context.Context) (types.State, error) {
	blob, err := s.db.Get(ctx, ds.NewKey(getStateKey()))
	if err != nil {
		return types.State{}, fmt.Errorf("failed to retrieve state: %w", err)
	}
	var pbState pb.State
	err = proto.Unmarshal(blob, &pbState)
	if err != nil {
		return types.State{}, fmt.Errorf("failed to unmarshal state from protobuf: %w", err)
	}

	var state types.State
	err = state.FromProto(&pbState)
	return state, err
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

const heightLength = 8

func encodeHeight(height uint64) []byte {
	heightBytes := make([]byte, heightLength)
	binary.LittleEndian.PutUint64(heightBytes, height)
	return heightBytes
}

func decodeHeight(heightBytes []byte) (uint64, error) {
	if len(heightBytes) != heightLength {
		return 0, fmt.Errorf("invalid height length: %d (expected %d)", len(heightBytes), heightLength)
	}
	return binary.LittleEndian.Uint64(heightBytes), nil
}
