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

func (hc *HeaderCache) getHeader(height uint64) (*types.SignedHeader, bool) {
	header, ok := hc.headers.Load(height)
	if !ok {
		return nil, false
	}
	return header.(*types.SignedHeader), true
}

func (hc *HeaderCache) setHeader(height uint64, header *types.SignedHeader) {
	if header != nil {
		hc.headers.Store(height, header)
	}
}

func (hc *HeaderCache) deleteHeader(height uint64) {
	hc.headers.Delete(height)
}

func (hc *HeaderCache) isSeen(hash string) bool {
	seen, ok := hc.hashes.Load(hash)
	if !ok {
		return false
	}
	return seen.(bool)
}

func (hc *HeaderCache) setSeen(hash string) {
	hc.hashes.Store(hash, true)
}

func (hc *HeaderCache) isDAIncluded(hash string) bool {
	daIncluded, ok := hc.daIncluded.Load(hash)
	if !ok {
		return false
	}
	return daIncluded.(bool)
}

func (hc *HeaderCache) setDAIncluded(hash string) {
	hc.daIncluded.Store(hash, true)
}
