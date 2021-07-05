package store

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"sync"

	"go.uber.org/multierr"

	"github.com/lazyledger/optimint/state"
	"github.com/lazyledger/optimint/types"
)

var (
	blockPrefix  = [1]byte{1}
	indexPrefix  = [1]byte{2}
	commitPrefix = [1]byte{3}
	statePreffix = [1]byte{4}
)

// DefaultStore is a default store implmementation.
type DefaultStore struct {
	db KVStore

	height uint64

	// mtx ensures that db is in sync with height
	mtx sync.RWMutex
}

var _ Store = &DefaultStore{}

// New returns new, default store.
func New() Store {
	return &DefaultStore{db: NewInMemoryKVStore()}
}

// Height returns height of the highest block saved in the Store.
func (s *DefaultStore) Height() uint64 {
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	return s.height
}

// SaveBlock adds block to the store along with corresponding commit.
// Stored height is updated if block height is greater than stored value.
func (s *DefaultStore) SaveBlock(block *types.Block, commit *types.Commit) error {
	hash := block.Header.Hash()
	blockBlob, err := block.MarshalBinary()
	if err != nil {
		return err
	}

	commitBlob, err := commit.MarshalBinary()
	if err != nil {
		return err
	}

	s.mtx.Lock()
	defer s.mtx.Unlock()
	// TODO(tzdybal): use transaction for consistency of DB (https://github.com/lazyledger/optimint/issues/80)
	err = multierr.Append(err, s.db.Set(getBlockKey(hash), blockBlob))
	err = multierr.Append(err, s.db.Set(getCommitKey(hash), commitBlob))
	err = multierr.Append(err, s.db.Set(getIndexKey(block.Header.Height), hash[:]))

	if err != nil {
		return err
	}

	if block.Header.Height > s.height {
		s.height = block.Header.Height
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
		return nil, err
	}
	return s.LoadBlockByHash(h)
}

// LoadBlockByHash returns block with given block header hash, or error if it's not found in Store.
func (s *DefaultStore) LoadBlockByHash(hash [32]byte) (*types.Block, error) {
	blockData, err := s.db.Get(getBlockKey(hash))

	if err != nil {
		return nil, err
	}

	block := new(types.Block)
	err = block.UnmarshalBinary(blockData)

	return block, err
}

// LoadCommit returns commit for a block at given height, or error if it's not found in Store.
func (s *DefaultStore) LoadCommit(height uint64) (*types.Commit, error) {
	hash, err := s.loadHashFromIndex(height)
	if err != nil {
		return nil, err
	}
	return s.LoadCommitByHash(hash)
}

// LoadCommitByHash returns commit for a block with given block header hash, or error if it's not found in Store.
func (s *DefaultStore) LoadCommitByHash(hash [32]byte) (*types.Commit, error) {
	commitData, err := s.db.Get(getCommitKey(hash))
	if err != nil {
		return nil, err
	}
	commit := new(types.Commit)
	err = commit.UnmarshalBinary(commitData)
	return commit, err
}

// UpdateState updates state saved in Store. Only one State is stored.
// If there is no State in Store, state will be saved.
func (s *DefaultStore) UpdateState(state state.State) error {
	blob, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return s.db.Set(getStateKey(), blob)
}

// LoadState returns last state saved with UpdateState.
func (s *DefaultStore) LoadState() (state.State, error) {
	var state state.State

	blob, err := s.db.Get(getStateKey())
	if err != nil {
		return state, err
	}

	err = json.Unmarshal(blob, &state)
	return state, err
}

func (s *DefaultStore) loadHashFromIndex(height uint64) ([32]byte, error) {
	blob, err := s.db.Get(getIndexKey(height))

	var hash [32]byte
	if err != nil {
		return hash, err
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
	return statePreffix[:]
}
