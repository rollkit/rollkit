package store

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"

	tmstate "github.com/tendermint/tendermint/proto/tendermint/state"
	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"
	tmtypes "github.com/tendermint/tendermint/types"
	"go.uber.org/multierr"

	"github.com/celestiaorg/optimint/state"
	"github.com/celestiaorg/optimint/types"
)

var (
	blockPrefix      = [1]byte{1}
	indexPrefix      = [1]byte{2}
	commitPrefix     = [1]byte{3}
	statePrefix      = [1]byte{4}
	responsesPrefix  = [1]byte{5}
	validatorsPrefix = [1]byte{6}
)

// DefaultStore is a default store implmementation.
type DefaultStore struct {
	db KVStore

	height uint64
}

var _ Store = &DefaultStore{}

// New returns new, default store.
func New(kv KVStore) Store {
	return &DefaultStore{
		db: kv,
	}
}

// Height returns height of the highest block saved in the Store.
func (s *DefaultStore) Height() uint64 {
	return atomic.LoadUint64(&s.height)
}

// SaveBlock adds block to the store along with corresponding commit.
// Stored height is updated if block height is greater than stored value.
func (s *DefaultStore) SaveBlock(block *types.Block, commit *types.Commit) error {
	hash := block.Header.Hash()
	blockBlob, err := block.MarshalBinary()
	if err != nil {
		return fmt.Errorf("error encoding Block into binary form: %w", err)
	}

	commitBlob, err := commit.MarshalBinary()
	if err != nil {
		return fmt.Errorf("error encoding Commit into binary form: %w", err)
	}

	bb := s.db.NewBatch()
	err = multierr.Append(err, bb.Set(getBlockKey(hash), blockBlob))
	err = multierr.Append(err, bb.Set(getCommitKey(hash), commitBlob))
	err = multierr.Append(err, bb.Set(getIndexKey(block.Header.Height), hash[:]))

	if err != nil {
		bb.Discard()
		return err
	}

	if err = bb.Commit(); err != nil {
		return fmt.Errorf("error committing a transaction: %w", err)
	}

	if block.Header.Height > atomic.LoadUint64(&s.height) {
		atomic.StoreUint64(&s.height, block.Header.Height)
	}

	return nil
}

// LoadBlock returns block at given height, or error if it's not found in Store.
// TODO(tzdybal): what is more common access pattern? by height or by hash?
// currently, we're indexing height->hash, and store blocks by hash, but we might as well store by height
// and index hash->height
func (s *DefaultStore) LoadBlock(height uint64) (*types.Block, error) {
	h, err := s.loadHashFromIndex(height)
	if err != nil {
		return nil, fmt.Errorf("error loading hash from index: %w", err)
	}
	return s.LoadBlockByHash(h)
}

// LoadBlockByHash returns block with given block header hash, or error if it's not found in Store.
func (s *DefaultStore) LoadBlockByHash(hash [32]byte) (*types.Block, error) {
	blockData, err := s.db.Get(getBlockKey(hash))
	if err != nil {
		return nil, fmt.Errorf("failed to load block data: %w", err)
	}
	block := new(types.Block)
	err = block.UnmarshalBinary(blockData)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal block data: %w", err)
	}

	return block, nil
}

// SaveBlockResponses saves block responses (events, tx responses, validator set updates, etc) in Store.
func (s *DefaultStore) SaveBlockResponses(height uint64, responses *tmstate.ABCIResponses) error {
	data, err := responses.Marshal()
	if err != nil {
		return fmt.Errorf("error marshalling response: %w", err)
	}
	return s.db.Set(getResponsesKey(height), data)
}

// LoadBlockResponses returns block results at given height, or error if it's not found in Store.
func (s *DefaultStore) LoadBlockResponses(height uint64) (*tmstate.ABCIResponses, error) {
	data, err := s.db.Get(getResponsesKey(height))
	if err != nil {
		return nil, fmt.Errorf("error retrieving block results from height %v: %w", height, err)
	}
	var responses tmstate.ABCIResponses
	err = responses.Unmarshal(data)
	return &responses, fmt.Errorf("error unmarshalling data: %w", err)
}

