package store

import "github.com/dgraph-io/badger/v3"

var _ KVStore = &BadgerKV{}

type BadgerKV struct {
	db *badger.DB
}

func (b *BadgerKV) Get(key []byte) ([]byte, error) {
	txn := b.db.NewTransaction(false)
	defer txn.Discard()
	item, err := txn.Get(key)
	if err != nil {
		return nil, err
	}
	return item.ValueCopy(nil)
}

func (b *BadgerKV) Set(key []byte, value []byte) error {
	txn := b.db.NewTransaction(true)
	err := txn.Set(key, value)
	if err != nil {
		txn.Discard()
		return err
	}
	return txn.Commit()
}

func (b *BadgerKV) Delete(key []byte) error {
	txn := b.db.NewTransaction(true)
	err := txn.Delete(key)
	if err != nil {
		txn.Discard()
		return err
	}
	return txn.Commit()
}
