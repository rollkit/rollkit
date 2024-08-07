package block

import (
	"sync"

	"github.com/rollkit/rollkit/types"
)

// DataCache maintains data that are seen and hard confirmed
type DataCache struct {
	data       *sync.Map
	hashes     *sync.Map
	daIncluded *sync.Map
}

// NewDataCache returns a new DataCache struct
func NewDataCache() *DataCache {
	return &DataCache{
		data:       new(sync.Map),
		hashes:     new(sync.Map),
		daIncluded: new(sync.Map),
	}
}

func (hc *DataCache) getData(height uint64) (*types.Data, bool) {
	data, ok := hc.data.Load(height)
	if !ok {
		return nil, false
	}
	return data.(*types.Data), true
}

func (hc *DataCache) setData(height uint64, data *types.Data) {
	if data != nil {
		hc.data.Store(height, data)
	}
}

func (hc *DataCache) deleteData(height uint64) {
	hc.data.Delete(height)
}

func (hc *DataCache) isSeen(hash string) bool {
	seen, ok := hc.hashes.Load(hash)
	if !ok {
		return false
	}
	return seen.(bool)
}

func (hc *DataCache) setSeen(hash string) {
	hc.hashes.Store(hash, true)
}

func (hc *DataCache) isDAIncluded(hash string) bool {
	daIncluded, ok := hc.daIncluded.Load(hash)
	if !ok {
		return false
	}
	return daIncluded.(bool)
}

func (hc *DataCache) setDAIncluded(hash string) {
	hc.daIncluded.Store(hash, true)
}
