package execution

import (
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
	Address     []byte `json:"address"`
	PublicKey   []byte `json:"public_key"`
	VotingPower int64  `json:"voting_power"`
}

// GenesisLoader defines the interface for loading genesis data
type GenesisLoader interface {
	// LoadGenesis loads the genesis data from a file and returns a Genesis object
	LoadGenesis(rootDir string, configDir string) (Genesis, error)
}

// Genesis represents the genesis state of the blockchain
type Genesis struct {
	ChainID              string
	GenesisDAStartHeight time.Time
	InitialHeight        uint64
	extraData            GenesisExtraData
	rawBytes             []byte
}

// NewGenesis creates a new Genesis instance
func NewGenesis(
	chainID string,
	initialHeight uint64,
	genesisDAStartHeight time.Time,
	extraData GenesisExtraData,
	rawBytes []byte,
) Genesis {
	return Genesis{
		ChainID:              chainID,
		GenesisDAStartHeight: genesisDAStartHeight,
		InitialHeight:        initialHeight,
		extraData:            extraData,
		rawBytes:             rawBytes,
	}
}

// ProposerAddress returns the address of the proposer (for backwards compatibility)
func (g Genesis) ProposerAddress() []byte {
	return g.extraData.ProposerAddress
}

// Bytes returns the genesis state as bytes
func (g Genesis) Bytes() []byte {
	return g.rawBytes
}
