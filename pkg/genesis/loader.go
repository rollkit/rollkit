package genesis

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rollkit/rollkit/core/execution"
)

// DefaultGenesisLoader implements execution.GenesisLoader interface
type DefaultGenesisLoader struct{}

// LoadGenesis implements execution.GenesisLoader.LoadGenesis
func (l *DefaultGenesisLoader) LoadGenesis(rootDir string, configDir string) (execution.Genesis, error) {
	genesisPath := filepath.Join(rootDir, configDir, "genesis.json")

	if _, err := os.Stat(genesisPath); os.IsNotExist(err) {
		return execution.Genesis{}, fmt.Errorf("genesis file not found at path: %s", genesisPath)
	}

	genesisBytes, err := os.ReadFile(genesisPath)
	if err != nil {
		return execution.Genesis{}, fmt.Errorf("failed to read genesis file: %w", err)
	}

	// Validate that the genesis file is valid JSON
	var genDoc map[string]interface{}
	if err := json.Unmarshal(genesisBytes, &genDoc); err != nil {
		return execution.Genesis{}, fmt.Errorf("invalid genesis file: %w", err)
	}

	// Extract required fields from genDoc
	chainID, ok := genDoc["chain_id"].(string)
	if !ok {
		return execution.Genesis{}, fmt.Errorf("invalid or missing chain_id in genesis file")
	}

	var initialHeight uint64 = 1 // Default value
	if height, ok := genDoc["initial_height"].(float64); ok {
		initialHeight = uint64(height)
	}

	// Parse genesis time
	genesisTime := time.Now().UTC() // Default to current time
	if timeStr, ok := genDoc["genesis_time"].(string); ok {
		parsedTime, err := time.Parse(time.RFC3339, timeStr)
		if err != nil {
			return execution.Genesis{}, fmt.Errorf("invalid genesis_time format: %w", err)
		}
		genesisTime = parsedTime
	}

	// Create and return a new Genesis
	return execution.NewGenesis(
		chainID,
		initialHeight,
		genesisTime,
		nil, // No proposer address for now
		genesisBytes,
	), nil
}
