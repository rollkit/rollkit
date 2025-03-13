package types

import (
	"testing"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/crypto/pb"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/p2p/key"
)

func TestGetRandomTx(t *testing.T) {
	// Generate 100 random transactions and check that they are all unique
	txSet := make(map[string]bool)
	for i := 0; i < 100; i++ {
		tx := GetRandomTx()
		key := string(tx[:])
		if _, ok := txSet[key]; ok {
			t.Errorf("Duplicate transaction generated: %v", tx)
		}
		txSet[key] = true
	}
}

func TestGetRandomBytes(t *testing.T) {
	// Generate 100 random byte slices of length 32 and check that they are all unique
	byteSet := make(map[string]bool)
	for i := 0; i < 100; i++ {
		bytes := GetRandomBytes(32)
		key := string(bytes[:])
		if _, ok := byteSet[key]; ok {
			t.Errorf("Duplicate byte slice generated: %v", bytes)
		}
		byteSet[key] = true
	}
}

func TestGetRandomValidatorSet(t *testing.T) {
	// Generate 100 random validator sets and check that they are all unique
	valSetSet := make(map[string]bool)
	for i := 0; i < 100; i++ {
		valSet := GetRandomValidatorSet()
		valSetHash := string(valSet.Hash())
		if _, ok := valSetSet[valSetHash]; ok {
			t.Errorf("Duplicate validator set generated: %v", valSet)
		}
		valSetSet[valSetHash] = true
	}
}

func TestGetRandomHeader(t *testing.T) {
	// Generate 100 random headers and check that they are all unique
	headerSet := make(map[string]bool)
	for i := 0; i < 100; i++ {
		header := GetRandomHeader("TestGetRandomHeader")
		headerHash := header.Hash().String()
		if _, ok := headerSet[headerHash]; ok {
			t.Errorf("Duplicate header generated: %v", header)
		}
		headerSet[headerHash] = true
	}
}

func TestGetNodeKey(t *testing.T) {
	t.Parallel()

	privKey, pubKey, err := crypto.GenerateKeyPair(crypto.Ed25519, 256)
	require.NoError(t, err)
	ed25519Key := key.NodeKey{
		PrivKey: privKey,
		PubKey:  pubKey,
	}
	invalid := key.NodeKey{
		PrivKey: nil,
		PubKey:  nil,
	}

	cases := []struct {
		name         string
		input        *key.NodeKey
		expectedType pb.KeyType
		err          error
	}{
		{"nil", nil, pb.KeyType(-1), errNilKey},
		{"empty", &key.NodeKey{}, pb.KeyType(-1), errNilKey},
		{"ed25519", &ed25519Key, pb.KeyType_Ed25519, nil},
		{"invalid", &invalid, pb.KeyType(-1), errUnsupportedKeyType},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actual, err := GetNodeKey(c.input)
			if c.err != nil {
				assert.Nil(t, actual)
				assert.Error(t, err)
				assert.ErrorIs(t, c.err, err)
			} else {
				require.NotNil(t, actual)
				assert.NoError(t, err)
				assert.Equal(t, c.expectedType, actual.Type())
			}
		})
	}
}
