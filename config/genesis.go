package config

import (
	"encoding/json"
	"os"
	"time"
)

// GenesisDoc represents the genesis document with methods to access its fields
type GenesisDoc struct {
	ChainID         string    `json:"chain_id"`
	InitialHeight   uint64    `json:"initial_height"`
	GenesisTime     time.Time `json:"genesis_time"`
	ProposerAddress []byte    `json:"proposer_address"`
}

// NewGenesisDoc creates a new genesis document
func NewGenesisDoc(chainID string, initialHeight uint64, genesisTime time.Time, proposerAddress []byte) *GenesisDoc {
	return &GenesisDoc{
		ChainID:         chainID,
		InitialHeight:   initialHeight,
		GenesisTime:     genesisTime,
		ProposerAddress: proposerAddress,
	}
}

// GetChainID returns the chain ID
func (g *GenesisDoc) GetChainID() string {
	return g.ChainID
}

// GetInitialHeight returns the initial height
func (g *GenesisDoc) GetInitialHeight() uint64 {
	return g.InitialHeight
}

// GetGenesisTime returns the genesis time
func (g *GenesisDoc) GetGenesisTime() time.Time {
	return g.GenesisTime
}

// GetProposerAddress returns the proposer address
func (g *GenesisDoc) GetProposerAddress() []byte {
	return g.ProposerAddress
}

// LoadGenesisDoc loads a genesis document from a JSON file
func LoadGenesisDoc(filename string) (*GenesisDoc, error) {
	bytes, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	doc := &GenesisDoc{}
	if err := json.Unmarshal(bytes, doc); err != nil {
		return nil, err
	}

	return doc, nil
}
