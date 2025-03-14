package block

import (
	"sync"

	"github.com/rollkit/rollkit/types"
)

// headerCache is a cache for headers.
type headerCache struct {
	mu sync.RWMutex
	// headers maps height -> header
	headers map[uint64]*types.SignedHeader
	// seen maps header hash -> bool
	seen map[string]bool
	// daIncluded maps header hash -> bool
	daIncluded map[string]bool
}

// newHeaderCache creates a new header cache.
func newHeaderCache() *headerCache {
	return &headerCache{
		headers:    make(map[uint64]*types.SignedHeader),
		seen:       make(map[string]bool),
		daIncluded: make(map[string]bool),
	}
}

// getHeader gets a header from the cache.
func (c *headerCache) getHeader(height uint64) *types.SignedHeader {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.headers[height]
}

// setHeader sets a header in the cache.
func (c *headerCache) setHeader(height uint64, header *types.SignedHeader) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.headers[height] = header
}

// deleteHeader deletes a header from the cache.
func (c *headerCache) deleteHeader(height uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.headers, height)
}

// isSeen checks if a header has been seen.
func (c *headerCache) isSeen(hash string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.seen[hash]
}

// setSeen marks a header as seen.
func (c *headerCache) setSeen(hash string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seen[hash] = true
}

// isDAIncluded checks if a header has been included in the DA layer.
func (c *headerCache) isDAIncluded(hash string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.daIncluded[hash]
}

// setDAIncluded marks a header as included in the DA layer.
func (c *headerCache) setDAIncluded(hash string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.daIncluded[hash] = true
}

// dataCache is a cache for data.
type dataCache struct {
	mu sync.RWMutex
	// data maps height -> data
	data map[uint64]*types.Data
	// seen maps data hash -> bool
	seen map[string]bool
	// daIncluded maps data hash -> bool
	daIncluded map[string]bool
}

// newDataCache creates a new data cache.
func newDataCache() *dataCache {
	return &dataCache{
		data:       make(map[uint64]*types.Data),
		seen:       make(map[string]bool),
		daIncluded: make(map[string]bool),
	}
}

// getData gets data from the cache.
func (c *dataCache) getData(height uint64) *types.Data {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.data[height]
}

// setData sets data in the cache.
func (c *dataCache) setData(height uint64, data *types.Data) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[height] = data
}

// deleteData deletes data from the cache.
func (c *dataCache) deleteData(height uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.data, height)
}

// isSeen checks if data has been seen.
func (c *dataCache) isSeen(hash string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.seen[hash]
}

// setSeen marks data as seen.
func (c *dataCache) setSeen(hash string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seen[hash] = true
}

// isDAIncluded checks if data has been included in the DA layer.
func (c *dataCache) isDAIncluded(hash string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.daIncluded[hash]
}

// setDAIncluded marks data as included in the DA layer.
func (c *dataCache) setDAIncluded(hash string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.daIncluded[hash] = true
}
