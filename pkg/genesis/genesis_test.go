package genesis

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewGenesis(t *testing.T) {
	// Test valid genesis creation
	validTime := time.Now()
	proposerAddress := []byte("proposer")

	genesis := NewGenesis(
		"test-chain",
		1,
		validTime,
		proposerAddress,
	)

	assert.Equal(t, "test-chain", genesis.ChainID)
	assert.Equal(t, uint64(1), genesis.InitialHeight)
	assert.Equal(t, validTime, genesis.GenesisDAStartHeight)
	assert.Equal(t, proposerAddress, genesis.ProposerAddress)

	// Test that NewGenesis validates and panics on invalid input
	genesis = NewGenesis(
		"", // Empty chain ID should cause panic
		1,
		validTime,
		proposerAddress,
	)
	err := genesis.Validate()
	assert.Error(t, err)

	genesis = NewGenesis(
		"test-chain",
		0, // Zero initial height should cause panic
		validTime,
		proposerAddress,
	)
	err = genesis.Validate()
	assert.Error(t, err)

	genesis = NewGenesis(
		"test-chain",
		1,
		time.Time{}, // Zero time should cause panic
		proposerAddress,
	)
	err = genesis.Validate()
	assert.Error(t, err)

	genesis = NewGenesis(
		"test-chain",
		1,
		validTime,
		nil, // Nil proposer address should cause panic
	)
	err = genesis.Validate()
	assert.Error(t, err)
}

func TestGenesis_Validate(t *testing.T) {
	validTime := time.Now()
	tests := []struct {
		name    string
		genesis Genesis
		wantErr bool
	}{
		{
			name: "valid genesis - chain ID can contain any character",
			genesis: Genesis{
				ChainID:              "test@chain#123!",
				GenesisDAStartHeight: validTime,
				InitialHeight:        1,
				ProposerAddress:      []byte("proposer"),
			},
			wantErr: false,
		},
		{
			name: "invalid - empty chain_id",
			genesis: Genesis{
				ChainID:              "",
				GenesisDAStartHeight: validTime,
				InitialHeight:        1,
				ProposerAddress:      []byte("proposer"),
			},
			wantErr: true,
		},
		{
			name: "invalid - zero initial height",
			genesis: Genesis{
				ChainID:              "test-chain",
				GenesisDAStartHeight: validTime,
				InitialHeight:        0,
				ProposerAddress:      []byte("proposer"),
			},
			wantErr: true,
		},
		{
			name: "invalid - zero time DA start height",
			genesis: Genesis{
				ChainID:              "test-chain",
				GenesisDAStartHeight: time.Time{},
				InitialHeight:        1,
				ProposerAddress:      []byte("proposer"),
			},
			wantErr: true,
		},
		{
			name: "invalid - nil proposer address",
			genesis: Genesis{
				ChainID:              "test-chain",
				GenesisDAStartHeight: validTime,
				InitialHeight:        1,
				ProposerAddress:      nil,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.genesis.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Genesis.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
