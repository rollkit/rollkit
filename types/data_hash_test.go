package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDataHashMethods(t *testing.T) {
	// Create sample data
	data := &Data{
		Txs: Txs{[]byte("test transaction")},
	}

	// Create a header
	header := &Header{}

	// Set the data commitment
	header.SetDataCommitment(data)

	// Verify that the data hash was correctly set
	assert.Equal(t, data.Hash(), header.DataHash, "DataHash should be set correctly")

	// Validate the data commitment
	assert.True(t, header.ValidateDataCommitment(data), "ValidateDataCommitment should return true for matching data")

	// Modify data and verify that validation fails
	modifiedData := &Data{
		Txs: Txs{[]byte("different transaction")},
	}
	assert.False(t, header.ValidateDataCommitment(modifiedData), "ValidateDataCommitment should return false for non-matching data")
}
