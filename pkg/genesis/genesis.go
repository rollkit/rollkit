package genesis

import (
	"encoding/json"
	"fmt"
	"time"
)

// GenesisExtraData represents the flexible configuration data in genesis
type GenesisExtraData struct {
	ProposerAddress []byte            `json:"proposer_address,omitempty"`
	Validators      []ValidatorInfo   `json:"validators,omitempty"`
	ConsensusParams map[string]string `json:"consensus_params,omitempty"`
}

// ValidatorInfo represents a validator's configuration in genesis
type ValidatorInfo struct {
	Address   []byte `json:"address"`
	PublicKey []byte `json:"public_key"`
}

// Genesis represents the genesis state of the blockchain
type Genesis struct {
	ChainID              string           `json:"chain_id"`
	GenesisDAStartHeight time.Time        `json:"genesis_da_start_height"` // TODO: change to uint64 and remove time.Time, basically we need a mechanism to convert DAHeight to time.Time
	InitialHeight        uint64           `json:"initial_height"`
	ExtraData            GenesisExtraData `json:"extra_data"`
	AppState             json.RawMessage  `json:"app_state,omitempty"`
}

// NewGenesis creates a new Genesis instance
func NewGenesis(
	chainID string,
	initialHeight uint64,
	genesisDAStartHeight time.Time,
	extraData GenesisExtraData,
	appState json.RawMessage,
) Genesis {
	return Genesis{
		ChainID:              chainID,
		GenesisDAStartHeight: genesisDAStartHeight,
		InitialHeight:        initialHeight,
		ExtraData:            extraData,
		AppState:             appState,
	}
}

// ProposerAddress returns the address of the proposer (for backwards compatibility)
func (g Genesis) ProposerAddress() []byte {
	return g.ExtraData.ProposerAddress
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