// LoadCommit returns commit for a block at given height, or error if it's not found in Store.
func (s *DefaultStore) LoadCommit(height uint64) (*types.Commit, error) {
	hash, err := s.loadHashFromIndex(height)
	if err != nil {
		return nil, fmt.Errorf("error loading hash from index: %w", err)
	}
	return s.LoadCommitByHash(hash)
}

// LoadCommitByHash returns commit for a block with given block header hash, or error if it's not found in Store.
func (s *DefaultStore) LoadCommitByHash(hash [32]byte) (*types.Commit, error) {
	commitData, err := s.db.Get(getCommitKey(hash))
	if err != nil {
		return nil, fmt.Errorf("error retrieving commit from hash %v: %w", hash, err)
	}
	commit := new(types.Commit)
	err = commit.UnmarshalBinary(commitData)
	return commit, fmt.Errorf("error decoding binary data of Commit into object: %w", err)
}

// UpdateState updates state saved in Store. Only one State is stored.
// If there is no State in Store, state will be saved.
func (s *DefaultStore) UpdateState(state state.State) error {
	blob, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("error marshalling state into JSON encoding: %w", err)
	}
	return s.db.Set(getStateKey(), blob)
}

// LoadState returns last state saved with UpdateState.
func (s *DefaultStore) LoadState() (state.State, error) {
	var state state.State

	blob, err := s.db.Get(getStateKey())
	if err != nil {
		return state, fmt.Errorf("error marshalling state into JSON encoding: %w", err)
	}

	err = json.Unmarshal(blob, &state)
	atomic.StoreUint64(&s.height, uint64(state.LastBlockHeight))
	return state, err
}

func (s *DefaultStore) SaveValidators(height uint64, validatorSet *tmtypes.ValidatorSet) error {
	pbValSet, err := validatorSet.ToProto()
	if err != nil {
		return fmt.Errorf("error converting ValidatorSet to protobuf: %w", err)
	}
	blob, err := pbValSet.Marshal()
	if err != nil {
		return fmt.Errorf("error marshalling ValidatorSet: %w", err)
	}

	return s.db.Set(getValidatorsKey(height), blob)
}

func (s *DefaultStore) LoadValidators(height uint64) (*tmtypes.ValidatorSet, error) {
	blob, err := s.db.Get(getValidatorsKey(height))
	if err != nil {
		return nil, fmt.Errorf("error loading Validators by height %v: %w", height, err)
	}
	var pbValSet tmproto.ValidatorSet
	err = pbValSet.Unmarshal(blob)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling protobuf ValidatorSet: %w", err)
	}

	return tmtypes.ValidatorSetFromProto(&pbValSet)
}

func (s *DefaultStore) loadHashFromIndex(height uint64) ([32]byte, error) {
	blob, err := s.db.Get(getIndexKey(height))

	var hash [32]byte
	if err != nil {
		return hash, fmt.Errorf("error loading hash by height %v: %w", height, err)
	}
	if len(blob) != len(hash) {
		return hash, errors.New("invalid hash length")
	}
	copy(hash[:], blob)
	return hash, nil
}

func getBlockKey(hash [32]byte) []byte {
	return append(blockPrefix[:], hash[:]...)
}

func getCommitKey(hash [32]byte) []byte {
	return append(commitPrefix[:], hash[:]...)
}

func getIndexKey(height uint64) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, height)
	return append(indexPrefix[:], buf[:]...)
}

func getStateKey() []byte {
	return statePrefix[:]
}

func getResponsesKey(height uint64) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, height)
	return append(responsesPrefix[:], buf[:]...)
}

func getValidatorsKey(height uint64) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, height)
	return append(validatorsPrefix[:], buf[:]...)
}
