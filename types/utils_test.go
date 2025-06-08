package types_test

import (
	// Import bytes package
	"testing"
	"time" // Used for time.Time comparison

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/stretchr/testify/assert"

	"github.com/rollkit/rollkit/pkg/signer" // Used for signer.Signer interface
	"github.com/rollkit/rollkit/pkg/signer/noop"
	"github.com/rollkit/rollkit/types" // Import the types package
)

var _ time.Time     // Dummy usage to prevent time import from being removed
var _ signer.Signer // Dummy usage to prevent signer import from being removed

func TestGetRandomTx(t *testing.T) {
	// Generate 100 random transactions and check that they are all unique
	txSet := make(map[string]bool)
	for range 100 {
		tx := types.GetRandomTx()
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
	for range 100 {
		bytes := types.GetRandomBytes(32)
		key := string(bytes[:])
		if _, ok := byteSet[key]; ok {
			t.Errorf("Duplicate byte slice generated: %v", bytes)
		}
		byteSet[key] = true
	}
}

func TestGetRandomHeader(t *testing.T) {
	// Generate 100 random headers and check that they are all unique
	headerSet := make(map[string]bool)
	for range 100 {
		appHashBytes := types.GetRandomBytes(32)
		header := types.GetRandomHeader("TestGetRandomHeader", appHashBytes)
		headerHash := header.Hash().String()
		if _, ok := headerSet[headerHash]; ok {
			t.Errorf("Duplicate header generated: %v", header)
		}
		headerSet[headerHash] = true
	}
}

func TestGetFirstSignedHeader(t *testing.T) {
	testCases := []struct {
		name    string
		chainID string
	}{
		{
			name:    "basic first signed header",
			chainID: "test-chain-1",
		},
		{
			name:    "another chain ID",
			chainID: "another-chain",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			privKey, _, err := crypto.GenerateEd25519Key(nil)
			assert.NoError(t, err)
			noopSigner, err := noop.NewNoopSigner(privKey)
			assert.NoError(t, err)

			firstSignedHeader, err := types.GetFirstSignedHeader(noopSigner, tc.chainID)
			assert.NoError(t, err)
			assert.NotNil(t, firstSignedHeader)
			assert.Equal(t, uint64(1), firstSignedHeader.Height())
			assert.Equal(t, tc.chainID, firstSignedHeader.ChainID())
			pubKey, err := noopSigner.GetPublic()
			assert.NoError(t, err)
			expectedAddress := types.KeyAddress(pubKey)
			assert.Equal(t, expectedAddress, firstSignedHeader.ProposerAddress)
			assert.NotNil(t, firstSignedHeader.Signature)

			// Verify signature
			b, err := firstSignedHeader.Header.MarshalBinary()
			assert.NoError(t, err)
			verified, err := privKey.GetPublic().Verify(b, firstSignedHeader.Signature)
			assert.NoError(t, err)
			assert.True(t, verified)
		})
	}
}

func TestGetGenesisWithPrivkey(t *testing.T) {
	testCases := []struct {
		name    string
		chainID string
	}{
		{
			name:    "basic genesis with privkey",
			chainID: "test-chain-1",
		},
		{
			name:    "another chain ID",
			chainID: "another-chain",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gen, privKey, pubKey := types.GetGenesisWithPrivkey(tc.chainID)

			assert.NotNil(t, gen)
			assert.NotNil(t, privKey)
			assert.NotNil(t, pubKey)
			assert.Equal(t, tc.chainID, gen.ChainID)
			assert.Equal(t, uint64(1), gen.InitialHeight)
			assert.NotZero(t, gen.GenesisDAStartTime) // Corrected field name

			// Verify the proposer address in genesis
			expectedAddress := types.KeyAddress(pubKey)
			assert.Equal(t, expectedAddress, gen.ProposerAddress)
		})
	}
}
