package store

import (
	"context"
	"path/filepath"
	"time"

	"github.com/dgraph-io/badger/v3"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
	badger3 "github.com/ipfs/go-ds-badger3"
)

// KVStore encapsulates key-value store abstraction, in minimalistic interface.
//
// KVStore MUST be thread safe.
type KVStore interface {
	Get(key []byte) ([]byte, error)        // Get gets the value for a key.
	Set(key []byte, value []byte) error    // Set updates the value for a key.
	Delete(key []byte) error               // Delete deletes a key.
	NewBatch() Batch                       // NewBatch creates a new batch.
	PrefixIterator(prefix []byte) Iterator // PrefixIterator creates iterator to traverse given prefix.
}

// Batch enables batching of transactions.
type Batch interface {
	Set(key, value []byte) error // Accumulates KV entries in a transaction.
	Delete(key []byte) error     // Deletes the given key.
	Commit() error               // Commits the transaction.
	Discard()                    // Discards the transaction.
}

// Iterator enables traversal over a given prefix.
type Iterator interface {
	Valid() bool
	Next()
	Key() []byte
	Value() []byte
	Error() error
	Discard()
}

// NewDefaultInMemoryKVStore builds KVStore that works in-memory (without accessing disk).
func NewDefaultInMemoryKVStore() (datastore.Datastore, error) {
	inMemoryOptions := &badger3.Options{
		GcDiscardRatio: 0.2,
		GcInterval:     15 * time.Minute,
		GcSleep:        10 * time.Second,
		Options:        badger.DefaultOptions("").WithInMemory(true),
	}
	return badger3.NewDatastore("", inMemoryOptions)
}

// NewDefaultKVStore creates instance of default key-value store.
func NewDefaultKVStore(rootDir, dbPath, dbName string) (datastore.Datastore, error) {
	path := filepath.Join(rootify(rootDir, dbPath), dbName)
	return badger3.NewDatastore(path, nil)
}

func PrefixEntries(ctx context.Context, store datastore.Datastore, prefix string) ([]query.Entry, error) {
	results, err := store.Query(ctx, query.Query{Prefix: string(prefix)})
	if err != nil {
		return nil, err
	}
	defer results.Close()

	entries, err := results.Rest()
	if err != nil {
		return nil, err
	}

	return entries, nil
}

// rootify works just like in cosmos-sdk
func rootify(rootDir, dbPath string) string {
	if filepath.IsAbs(dbPath) {
		return dbPath
	}
	return filepath.Join(rootDir, dbPath)
}
