package types

import (
	"testing"

	"github.com/cometbft/cometbft/crypto/ed25519"
	"github.com/cometbft/cometbft/crypto/secp256k1"
	"github.com/cometbft/cometbft/crypto/sr25519"
	"github.com/cometbft/cometbft/p2p"
	"github.com/libp2p/go-libp2p/core/crypto/pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	ed25519Key := p2p.NodeKey{
		PrivKey: ed25519.GenPrivKey(),
	}
	secp256k1Key := p2p.NodeKey{
		PrivKey: secp256k1.GenPrivKey(),
	}
	invalid := p2p.NodeKey{
		PrivKey: sr25519.GenPrivKey(),
	}

	cases := []struct {
		name         string
		input        *p2p.NodeKey
		expectedType pb.KeyType
		err          error
	}{
		{"nil", nil, pb.KeyType(-1), errNilKey},
		{"empty", &p2p.NodeKey{}, pb.KeyType(-1), errNilKey},
		{"secp256k1", &secp256k1Key, pb.KeyType_Secp256k1, nil},
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
