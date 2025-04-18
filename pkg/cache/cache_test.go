package cache

import (
	"fmt"
	"sync"
	"testing"
)

// TestNewCache verifies that NewCache initializes correctly
func TestNewCache(t *testing.T) {
	cache := NewCache[string]()
	if cache == nil {
		t.Fatal("NewCache returned nil")
	}
	if cache.items == nil {
		t.Error("items map not initialized")
	}
	if cache.hashes == nil {
		t.Error("hashes map not initialized")
	}
	if cache.daIncluded == nil {
		t.Error("daIncluded map not initialized")
	}
}

// TestCacheItemOperations tests the item-related operations (Get, Set, Delete)
func TestCacheItemOperations(t *testing.T) {
	cache := NewCache[string]()
	testValue := "test"

	// Test setting and getting an item
	cache.SetItem(1, &testValue)
	if got := cache.GetItem(1); got == nil || *got != testValue {
		t.Errorf("GetItem(1) = %v, want %v", got, testValue)
	}

	// Test getting a non-existent item
	if got := cache.GetItem(2); got != nil {
		t.Errorf("GetItem(2) = %v, want nil", got)
	}

	// Test deleting an item
	cache.DeleteItem(1)
	if got := cache.GetItem(1); got != nil {
		t.Errorf("After DeleteItem(1), GetItem(1) = %v, want nil", got)
	}
}

// TestCacheHashOperations tests the hash-related operations
func TestCacheHashOperations(t *testing.T) {
	cache := NewCache[string]()
	testHash := "testhash123"

	// Test initial state
	if cache.IsSeen(testHash) {
		t.Error("New hash should not be seen initially")
	}

	// Test setting and checking seen status
	cache.SetSeen(testHash)
	if !cache.IsSeen(testHash) {
		t.Error("Hash should be seen after SetSeen")
	}

	// Test non-existent hash
	if cache.IsSeen("nonexistenthash") {
		t.Error("Non-existent hash should not be seen")
	}
}

// TestCacheDAIncludedOperations tests the DA-included operations
func TestCacheDAIncludedOperations(t *testing.T) {
	cache := NewCache[string]()
	testHash := "testhash123"

	// Test initial state
	if cache.IsDAIncluded(testHash) {
		t.Error("New hash should not be DA-included initially")
	}

	// Test setting and checking DA-included status
	cache.SetDAIncluded(testHash)
	if !cache.IsDAIncluded(testHash) {
		t.Error("Hash should be DA-included after SetDAIncluded")
	}

	// Test non-existent hash
	if cache.IsDAIncluded("nonexistenthash") {
		t.Error("Non-existent hash should not be DA-included")
	}
}

// TestCacheConcurrency tests concurrent access to the cache
func TestCacheConcurrency(t *testing.T) {
	cache := NewCache[string]()
	const goroutines = 10
	const operations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < operations; j++ {
				// Test concurrent item operations
				value := fmt.Sprintf("value-%d-%d", id, j)
				cache.SetItem(uint64(j), &value)
				_ = cache.GetItem(uint64(j))
				cache.DeleteItem(uint64(j))

				// Test concurrent hash operations
				hash := fmt.Sprintf("hash-%d-%d", id, j)
				cache.SetSeen(hash)
				_ = cache.IsSeen(hash)

				// Test concurrent DA-included operations
				cache.SetDAIncluded(hash)
				_ = cache.IsDAIncluded(hash)
			}
		}(i)
	}

	wg.Wait()
}
