package types

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evstack/ev-node/pkg/genesis"
)

// TestNewFromGenesisDoc tests the creation of State from a Genesis document.
func TestNewFromGenesisDoc(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	// Case 1: Valid Genesis Document
	genTime := time.Now()
	genDoc := genesis.Genesis{
		ChainID:            "test-chain",
		InitialHeight:      10,
		GenesisDAStartTime: genTime,
	}

	state, err := NewFromGenesisDoc(genDoc)
	require.NoError(err)

	assert.Equal(InitStateVersion, state.Version)
	assert.Equal(genDoc.ChainID, state.ChainID)
	assert.Equal(genDoc.InitialHeight, state.InitialHeight)
	assert.Equal(uint64(1), state.DAHeight, "Initial DAHeight should be 1")
	assert.Equal(genDoc.InitialHeight-1, state.LastBlockHeight)
	assert.Equal(genDoc.GenesisDAStartTime, state.LastBlockTime)
	assert.Nil(state.LastResultsHash)
	assert.Nil(state.AppHash)

	// Case 2: Invalid Genesis Document (InitialHeight = 0)
	invalidGenDoc := genesis.Genesis{
		ChainID:            "invalid-chain",
		InitialHeight:      0,
		GenesisDAStartTime: genTime,
	}

	_, err = NewFromGenesisDoc(invalidGenDoc)
	require.Error(err)
	assert.Contains(err.Error(), "initial height must be 1 when starting a new app")
}
