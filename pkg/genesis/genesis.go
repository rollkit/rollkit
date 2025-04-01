package genesis

import (
	"encoding/json"
	"fmt"
	"time"
)

// Genesis represents the genesis state of the blockchain
type Genesis struct {
	ChainID              string          `json:"chain_id"`
	GenesisDAStartHeight time.Time       `json:"genesis_da_start_height"` // TODO: change to uint64 and remove time.Time, basically we need a mechanism to convert DAHeight to time.Time
	InitialHeight        uint64          `json:"initial_height"`
	ProposerAddress      []byte          `json:"proposer_address"`
	AppState             json.RawMessage `json:"app_state,omitempty"`
}

// NewGenesis creates a new Genesis instance
func NewGenesis(
	chainID string,
	initialHeight uint64,
	genesisDAStartHeight time.Time,
	proposerAddress []byte,
	appState json.RawMessage,
) Genesis {
	genesis := Genesis{
		ChainID:              chainID,
		GenesisDAStartHeight: genesisDAStartHeight,
		InitialHeight:        initialHeight,
		ProposerAddress:      proposerAddress,
		AppState:             appState,
	}
	genesis.Validate()

	return genesis
}

// Validate checks if the Genesis object is valid
func (g Genesis) Validate() error {
	if g.ChainID == "" {
		return fmt.Errorf("invalid or missing chain_id in genesis file")
	}

	// Check initial height
	if g.InitialHeight < 1 {
		return fmt.Errorf("initial_height must be at least 1, got %d", g.InitialHeight)
	}

	// Check DA start height is not zero time
	if g.GenesisDAStartHeight.IsZero() {
		return fmt.Errorf("genesis_da_start_height cannot be zero time")
	}

	return nil
}
