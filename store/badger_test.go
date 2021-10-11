package store

import (
	"testing"

	"github.com/dgraph-io/badger/v3"
)

func TestGetErrors(t *testing.T) {
	dalcKV := NewInMemoryKVStore()

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
			_, err := dalcKV.Get(tt.key)
			if err.Error() != tt.err.Error() {
				t.Errorf("Invalid err, got: %v expected %v", err, tt.err)
			}
		})
	}
}

func TestSetErrors(t *testing.T) {
	dalcKV := NewInMemoryKVStore()

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
			err := dalcKV.Set(tt.key, tt.value)
			if err.Error() != tt.err.Error() {
				t.Errorf("Invalid err, got: %v expected %v", err, tt.err)
			}
		})
	}
}

func TestDeleteErrors(t *testing.T) {
	dalcKV := NewInMemoryKVStore()

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
			err := dalcKV.Delete(tt.key)
			if err.Error() != tt.err.Error() {
				t.Errorf("Invalid err, got: %v expected %v", err, tt.err)
			}
		})
	}
}
