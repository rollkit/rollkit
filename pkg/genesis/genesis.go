package genesis

import (
	"fmt"
	"time"
)

// Genesis represents the genesis state of the blockchain.
// This genesis struct only contains the fields required by rollkit.
// The app state or other fields are not included here.
type Genesis struct {
	ChainID            string    `json:"chain_id"`
	GenesisDAStartTime time.Time `json:"genesis_da_start_height"` // TODO: change to uint64 and remove time.Time, basically we need a mechanism to convert DAHeight to time.Time
	InitialHeight      uint64    `json:"initial_height"`
	ProposerAddress    []byte    `json:"proposer_address"`
}

// NewGenesis creates a new Genesis instance.
func NewGenesis(
	chainID string,
	initialHeight uint64,
	genesisDAStartHeight time.Time,
	proposerAddress []byte,
) Genesis {
	genesis := Genesis{
		ChainID:            chainID,
		GenesisDAStartTime: genesisDAStartHeight,
		InitialHeight:      initialHeight,
		ProposerAddress:    proposerAddress,
	}

	return genesis
}

// Validate checks if the Genesis object is valid.
func (g Genesis) Validate() error {
	if g.ChainID == "" {
		return fmt.Errorf("invalid or missing chain_id in genesis file")
	}

	// Check initial height
	if g.InitialHeight < 1 {
		return fmt.Errorf("initial_height must be at least 1, got %d", g.InitialHeight)
	}

	// Check DA start height is not zero time
	if g.GenesisDAStartTime.IsZero() {
		return fmt.Errorf("genesis_da_start_height cannot be zero time")
	}

	if g.ProposerAddress == nil {
		return fmt.Errorf("proposer_address cannot be nil")
	}

	return nil
}
