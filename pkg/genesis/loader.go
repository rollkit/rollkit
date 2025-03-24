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

	if genesis.ChainID == "" {
		return Genesis{}, fmt.Errorf("invalid or missing chain_id in genesis file")
	}

	return genesis, nil
}
