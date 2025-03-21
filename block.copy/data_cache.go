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

// GetItem returns the data for the given height
func (dc *DataCache) GetItem(height uint64) *types.Data {
	data, ok := dc.data.Load(height)
	if !ok {
		return nil
	}
	return data.(*types.Data)
}

// SetItem sets the data for the given height
func (dc *DataCache) SetItem(height uint64, data *types.Data) {
	dc.data.Store(height, data)
}

// DeleteItem deletes the data for the given height
func (dc *DataCache) DeleteItem(height uint64) {
	dc.data.Delete(height)
}

// IsSeen returns whether the data with the given hash has been seen
func (dc *DataCache) IsSeen(hash string) bool {
	seen, ok := dc.hashes.Load(hash)
	if !ok {
		return false
	}
	return seen.(bool)
}

// SetSeen marks the data with the given hash as seen
func (dc *DataCache) SetSeen(hash string) {
	dc.hashes.Store(hash, true)
}

// IsDAIncluded returns whether the data with the given hash has been included in DA
func (dc *DataCache) IsDAIncluded(hash string) bool {
	daIncluded, ok := dc.daIncluded.Load(hash)
	if !ok {
		return false
	}
	return daIncluded.(bool)
}

// SetDAIncluded marks the data with the given hash as included in DA
func (dc *DataCache) SetDAIncluded(hash string) {
	dc.daIncluded.Store(hash, true)
}
