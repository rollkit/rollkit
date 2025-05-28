package cache

import (
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
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

// GetItemByHash returns an item from the cache by hash
func (c *Cache[T]) GetItemByHash(hash string) *T {
	item, ok := c.items.Load(hash)
	if !ok {
		return nil
	}
	return item.(*T)
}

// SetItemByHash sets an item in the cache by hash
func (c *Cache[T]) SetItemByHash(hash string, item *T) {
	c.items.Store(hash, item)
}

// DeleteItemByHash deletes an item from the cache by hash
func (c *Cache[T]) DeleteItemByHash(hash string) {
	c.items.Delete(hash)
}

// GetItem returns an item from the cache by height
func (c *Cache[T]) GetItem(height uint64) *T {
	item, ok := c.items.Load(height)
	if !ok {
		return nil
	}
	val := item.(*T)
	return val
}

// SetItem sets an item in the cache by height
func (c *Cache[T]) SetItem(height uint64, item *T) {
	c.items.Store(height, item)
}

// DeleteItem deletes an item from the cache by height
func (c *Cache[T]) DeleteItem(height uint64) {
	c.items.Delete(height)
}

// IsSeen returns true if the hash has been seen
func (c *Cache[T]) IsSeen(hash string) bool {
	seen, ok := c.hashes.Load(hash)
	if !ok {
		return false
	}
	return seen.(bool)
}

// SetSeen sets the hash as seen
func (c *Cache[T]) SetSeen(hash string) {
	c.hashes.Store(hash, true)
}

// IsDAIncluded returns true if the hash has been DA-included
func (c *Cache[T]) IsDAIncluded(hash string) bool {
	daIncluded, ok := c.daIncluded.Load(hash)
	if !ok {
		return false
	}
	return daIncluded.(bool)
}

// SetDAIncluded sets the hash as DA-included
func (c *Cache[T]) SetDAIncluded(hash string) {
	c.daIncluded.Store(hash, true)
}

const (
	itemsByHeightFilename = "items_by_height.gob"
	itemsByHashFilename   = "items_by_hash.gob"
	hashesFilename        = "hashes.gob"
	daIncludedFilename    = "da_included.gob"
)

// saveMapGob saves a map to a file using gob encoding.
func saveMapGob[K comparable, V any](filePath string, data map[K]V) error {
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", filePath, err)
	}
	defer file.Close()

	encoder := gob.NewEncoder(file)
	if err := encoder.Encode(data); err != nil {
		return fmt.Errorf("failed to encode to file %s: %w", filePath, err)
	}
	return nil
}

// loadMapGob loads a map from a file using gob encoding.
// if the file does not exist, it returns an empty map and no error.
func loadMapGob[K comparable, V any](filePath string) (map[K]V, error) {
	m := make(map[K]V)
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return m, nil // return empty map if file not found
		}
		return nil, fmt.Errorf("failed to open file %s: %w", filePath, err)
	}
	defer file.Close()

	decoder := gob.NewDecoder(file)
	if err := decoder.Decode(&m); err != nil {
		return nil, fmt.Errorf("failed to decode file %s: %w", filePath, err)
	}
	return m, nil
}

// SaveToDisk saves the cache contents to disk in the specified folder.
// It's the caller's responsibility to ensure that type T (and any types it contains)
// are registered with the gob package if necessary (e.g., using gob.Register).
func (c *Cache[T]) SaveToDisk(folderPath string) error {
	if err := os.MkdirAll(folderPath, 0o755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", folderPath, err)
	}

	// prepare items maps
	itemsByHeightMap := make(map[uint64]*T)
	itemsByHashMap := make(map[string]*T)

	var invalidItemsErr error
	c.items.Range(func(k, v interface{}) bool {
		itemVal, ok := v.(*T)
		if !ok {
			invalidItemsErr = fmt.Errorf("invalid item type: %T", v)
			return false // early exit if the value is not of type *T
		}
		switch key := k.(type) {
		case uint64:
			itemsByHeightMap[key] = itemVal
		case string:
			itemsByHashMap[key] = itemVal
		}
		return true
	})

	if invalidItemsErr != nil {
		return invalidItemsErr
	}

	if err := saveMapGob(filepath.Join(folderPath, itemsByHeightFilename), itemsByHeightMap); err != nil {
		return err
	}
	if err := saveMapGob(filepath.Join(folderPath, itemsByHashFilename), itemsByHashMap); err != nil {
		return err
	}

	// prepare hashes map
	hashesToSave := make(map[string]bool)
	c.hashes.Range(func(k, v interface{}) bool {
		keyStr, okKey := k.(string)
		valBool, okVal := v.(bool)
		if okKey && okVal {
			hashesToSave[keyStr] = valBool
		}
		return true
	})
	if err := saveMapGob(filepath.Join(folderPath, hashesFilename), hashesToSave); err != nil {
		return err
	}

	// prepare daIncluded map
	daIncludedToSave := make(map[string]bool)
	c.daIncluded.Range(func(k, v interface{}) bool {
		keyStr, okKey := k.(string)
		valBool, okVal := v.(bool)
		if okKey && okVal {
			daIncludedToSave[keyStr] = valBool
		}
		return true
	})
	return saveMapGob(filepath.Join(folderPath, daIncludedFilename), daIncludedToSave)
}

// LoadFromDisk loads the cache contents from disk from the specified folder.
// It populates the current cache instance. If files are missing, corresponding parts of the cache will be empty.
// It's the caller's responsibility to ensure that type T (and any types it contains)
// are registered with the gob package if necessary (e.g., using gob.Register).
func (c *Cache[T]) LoadFromDisk(folderPath string) error {
	// load items by height
	itemsByHeightMap, err := loadMapGob[uint64, *T](filepath.Join(folderPath, itemsByHeightFilename))
	if err != nil {
		return fmt.Errorf("failed to load items by height: %w", err)
	}
	for k, v := range itemsByHeightMap {
		c.items.Store(k, v)
	}

	// load items by hash
	itemsByHashMap, err := loadMapGob[string, *T](filepath.Join(folderPath, itemsByHashFilename))
	if err != nil {
		return fmt.Errorf("failed to load items by hash: %w", err)
	}
	for k, v := range itemsByHashMap {
		c.items.Store(k, v)
	}

	// load hashes
	hashesMap, err := loadMapGob[string, bool](filepath.Join(folderPath, hashesFilename))
	if err != nil {
		return fmt.Errorf("failed to load hashes: %w", err)
	}
	for k, v := range hashesMap {
		c.hashes.Store(k, v)
	}

	// load daIncluded
	daIncludedMap, err := loadMapGob[string, bool](filepath.Join(folderPath, daIncludedFilename))
	if err != nil {
		return fmt.Errorf("failed to load daIncluded: %w", err)
	}
	for k, v := range daIncludedMap {
		c.daIncluded.Store(k, v)
	}

	return nil
}
