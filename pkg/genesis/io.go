package genesis

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// LoadGenesis loads the genesis state from the specified file path
func LoadGenesis(genesisPath string) (Genesis, error) {
	// Validate and clean the file path
	cleanPath := filepath.Clean(genesisPath)
	if _, err := os.Stat(cleanPath); os.IsNotExist(err) {
		return Genesis{}, fmt.Errorf("genesis file not found at path: %s", cleanPath)
	}

	genesisJSON, err := os.ReadFile(cleanPath)
	if err != nil {
		return Genesis{}, fmt.Errorf("failed to read genesis file: %w", err)
	}

	var genesis Genesis
	err = json.Unmarshal(genesisJSON, &genesis)
	if err != nil {
		return Genesis{}, fmt.Errorf("invalid genesis file: %w", err)
	}

	if err := genesis.Validate(); err != nil {
		return Genesis{}, err
	}

	return genesis, nil
}

// SaveGenesis saves the genesis state to the specified file path
func SaveGenesis(genesis Genesis, genesisPath string) error {
	// Validate and clean the file path
	cleanPath := filepath.Clean(genesisPath)

	genesisJSON, err := json.MarshalIndent(genesis, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal genesis state: %w", err)
	}

	err = os.WriteFile(cleanPath, genesisJSON, 0600)
	if err != nil {
		return fmt.Errorf("failed to write genesis file: %w", err)
	}

	return nil
}
