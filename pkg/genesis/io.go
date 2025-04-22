package genesis

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// LoadGenesis loads the genesis state from the specified file path.
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
	if err := json.Unmarshal(genesisJSON, &genesis); err != nil {
		return Genesis{}, fmt.Errorf("invalid genesis file: %w", err)
	}

	if err := genesis.Validate(); err != nil {
		return Genesis{}, err
	}

	return genesis, nil
}

// Save saves the genesis state to the specified file path.
// It should only be used when the application is NOT handling the genesis creation.
func (g Genesis) Save(genesisPath string) error {
	genesisJSON, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal genesis state: %w", err)
	}

	err = os.WriteFile(filepath.Clean(genesisPath), genesisJSON, 0o600)
	if err != nil {
		return fmt.Errorf("failed to write genesis file: %w", err)
	}

	return nil
}
