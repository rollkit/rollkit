package cache

import (
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewCache verifies that NewCache initializes correctly
func TestNewCache(t *testing.T) {
	cache := NewCache[string]()
	require.NotNil(t, cache, "NewCache returned nil")
	assert.NotNil(t, cache.items, "items map not initialized")
	assert.NotNil(t, cache.hashes, "hashes map not initialized")
	assert.NotNil(t, cache.daIncluded, "daIncluded map not initialized")
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
	cache.SetDAIncluded(testHash, 1)
	if !cache.IsDAIncluded(testHash) {
		t.Error("Hash should be DA-included after SetDAIncluded")
	}

	// Test non-existent hash
	if cache.IsDAIncluded("nonexistenthash") {
		t.Error("Non-existent hash should not be DA-included")
	}
}

// TestCacheDAIncludedHeightOperations tests the DA-included height operations,
// specifically the new GetDAIncludedHeight method which returns the DA height
// where a hash was included, replacing the previous boolean-only tracking.
func TestCacheDAIncludedHeightOperations(t *testing.T) {
	cache := NewCache[int]()
	testHash := "test-hash-123"

	// Test initial state - not DA-included
	if cache.IsDAIncluded(testHash) {
		t.Error("Hash should not be DA-included initially")
	}

	// Test GetDAIncludedHeight for non-existent hash
	height, ok := cache.GetDAIncludedHeight(testHash)
	if ok {
		t.Error("GetDAIncludedHeight should return false for non-existent hash")
	}
	if height != 0 {
		t.Error("Height should be 0 for non-existent hash")
	}

	// Test setting and getting DA-included height
	expectedHeight := uint64(42)
	cache.SetDAIncluded(testHash, expectedHeight)

	if !cache.IsDAIncluded(testHash) {
		t.Error("Hash should be DA-included after SetDAIncluded")
	}

	height, ok = cache.GetDAIncludedHeight(testHash)
	if !ok {
		t.Error("GetDAIncludedHeight should return true for DA-included hash")
	}
	if height != expectedHeight {
		t.Errorf("Expected height %d, got %d", expectedHeight, height)
	}

	// Test updating height for same hash
	newHeight := uint64(100)
	cache.SetDAIncluded(testHash, newHeight)

	height, ok = cache.GetDAIncludedHeight(testHash)
	if !ok {
		t.Error("GetDAIncludedHeight should still return true after update")
	}
	if height != newHeight {
		t.Errorf("Expected updated height %d, got %d", newHeight, height)
	}
}

// TestCacheDAIncludedHeightConcurrency tests concurrent access to the DA-included height operations.
// This ensures that the new height-based DA inclusion tracking is thread-safe when accessed
// from multiple goroutines simultaneously, which is critical for the DA includer workflow.
func TestCacheDAIncludedHeightConcurrency(t *testing.T) {
	cache := NewCache[int]()
	numGoroutines := 10
	numOperations := 100

	var wg sync.WaitGroup
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				hash := fmt.Sprintf("hash-%d-%d", goroutineID, j)
				height := uint64(goroutineID*1000 + j)

				// Test concurrent height operations
				cache.SetDAIncluded(hash, height)
				retrievedHeight, ok := cache.GetDAIncludedHeight(hash)
				if !ok {
					t.Errorf("Hash %s should be DA-included", hash)
				}
				if retrievedHeight != height {
					t.Errorf("Expected height %d for hash %s, got %d", height, hash, retrievedHeight)
				}

				// Test IsDAIncluded still works
				if !cache.IsDAIncluded(hash) {
					t.Errorf("Hash %s should be DA-included", hash)
				}
			}
		}(i)
	}
	wg.Wait()
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
				cache.SetDAIncluded(hash, 1)
				_ = cache.IsDAIncluded(hash)
			}
		}(i)
	}

	wg.Wait()
}

