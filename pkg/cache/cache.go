package cache

import (
	"sync"
)

// Cache is a generic cache that maintains items that are seen and hard confirmed
type Cache[T any] struct {
	items      *sync.Map
	hashes     *sync.Map
	daIncluded *sync.Map
}

// NewCache returns a new Cache struct
func NewCache[T any]() *Cache[T] {
	return &Cache[T]{
		items:      new(sync.Map),
		hashes:     new(sync.Map),
		daIncluded: new(sync.Map),
	}
}

func (c *Cache[T]) GetItem(height uint64) *T {
	item, ok := c.items.Load(height)
	if !ok {
		return nil
	}
	val := item.(*T)
	return val
}

func (c *Cache[T]) SetItem(height uint64, item *T) {
	c.items.Store(height, item)
}

func (c *Cache[T]) DeleteItem(height uint64) {
	c.items.Delete(height)
}

func (c *Cache[T]) IsSeen(hash string) bool {
	seen, ok := c.hashes.Load(hash)
	if !ok {
		return false
	}
	return seen.(bool)
}

func (c *Cache[T]) SetSeen(hash string) {
	c.hashes.Store(hash, true)
}

func (c *Cache[T]) IsDAIncluded(hash string) bool {
	daIncluded, ok := c.daIncluded.Load(hash)
	if !ok {
		return false
	}
	return daIncluded.(bool)
}

func (c *Cache[T]) SetDAIncluded(hash string) {
	c.daIncluded.Store(hash, true)
}
