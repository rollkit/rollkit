package genesis

import (
	"encoding/json"
	"fmt"
	"os"
)

// LoadGenesis loads the genesis state from the specified file path
func LoadGenesis(genesisPath string) (Genesis, error) {
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

	if err := genesis.Validate(); err != nil {
		return Genesis{}, err
	}

	return genesis, nil
}

// SaveGenesis saves the genesis state to the specified file path
func SaveGenesis(genesis Genesis, genesisPath string) error {
	if err := genesis.Validate(); err != nil {
		return fmt.Errorf("invalid genesis state: %w", err)
	}

	genesisJSON, err := json.MarshalIndent(genesis, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal genesis state: %w", err)
	}

	err = os.WriteFile(genesisPath, genesisJSON, 0644)
	if err != nil {
		return fmt.Errorf("failed to write genesis file: %w", err)
	}

	return nil
}
