package store

import (
	"context"
	"path/filepath"
	"time"

	"github.com/dgraph-io/badger/v3"
	ds "github.com/ipfs/go-datastore"
	dsq "github.com/ipfs/go-datastore/query"
	badger3 "github.com/ipfs/go-ds-badger3"
)

// NewDefaultInMemoryKVStore builds KVStore that works in-memory (without accessing disk).
func NewDefaultInMemoryKVStore() (ds.Datastore, error) {
	inMemoryOptions := &badger3.Options{
		GcDiscardRatio: 0.2,
		GcInterval:     15 * time.Minute,
		GcSleep:        10 * time.Second,
		Options:        badger.DefaultOptions("").WithInMemory(true),
	}
	return badger3.NewDatastore("", inMemoryOptions)
}

// NewDefaultKVStore creates instance of default key-value store.
func NewDefaultKVStore(rootDir, dbPath, dbName string) (ds.Datastore, error) {
	path := filepath.Join(rootify(rootDir, dbPath), dbName)
	return badger3.NewDatastore(path, nil)
}

func PrefixEntries(ctx context.Context, store ds.Datastore, prefix string) ([]dsq.Entry, error) {
	results, err := store.Query(ctx, dsq.Query{Prefix: prefix})
	if err != nil {
		return nil, err
	}
	defer func() {
		err = results.Close()
	}()

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
