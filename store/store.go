package store

import (
	"context"
	"encoding/binary"
	"fmt"
	"strconv"
	"sync/atomic"

	abci "github.com/cometbft/cometbft/abci/types"
	ds "github.com/ipfs/go-datastore"

	"github.com/rollkit/rollkit/types"
	pb "github.com/rollkit/rollkit/types/pb/rollkit/v1"
)

var (
	headerPrefix         = "h"
	dataPrefix           = "d"
	indexPrefix          = "i"
	signaturePrefix      = "c"
	extendedCommitPrefix = "ec"
	statePrefix          = "s"
	responsesPrefix      = "r"
	metaPrefix           = "m"
)

// DefaultStore is a default store implmementation.
type DefaultStore struct {
	db     ds.Batching
	height atomic.Uint64
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
func (s *DefaultStore) SetHeight(ctx context.Context, height uint64) {
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

// Height returns height of the highest block saved in the Store.
func (s *DefaultStore) Height() uint64 {
	return s.height.Load()
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
		return fmt.Errorf("failed to put commit blob in batch: %w", err)
	}
	if err := batch.Put(ctx, ds.NewKey(getIndexKey(hash)), encodeHeight(height)); err != nil {
		return fmt.Errorf("failed to put index key in batch: %w", err)
	}
	if err := batch.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit batch: %w", err)
	}

	return nil
}

// TODO: We unmarshal the header and data here, but then we re-marshal them to proto to hash or send them to DA, we should not unmarshal them here and allow the caller to handle them as needed.

// GetBlockData returns block header and data at given height, or error if it's not found in Store.
func (s *DefaultStore) GetBlockData(ctx context.Context, height uint64) (*types.SignedHeader, *types.Data, error) {
	headerBlob, err := s.db.Get(ctx, ds.NewKey(getHeaderKey(height)))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load block header: %w", err)
	}
	header := new(types.SignedHeader)
	err = header.UnmarshalBinary(headerBlob)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal block header: %w", err)
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
func (s *DefaultStore) GetBlockByHash(ctx context.Context, hash types.Hash) (*types.SignedHeader, *types.Data, error) {
	height, err := s.getHeightByHash(ctx, hash)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load height from index %w", err)
	}
	return s.GetBlockData(ctx, height)
}

// getHeightByHash returns height for a block with given block header hash.
func (s *DefaultStore) getHeightByHash(ctx context.Context, hash types.Hash) (uint64, error) {
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

// SaveBlockResponses saves block responses (events, tx responses, validator set updates, etc) in Store.
func (s *DefaultStore) SaveBlockResponses(ctx context.Context, height uint64, responses *abci.ResponseFinalizeBlock) error {
	data, err := responses.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}
	return s.db.Put(ctx, ds.NewKey(getResponsesKey(height)), data)
}

// GetBlockResponses returns block results at given height, or error if it's not found in Store.
func (s *DefaultStore) GetBlockResponses(ctx context.Context, height uint64) (*abci.ResponseFinalizeBlock, error) {
	data, err := s.db.Get(ctx, ds.NewKey(getResponsesKey(height)))
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve block results from height %v: %w", height, err)
	}
	var responses abci.ResponseFinalizeBlock
	err = responses.Unmarshal(data)
	if err != nil {
		return &responses, fmt.Errorf("failed to unmarshal data: %w", err)
	}
	return &responses, nil
}

// GetSignatureByHash returns signature for a block at given height, or error if it's not found in Store.
func (s *DefaultStore) GetSignatureByHash(ctx context.Context, hash types.Hash) (*types.Signature, error) {
	height, err := s.getHeightByHash(ctx, hash)
	if err != nil {
		return nil, fmt.Errorf("failed to load hash from index: %w", err)
	}
	return s.GetSignature(ctx, height)
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

// SaveExtendedCommit saves extended commit information in Store.
func (s *DefaultStore) SaveExtendedCommit(ctx context.Context, height uint64, commit *abci.ExtendedCommitInfo) error {
	bytes, err := commit.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal Extended Commit: %w", err)
	}
	return s.db.Put(ctx, ds.NewKey(getExtendedCommitKey(height)), bytes)
}

// GetExtendedCommit returns extended commit (commit with vote extensions) for a block at given height.
func (s *DefaultStore) GetExtendedCommit(ctx context.Context, height uint64) (*abci.ExtendedCommitInfo, error) {
	bytes, err := s.db.Get(ctx, ds.NewKey(getExtendedCommitKey(height)))
	if err != nil {
		return nil, fmt.Errorf("failed to load extended commit data: %w", err)
	}
	extendedCommit := new(abci.ExtendedCommitInfo)
	err = extendedCommit.Unmarshal(bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal extended commit: %w", err)
	}
	return extendedCommit, nil
}

// UpdateState updates state saved in Store. Only one State is stored.
// If there is no State in Store, state will be saved.
func (s *DefaultStore) UpdateState(ctx context.Context, state types.State) error {
	pbState, err := state.ToProto()
	if err != nil {
		return fmt.Errorf("failed to marshal state to JSON: %w", err)
	}
	data, err := pbState.Marshal()
	if err != nil {
		return err
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
	err = pbState.Unmarshal(blob)
	if err != nil {
		return types.State{}, fmt.Errorf("failed to unmarshal state from JSON: %w", err)
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

func getHeaderKey(height uint64) string {
	return GenerateKey([]string{headerPrefix, strconv.FormatUint(height, 10)})
}

func getDataKey(height uint64) string {
	return GenerateKey([]string{dataPrefix, strconv.FormatUint(height, 10)})
}

func getSignatureKey(height uint64) string {
	return GenerateKey([]string{signaturePrefix, strconv.FormatUint(height, 10)})
}

func getExtendedCommitKey(height uint64) string {
	return GenerateKey([]string{extendedCommitPrefix, strconv.FormatUint(height, 10)})
}

func getStateKey() string {
	return statePrefix
}

func getResponsesKey(height uint64) string {
	return GenerateKey([]string{responsesPrefix, strconv.FormatUint(height, 10)})
}

func getMetaKey(key string) string {
	return GenerateKey([]string{metaPrefix, key})
}

func getIndexKey(hash types.Hash) string {
	return GenerateKey([]string{indexPrefix, hash.String()})
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
