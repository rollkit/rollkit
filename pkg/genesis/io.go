package genesis

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

var ErrGenesisExists = fmt.Errorf("genesis file already exists")

// CreateGenesis creates and saves a genesis file with the given app state.
// If the genesis file already exists, it skips the creation and returns ErrGenesisExists.
// The genesis file is saved in the config directory of the specified home path.
func CreateGenesis(homePath string, chainID string, initialHeight uint64, proposerAddress []byte) error {
	configDir := filepath.Join(homePath, "config")
	genesisPath := filepath.Join(configDir, "genesis.json")

	// Check if the genesis file already exists
	if _, err := os.Stat(genesisPath); err == nil {
		return ErrGenesisExists
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to check for existing genesis file at %s: %w", genesisPath, err)
	}

	// If the directory doesn't exist, create it
	if err := os.MkdirAll(configDir, 0o750); err != nil {
		return fmt.Errorf("error creating config directory: %w", err)
	}

	genesisData := NewGenesis(
		chainID,
		initialHeight,
		time.Now(),      // Current time as genesis DA start height
		proposerAddress, // Proposer address
	)

	if err := genesisData.Save(genesisPath); err != nil {
		return fmt.Errorf("error writing genesis file: %w", err)
	}

	return nil
}

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
