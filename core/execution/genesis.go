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
	chainID         string
	initialHeight   uint64
	genesisTime     time.Time
	proposerAddress []byte
	rawBytes        []byte
}

// NewGenesis creates a new Genesis instance
func NewGenesis(
	chainID string,
	initialHeight uint64,
	genesisTime time.Time,
	proposerAddress []byte,
	rawBytes []byte,
) Genesis {
	return Genesis{
		chainID:         chainID,
		initialHeight:   initialHeight,
		genesisTime:     genesisTime,
		proposerAddress: proposerAddress,
		rawBytes:        rawBytes,
	}
}

// ChainID returns the chain identifier
func (g Genesis) ChainID() string {
	return g.chainID
}

// InitialHeight returns the initial block height
func (g Genesis) InitialHeight() uint64 {
	return g.initialHeight
}

// GenesisTime returns the genesis time
func (g Genesis) GenesisTime() time.Time {
	return g.genesisTime
}

// ProposerAddress returns the address of the proposer
func (g Genesis) ProposerAddress() []byte {
	return g.proposerAddress
}

// Bytes returns the genesis state as bytes
func (g Genesis) Bytes() []byte {
	return g.rawBytes
}
