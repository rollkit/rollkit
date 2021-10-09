package store

import (
	"github.com/dgraph-io/badger/v3"
)

var _ KVStore = &BadgerKV{}
var _ Batch = &BadgerBatch{}

// BadgerKV is a implementation of KVStore using Badger v3.
type BadgerKV struct {
	db *badger.DB
}

// Get returns value for given key, or error.
func (b *BadgerKV) Get(key []byte) ([]byte, error) {
	txn := b.db.NewTransaction(false)
	defer txn.Discard()
	item, err := txn.Get(key)
	if err != nil {
		return nil, err
	}
	return item.ValueCopy(nil)
}

// Set saves key-value mapping in store.
func (b *BadgerKV) Set(key []byte, value []byte) error {
	txn := b.db.NewTransaction(true)
	err := txn.Set(key, value)
	if err != nil {
		txn.Discard()
		return err
	}
	return txn.Commit()
}

// Delete removes key and corresponding value from store.
func (b *BadgerKV) Delete(key []byte) error {
	txn := b.db.NewTransaction(true)
	err := txn.Delete(key)
	if err != nil {
		txn.Discard()
		return err
	}
	return txn.Commit()
}

func (b *BadgerKV) NewBatch() Batch {
	//write transactions should be short lived as they use extra resources in badger
	return &BadgerBatch{
		txn: b.db.NewTransaction(true),
	}
}

type BadgerBatch struct {
	txn *badger.Txn
}

func (bb *BadgerBatch) Set(key, value []byte) error {
	if err := bb.txn.Set(key, value); err != nil {
		bb.Discard()
		return err
	}

	return nil
}

func (bb *BadgerBatch) Delete(key []byte) error {
	return bb.txn.Delete(key)
}

func (bb *BadgerBatch) Commit() error {
	return bb.txn.Commit()
}

func (bb *BadgerBatch) Discard() {
	bb.txn.Discard()
}
