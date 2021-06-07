package store

import (
	"encoding/binary"
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
	key := append(blockPrefix[:], hash[:]...)

	height := make([]byte, 8)
	binary.LittleEndian.PutUint64(height, block.Header.Height)
	ikey := append(indexPrefix[:], height[:]...)

	blob, err := block.MarshalBinary()

	s.mtx.Lock()
	defer s.mtx.Unlock()
	// TODO(tzdybal): use transaction for consistency of DB
	err = multierr.Append(err, s.db.Set(key, blob))
	err = multierr.Append(err, s.db.Set(ikey, hash[:]))

	if block.Header.Height > s.height {
		s.height = block.Header.Height
	}

	return err
}

// TODO(tzdybal): what is more common access pattern? by height or by hash?
// currently, we're indexing height->hash, and store blocks by hash, but we might as well store by height
// and index hash->height
func (s *DefaultStore) LoadBlock(height uint64) (*types.Block, error) {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, height)
	ikey := append(indexPrefix[:], buf[:]...)

	hash, err := s.db.Get(ikey)

	if err != nil {
		return nil, err
	}

	// TODO(tzdybal): any better way to convert slice to array?
	var h [32]byte
	copy(h[:], hash)
	return s.LoadBlockByHash(h)
}

func (s *DefaultStore) LoadBlockByHash(hash [32]byte) (*types.Block, error) {
	key := append(blockPrefix[:], hash[:]...)

	blockData, err := s.db.Get(key)

	if err != nil {
		return nil, err
	}

	block := new(types.Block)
	err = block.UnmarshalBinary(blockData)

	return block, err
}

func (d *DefaultStore) LoadCommit(height uint64) (*types.Block, error) {
	panic("not implemented") // TODO: Implement
}

func (d *DefaultStore) LoadCommitByHash(hash [32]byte) (*types.Block, error) {
	panic("not implemented") // TODO: Implement
}
