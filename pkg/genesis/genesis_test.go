package genesis

import (
	"encoding/json"
	"testing"
	"time"
)

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
				AppState:             json.RawMessage(`{}`),
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
