package store

import (
	"path/filepath"

	"github.com/dgraph-io/badger/v3"
)

// KVStore encapsulates key-value store abstraction, in minimalistic interface.
//
// KVStore MUST be thread safe.
type KVStore interface {
	Get(key []byte) ([]byte, error)     // Get gets the value for a key.
	Set(key []byte, value []byte) error // Set updates the value for a key.
	Delete(key []byte) error            // Delete deletes a key.
	NewBatch() Batch
}

type Batch interface {
	Aggregate(key, value []byte) error //Accumulates KV entries in a transaction
	Commit() error                     //Commits the transaction
}

//TAKE A LOOK AT THIS!: https://pkg.go.dev/github.com/dgraph-io/badger/v3#example-Txn.NewIterator

// NewInMemoryKVStore builds KVStore that works in-memory (without accessing disk).
func NewDefaultInMemoryKVStore() KVStore {
	db, err := badger.Open(badger.DefaultOptions("").WithInMemory(true))
	if err != nil {
		panic(err)
	}
	return &BadgerKV{
		db: db,
	}
}

func NewDefaultKVStore(rootDir, dbPath, dbName string) KVStore {
	path := filepath.Join(rootify(rootDir, dbPath), dbName)
	db, err := badger.Open(badger.DefaultOptions(path))
	if err != nil {
		panic(err)
	}
	return &BadgerKV{
		db: db,
	}
}

/*func NewKVBatchWriter(kvstore KVStore, limit uint) KVBatchWriter {
	db := kvstore.GetDb()
	return &BadgerKVBatchWriter{
		db: *db,
		//batch:  db.db.NewWriteBatch(),
		kvlist: make([]*pb.KV, 0, limit),
		limit:  limit,
	}
}*/

// rootify works just like in cosmos-sdk
func rootify(rootDir, dbPath string) string {
	if filepath.IsAbs(dbPath) {
		return dbPath
	}
	return filepath.Join(rootDir, dbPath)
}
