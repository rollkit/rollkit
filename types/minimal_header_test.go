package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMinimalHeaderADRCompliance(t *testing.T) {
	// Create a minimal header for testing
	minHeader := &MinimalHeader{
		ParentHash:     Hash{},
		BlockHeight:    1,
		BlockTimestamp: 1000000000,
		BlockChainID:   "test-chain",
		DataCommitment: Hash{},
		StateRoot:      Hash{},
		ExtraData:      []byte("test"),
	}

	// Test the header methods
	assert.Equal(t, uint64(1), minHeader.Height())
	assert.Equal(t, "test-chain", minHeader.ChainID())

	// Test conversion from standard header
	fullHeader := &Header{
		BaseHeader: BaseHeader{
			Height:  2,
			Time:    2000000000,
			ChainID: "full-chain",
		},
		LastHeaderHash:  Hash{},
		DataHash:        Hash{1, 2, 3},
		AppHash:         Hash{4, 5, 6},
		ProposerAddress: []byte("proposer"),
	}

	convertedHeader := FromHeader(fullHeader)
	assert.Equal(t, uint64(2), convertedHeader.Height())
	assert.Equal(t, "full-chain", convertedHeader.ChainID())
	assert.Equal(t, fullHeader.DataHash, convertedHeader.DataCommitment)
	assert.Equal(t, fullHeader.AppHash, convertedHeader.StateRoot)

	// Test conversion to standard header
	convertedFullHeader := minHeader.ToHeader()
	assert.Equal(t, minHeader.Height(), convertedFullHeader.Height())
	assert.Equal(t, minHeader.ChainID(), convertedFullHeader.ChainID())
	assert.Equal(t, minHeader.DataCommitment, convertedFullHeader.DataHash)
	assert.Equal(t, minHeader.StateRoot, convertedFullHeader.AppHash)

	// Test validation
	err := minHeader.ValidateBasic()
	assert.NoError(t, err)

	// Test validation with invalid header
	invalidHeader := &MinimalHeader{
		BlockHeight:  0, // Invalid height
		BlockChainID: "test-chain",
	}
	err = invalidHeader.ValidateBasic()
	assert.Error(t, err)
}
