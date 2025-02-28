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
}

type genesisDoc struct {
	ChainID       string    `json:"chain_id"`
	InitialHeight uint64    `json:"initial_height"`
	GenesisTime   time.Time `json:"genesis_time"`
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
