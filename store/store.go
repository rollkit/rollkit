package store

import (
	"encoding/binary"
	"errors"
	"sync"

	"go.uber.org/multierr"

	"github.com/lazyledger/optimint/hash"
	"github.com/lazyledger/optimint/types"
)

var (
	blockPrefix  = [1]byte{1}
	indexPrefix  = [1]byte{2}
	commitPrefix = [1]byte{3}
)

type DefaultStore struct {
	db KVStore

	height uint64

	// mtx protects height
	mtx sync.RWMutex
}

var _ Store = &DefaultStore{}

func New() Store {
	return &DefaultStore{db: NewInMemoryKVStore()}
}

func (s *DefaultStore) Height() uint64 {
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	return s.height
}

func (s *DefaultStore) SaveBlock(block *types.Block, commit *types.Commit) error {
	hash, err := hash.Hash(&block.Header)
	if err != nil {
		return err
	}
	blockKey := getBlockKey(hash)
	commitKey := getCommitKey(hash)
	indexKey := getIndexKey(block.Header.Height)

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
	// TODO(tzdybal): use transaction for consistency of DB
	err = multierr.Append(err, s.db.Set(blockKey, blockBlob))
	err = multierr.Append(err, s.db.Set(commitKey, commitBlob))
	err = multierr.Append(err, s.db.Set(indexKey, hash[:]))

	if block.Header.Height > s.height {
		s.height = block.Header.Height
	}

	return err
}

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

func (s *DefaultStore) LoadBlockByHash(hash [32]byte) (*types.Block, error) {
	blockData, err := s.db.Get(getBlockKey(hash))

	if err != nil {
		return nil, err
	}

	block := new(types.Block)
	err = block.UnmarshalBinary(blockData)

	return block, err
}

func (s *DefaultStore) LoadCommit(height uint64) (*types.Commit, error) {
	hash, err := s.loadHashFromIndex(height)
	if err != nil {
		return nil, err
	}
	return s.LoadCommitByHash(hash)
}

func (s *DefaultStore) LoadCommitByHash(hash [32]byte) (*types.Commit, error) {
	commitData, err := s.db.Get(getCommitKey(hash))
	if err != nil {
		return nil, err
	}
	commit := new(types.Commit)
	err = commit.UnmarshalBinary(commitData)
	return commit, err
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
