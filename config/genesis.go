package config

import (
	"encoding/json"
	"os"
	"time"
)

// GenesisDoc representa el documento génesis con métodos para acceder a sus campos
type GenesisDoc interface {
	GetChainID() string
	GetInitialHeight() uint64
	GetGenesisTime() time.Time
	GetProposerAddress() []byte
}

func NewGenesisDoc(chainID string, initialHeight uint64, genesisTime time.Time, proposerAddress []byte) GenesisDoc {
	return &genesisDoc{
		ChainID:         chainID,
		InitialHeight:   initialHeight,
		GenesisTime:     genesisTime,
		ProposerAddress: proposerAddress,
	}
}

type genesisDoc struct {
	ChainID         string    `json:"chain_id"`
	InitialHeight   uint64    `json:"initial_height"`
	GenesisTime     time.Time `json:"genesis_time"`
	ProposerAddress []byte    `json:"proposer_address"`
}

func (g *genesisDoc) GetChainID() string {
	return g.ChainID
}

func (g *genesisDoc) GetInitialHeight() uint64 {
	return g.InitialHeight
}

func (g *genesisDoc) GetGenesisTime() time.Time {
	return g.GenesisTime
}

func (g *genesisDoc) GetProposerAddress() []byte {
	return g.ProposerAddress
}

// LoadGenesisDoc carga un documento génesis desde un archivo JSON
func LoadGenesisDoc(filename string) (GenesisDoc, error) {
	bytes, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	doc := &genesisDoc{}
	if err := json.Unmarshal(bytes, doc); err != nil {
		return nil, err
	}

	return doc, nil
}
