package genesis

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// DefaultGenesisLoader implements GenesisLoader interface
type DefaultGenesisLoader struct{}

// LoadGenesis implements GenesisLoader.LoadGenesis
func (l *DefaultGenesisLoader) LoadGenesis(rootDir string, configDir string) (Genesis, error) {
	genesisPath := filepath.Join(rootDir, configDir, "genesis.json")

	if _, err := os.Stat(genesisPath); os.IsNotExist(err) {
		return Genesis{}, fmt.Errorf("genesis file not found at path: %s", genesisPath)
	}

	genesisJSON, err := os.ReadFile(genesisPath)
	if err != nil {
		return Genesis{}, fmt.Errorf("failed to read genesis file: %w", err)
	}

	var genesis Genesis
	err = json.Unmarshal(genesisJSON, &genesis)
	if err != nil {
		return Genesis{}, fmt.Errorf("invalid genesis file: %w", err)
	}

	if genesis.ChainID == "" {
		return Genesis{}, fmt.Errorf("invalid or missing chain_id in genesis file")
	}

	return genesis, nil
}

// NewDefaultGenesisLoader creates a new instance of DefaultGenesisLoader
func NewDefaultGenesisLoader() *DefaultGenesisLoader {
	return &DefaultGenesisLoader{}
}

// GenesisLoader defines the interface for loading genesis state
type GenesisLoader interface {
	LoadGenesis(rootDir string, configDir string) (Genesis, error)
}
