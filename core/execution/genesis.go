package execution

import (
	"time"
)

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
	proposerAddress      []byte
	rawBytes             []byte
}

// NewGenesis creates a new Genesis instance
func NewGenesis(
	chainID string,
	initialHeight uint64,
	genesisDAStartHeight time.Time,
	proposerAddress []byte,
	rawBytes []byte,
) Genesis {
	return Genesis{
		ChainID:              chainID,
		GenesisDAStartHeight: genesisDAStartHeight,
		InitialHeight:        initialHeight,
		proposerAddress:      proposerAddress,
		rawBytes:             rawBytes,
	}
}

// ProposerAddress returns the address of the proposer
func (g Genesis) ProposerAddress() []byte {
	return g.proposerAddress
}

// Bytes returns the genesis state as bytes
func (g Genesis) Bytes() []byte {
	return g.rawBytes
}
