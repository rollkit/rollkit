package execution

import (
	"time"
)

// Genesis defines the interface for genesis state
type Genesis interface {
	// ChainID returns the chain identifier
	ChainID() string

	// InitialHeight returns the initial block height
	InitialHeight() uint64

	// GenesisTime returns the genesis time
	GenesisTime() time.Time

	// ProposerAddress returns the address of the proposer
	ProposerAddress() []byte

	// Bytes returns the genesis state as bytes
	Bytes() []byte
}

// BaseGenesis provides a basic implementation of the Genesis interface
type BaseGenesis struct {
	chainID         string
	initialHeight   uint64
	genesisTime     time.Time
	proposerAddress []byte
	rawBytes        []byte
}

// NewBaseGenesis creates a new BaseGenesis instance
func NewBaseGenesis(
	chainID string,
	initialHeight uint64,
	genesisTime time.Time,
	proposerAddress []byte,
	rawBytes []byte,
) *BaseGenesis {
	return &BaseGenesis{
		chainID:         chainID,
		initialHeight:   initialHeight,
		genesisTime:     genesisTime,
		proposerAddress: proposerAddress,
		rawBytes:        rawBytes,
	}
}

// ChainID implements Genesis interface
func (bg *BaseGenesis) ChainID() string {
	return bg.chainID
}

// InitialHeight implements Genesis interface
func (bg *BaseGenesis) InitialHeight() uint64 {
	return bg.initialHeight
}

// GenesisTime implements Genesis interface
func (bg *BaseGenesis) GenesisTime() time.Time {
	return bg.genesisTime
}

// ProposerAddress implements Genesis interface
func (bg *BaseGenesis) ProposerAddress() []byte {
	return bg.proposerAddress
}

// Bytes implements Genesis interface
func (bg *BaseGenesis) Bytes() []byte {
	return bg.rawBytes
}
