package store

import (
	"encoding/binary"
	"sync"

	"github.com/lazyledger/optimint/types"
	"go.uber.org/multierr"
)

var (
	blockPrefix = [1]byte{1}
	indexPrefix = [1]byte{2}
)

type DefaultBlockStore struct {
	db KVStore

	height uint64

	// mtx protects height
	mtx sync.RWMutex
}

var _ BlockStore = &DefaultBlockStore{}

func NewBlockStore() BlockStore {
	return &DefaultBlockStore{db: NewInMemoryKVStore()}
}

func (bs *DefaultBlockStore) Height() uint64 {
	bs.mtx.RLock()
	defer bs.mtx.RUnlock()
	return bs.height
}

func (bs *DefaultBlockStore) SaveBlock(block *types.Block) error {
	// TODO(tzdybal): proper serialization & hashing
	hash, err := types.Hash(&block.Header)
	if err != nil {
		return err
	}
	key := append(blockPrefix[:], hash[:]...)

	height := make([]byte, 8)
	binary.LittleEndian.PutUint64(height, block.Header.Height)
	ikey := append(indexPrefix[:], height[:]...)

	blob, err := block.Serialize()
	if err != nil {
		return err
	}

	bs.mtx.Lock()
	defer bs.mtx.Unlock()
	// TODO(tzdybal): use transaction for consistency of DB
	err = multierr.Append(err, bs.db.Set(key, blob))
	err = multierr.Append(err, bs.db.Set(ikey, hash[:]))

	if block.Header.Height > bs.height {
		bs.height = block.Header.Height
	}

	return err
}

// TODO(tzdybal): what is more common access pattern? by height or by hash?
// currently, we're indexing height->hash, and store blocks by hash, but we might as well store by height
// and index hash->height
func (bs *DefaultBlockStore) LoadBlock(height uint64) (*types.Block, error) {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, height)
	ikey := append(indexPrefix[:], buf[:]...)

	hash, err := bs.db.Get(ikey)

	if err != nil {
		return nil, err
	}

	// TODO(tzdybal): any better way to convert slice to array?
	var h [32]byte
	copy(h[:], hash)
	return bs.LoadBlockByHash(h)
}

func (bs *DefaultBlockStore) LoadBlockByHash(hash [32]byte) (*types.Block, error) {
	key := append(blockPrefix[:], hash[:]...)

	blockData, err := bs.db.Get(key)

	if err != nil {
		return nil, err
	}

	return types.DeserializeBlock(blockData)
}
