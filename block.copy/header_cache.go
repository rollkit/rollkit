package block

import (
	"sync"

	"github.com/rollkit/rollkit/types"
)

// HeaderCache maintains headers that are seen and hard confirmed
type HeaderCache struct {
	headers    *sync.Map
	hashes     *sync.Map
	daIncluded *sync.Map
}

// NewHeaderCache returns a new HeaderCache struct
func NewHeaderCache() *HeaderCache {
	return &HeaderCache{
		headers:    new(sync.Map),
		hashes:     new(sync.Map),
		daIncluded: new(sync.Map),
	}
}

// GetItem returns the header for the given height
func (hc *HeaderCache) GetItem(height uint64) *types.SignedHeader {
	header, ok := hc.headers.Load(height)
	if !ok {
		return nil
	}
	return header.(*types.SignedHeader)
}

// SetItem sets the header for the given height
func (hc *HeaderCache) SetItem(height uint64, header *types.SignedHeader) {
	hc.headers.Store(height, header)
}

// DeleteItem deletes the header for the given height
func (hc *HeaderCache) DeleteItem(height uint64) {
	hc.headers.Delete(height)
}

// IsSeen returns whether the header with the given hash has been seen
func (hc *HeaderCache) IsSeen(hash string) bool {
	seen, ok := hc.hashes.Load(hash)
	if !ok {
		return false
	}
	return seen.(bool)
}

// SetSeen marks the header with the given hash as seen
func (hc *HeaderCache) SetSeen(hash string) {
	hc.hashes.Store(hash, true)
}

// IsDAIncluded returns whether the header with the given hash has been included in DA
func (hc *HeaderCache) IsDAIncluded(hash string) bool {
	daIncluded, ok := hc.daIncluded.Load(hash)
	if !ok {
		return false
	}
	return daIncluded.(bool)
}

// SetDAIncluded marks the header with the given hash as included in DA
func (hc *HeaderCache) SetDAIncluded(hash string) {
	hc.daIncluded.Store(hash, true)
}