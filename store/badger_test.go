package store

import (
	"testing"

	"github.com/dgraph-io/badger"
)

func TestGet(t *testing.T) {
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