// TestCachePersistence tests saving and loading the cache to/from disk.
func TestCachePersistence(t *testing.T) {
	tempDir := t.TempDir()

	// register the type for gob encoding if it's a custom struct
	// for basic types like string, int, this is not strictly necessary
	// but good practice for generic cache testing.
	type testStruct struct {
		ID   int
		Data string
	}
	gob.Register(&testStruct{}) // register pointer to ensure compatibility

	cache1 := NewCache[testStruct]()
	val1 := testStruct{ID: 1, Data: "hello"}
	val2 := testStruct{ID: 2, Data: "world"}
	hash1 := "hash_val1"
	hash2 := "hash_val2"

	// populate cache1
	cache1.SetItem(100, &val1)
	cache1.SetItem(200, &val2)
	cache1.SetSeen(hash1)
	cache1.SetDAIncluded(hash2, 1)

	// save cache1 to disk
	err := cache1.SaveToDisk(tempDir)
	require.NoError(t, err, "Failed to save cache to disk")

	// create a new cache and load from disk
	cache2 := NewCache[testStruct]()
	err = cache2.LoadFromDisk(tempDir)
	require.NoError(t, err, "Failed to load cache from disk")

	// verify cache2 contents
	// items by height
	retrievedVal1Height := cache2.GetItem(100)
	assert.NotNil(t, retrievedVal1Height, "Item by height 100 should exist")
	assert.Equal(t, val1, *retrievedVal1Height, "Item by height 100 mismatch")

	retrievedVal2Height := cache2.GetItem(200)
	assert.NotNil(t, retrievedVal2Height, "Item by height 200 should exist")
	assert.Equal(t, val2, *retrievedVal2Height, "Item by height 200 mismatch")
	// seen hashes
	assert.True(t, cache2.IsSeen(hash1), "Hash hash_val1 should be seen")
	assert.False(t, cache2.IsSeen(hash2), "Hash hash_val2 should not be seen unless explicitly set") // only hash1 was SetSeen

	// daIncluded hashes
	assert.True(t, cache2.IsDAIncluded(hash2), "Hash hash_val2 should be DAIncluded")
	assert.False(t, cache2.IsDAIncluded(hash1), "Hash hash_val1 should not be DAIncluded unless explicitly set") // only hash2 was SetDAIncluded

	// test loading from a non-existent directory (should not error, should be empty)
	cache3 := NewCache[testStruct]()
	err = cache3.LoadFromDisk(filepath.Join(tempDir, "nonexistent_subdir"))
	require.NoError(t, err, "Loading from non-existent dir should not error")
	assert.Nil(t, cache3.GetItem(100), "Cache should be empty after loading from non-existent dir")

	// test saving to a path where a file exists (should error)
	filePath := filepath.Join(tempDir, "file_exists_test")
	_, err = os.Create(filePath)
	require.NoError(t, err)
	cache4 := NewCache[testStruct]()
	err = cache4.SaveToDisk(filePath) // try to save to a path that is a file
	assert.Error(t, err, "Saving to a path that is a file should error")
}

// TestCachePersistence_EmptyCache tests saving and loading an empty cache.
func TestCachePersistence_EmptyCache(t *testing.T) {
	tempDir := t.TempDir()

	gob.Register(&struct{}{}) // register dummy struct for generic type T

	cache1 := NewCache[struct{}]()

	err := cache1.SaveToDisk(tempDir)
	require.NoError(t, err, "Failed to save empty cache")

	cache2 := NewCache[struct{}]()
	err = cache2.LoadFromDisk(tempDir)
	require.NoError(t, err, "Failed to load empty cache")

	// check a few operations to ensure it's empty
	assert.Nil(t, cache2.GetItem(1), "Item should not exist in loaded empty cache")
	assert.False(t, cache2.IsSeen("somehash"), "Hash should not be seen in loaded empty cache")
	assert.False(t, cache2.IsDAIncluded("somehash"), "Hash should not be DA-included in loaded empty cache")
}

// TestCachePersistence_Overwrite tests overwriting existing cache files.
func TestCachePersistence_Overwrite(t *testing.T) {
	tempDir := t.TempDir()

	gob.Register(0) // register int for generic type T

	// initial save
	cache1 := NewCache[int]()
	val1 := 123
	cache1.SetItem(1, &val1)
	cache1.SetSeen("hash1")
	err := cache1.SaveToDisk(tempDir)
	require.NoError(t, err)

	// second save (overwrite)
	cache2 := NewCache[int]()
	val2 := 456
	cache2.SetItem(2, &val2)
	cache2.SetDAIncluded("hash2", 1)
	err = cache2.SaveToDisk(tempDir) // save to the same directory
	require.NoError(t, err)

	// load and verify (should contain only cache2's data)
	cache3 := NewCache[int]()
	err = cache3.LoadFromDisk(tempDir)
	require.NoError(t, err)

	assert.Nil(t, cache3.GetItem(1), "Item from first save should be overwritten")
	assert.False(t, cache3.IsSeen("hash1"), "Seen hash from first save should be overwritten")

	loadedVal2 := cache3.GetItem(2)
	assert.NotNil(t, loadedVal2)
	assert.Equal(t, val2, *loadedVal2, "Item from second save should be present")
	assert.True(t, cache3.IsDAIncluded("hash2"), "DAIncluded hash from second save should be present")
}
