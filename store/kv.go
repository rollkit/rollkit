package store

import (
	"bytes"
	"context"
	"fmt"
	"path"
	"path/filepath"
	"time"

	"github.com/dgraph-io/badger/v3"
	ds "github.com/ipfs/go-datastore"
	dsq "github.com/ipfs/go-datastore/query"
	badger3 "github.com/ipfs/go-ds-badger3"
)

// NewDefaultInMemoryKVStore builds KVStore that works in-memory (without accessing disk).
func NewDefaultInMemoryKVStore() (ds.TxnDatastore, error) {
	inMemoryOptions := &badger3.Options{
		GcDiscardRatio: 0.2,
		GcInterval:     15 * time.Minute,
		GcSleep:        10 * time.Second,
		Options:        badger.DefaultOptions("").WithInMemory(true),
	}
	return badger3.NewDatastore("", inMemoryOptions)
}

// NewDefaultKVStore creates instance of default key-value store.
func NewDefaultKVStore(rootDir, dbPath, dbName string) (ds.TxnDatastore, error) {
	path := filepath.Join(rootify(rootDir, dbPath), dbName)
	return badger3.NewDatastore(path, nil)
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
