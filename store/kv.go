package store

import (
	"bytes"
	"context"
	"fmt"
	"path"
	"path/filepath"
	"time"

	ds "github.com/ipfs/go-datastore"
	dsq "github.com/ipfs/go-datastore/query"

	badger4 "github.com/celestiaorg/go-ds-badger4"
)

// NewDefaultInMemoryKVStore builds KVStore that works in-memory (without accessing disk).
func NewDefaultInMemoryKVStore() (ds.TxnDatastore, error) {
	inMemoryOptions := &badger4.Options{
		GcDiscardRatio: 0.2,
		GcInterval:     15 * time.Minute,
		GcSleep:        10 * time.Second,
		Options:        badger4.DefaultOptions.WithInMemory(true),
	}
	return badger4.NewDatastore("", inMemoryOptions)
}

// NewDefaultKVStore creates instance of default key-value store.
func NewDefaultKVStore(rootDir, dbPath, dbName string) (ds.TxnDatastore, error) {
	path := filepath.Join(rootify(rootDir, dbPath), dbName)
	return badger4.NewDatastore(path, nil)
}

// PrefixEntries retrieves all entries in the datastore whose keys have the supplied prefix
func PrefixEntries(ctx context.Context, store ds.Datastore, prefix string) (dsq.Results, error) {
	results, err := store.Query(ctx, dsq.Query{Prefix: prefix})
	if err != nil {
		return nil, err
	}
	return results, nil
}

// GenerateKey ...
func GenerateKey(fields []interface{}) string {
	var b bytes.Buffer
	b.WriteString("/")
	for _, f := range fields {
		b.Write([]byte(fmt.Sprintf("%v", f) + "/"))
	}
	return path.Clean(b.String())
}

// rootify works just like in cosmos-sdk
func rootify(rootDir, dbPath string) string {
	if filepath.IsAbs(dbPath) {
		return dbPath
	}
	return filepath.Join(rootDir, dbPath)
}
