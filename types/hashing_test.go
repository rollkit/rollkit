package types

import (
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHeaderHash tests the Hash method of the Header.
func TestHeaderHash(t *testing.T) {
	// Create a sample header
	header := &Header{
		BaseHeader: BaseHeader{
			Height: 1,
			Time:   1234567890,
		},
		DataHash: []byte("datahash"),
	}

	hash1 := header.Hash()

	headerBytes, err := header.MarshalBinary()
	require.NoError(t, err)
	expectedHash := sha256.Sum256(headerBytes)

	assert.NotNil(t, hash1)
	assert.Len(t, hash1, sha256.Size)
	assert.Equal(t, Hash(expectedHash[:]), hash1, "Header hash should match manual calculation")

	header.BaseHeader.Height = 2
	hash2 := header.Hash()
	assert.NotEqual(t, hash1, hash2, "Different headers should have different hashes")
}

// TestDataHash tests the Hash method of the Data.
func TestDataHash(t *testing.T) {
	data := &Data{
		Txs: Txs{Tx("tx1"), Tx("tx2")},
		Metadata: &Metadata{
			ChainID:      "test-chain",
			Height:       1,
			Time:         1234567890,
			LastDataHash: []byte("lastdatahash"),
		},
	}

	hash1 := data.Hash()

	dataBytes, err := data.MarshalBinary()
	require.NoError(t, err)

	hasher := sha256.New()
	hasher.Write(leafPrefix)
	hasher.Write(dataBytes)
	expectedHash := hasher.Sum(nil)

	assert.NotNil(t, hash1)
	assert.Len(t, hash1, sha256.Size)
	assert.Equal(t, Hash(expectedHash), hash1, "Data hash should match manual calculation with prefix")

	data.Txs = Txs{Tx("tx3")}
	hash2 := data.Hash()
	assert.NotEqual(t, hash1, hash2, "Different data (Txs) should have different hashes")

	data.Metadata.Height = 2
	hash3 := data.Hash()
	assert.NotEqual(t, hash1, hash3, "Different data (Metadata) should have different hashes")
	assert.NotEqual(t, hash2, hash3)
}

// TestLeafHashOpt tests the helper function leafHashOpt directly.
func TestLeafHashOpt(t *testing.T) {
	data := []byte("some data")
	hasher := sha256.New()

	hash1 := leafHashOpt(hasher, data)

	hasher.Reset()
	hasher.Write(leafPrefix)
	hasher.Write(data)
	expectedHash := hasher.Sum(nil)

	assert.Equal(t, expectedHash, hash1)
	assert.Len(t, hash1, sha256.Size)

	emptyData := []byte{}
	hash2 := leafHashOpt(hasher, emptyData)

	hasher.Reset()
	hasher.Write(leafPrefix)
	hasher.Write(emptyData)
	expectedHash2 := hasher.Sum(nil)

	assert.Equal(t, expectedHash2, hash2)
	assert.NotEqual(t, hash1, hash2)
}
