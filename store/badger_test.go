package store

import (
	"context"
	"errors"
	"testing"

	"github.com/dgraph-io/badger/v3"
	"github.com/ipfs/go-datastore"
)

func TestGetErrors(t *testing.T) {
	dalcKV, _ := NewDefaultInMemoryKVStore()

	tc := []struct {
		name string
		key  []byte
		err  error
	}{
		{"empty key", []byte{}, badger.ErrEmptyKey},
		{"not found key", []byte("missing key"), ErrKeyNotFound},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			_, err := dalcKV.Get(context.Background(), datastore.NewKey(string(tt.key)))
			if !errors.Is(err, tt.err) {
				t.Errorf("Invalid err, got: %v expected %v", err, tt.err)
			}
		})
	}
}

func TestSetErrors(t *testing.T) {
	dalcKV, _ := NewDefaultInMemoryKVStore()

	tc := []struct {
		name  string
		key   []byte
		value []byte
		err   error
	}{
		{"empty key", []byte{}, []byte{}, badger.ErrEmptyKey},
		{"invalid key", []byte("!badger!key"), []byte("invalid header"), badger.ErrInvalidKey},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			err := dalcKV.Put(context.Background(), datastore.NewKey(string(tt.key)), tt.value)
			if !errors.Is(tt.err, err) {
				t.Errorf("Invalid err, got: %v expected %v", err, tt.err)
			}
		})
	}
}

func TestDeleteErrors(t *testing.T) {
	dalcKV, _ := NewDefaultInMemoryKVStore()

	tc := []struct {
		name string
		key  []byte
		err  error
	}{
		{"empty key", []byte{}, badger.ErrEmptyKey},
		{"invalid key", []byte("!badger!key"), badger.ErrInvalidKey},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			err := dalcKV.Delete(context.Background(), datastore.NewKey(string(tt.key)))
			if !errors.Is(err, tt.err) {
				t.Errorf("Invalid err, got: %v expected %v", err, tt.err)
			}
		})
	}
}
