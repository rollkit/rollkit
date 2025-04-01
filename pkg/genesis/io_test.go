package genesis

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadAndSaveGenesis(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir, err := os.MkdirTemp("", "genesis-test-*")
	require.NoError(t, err)
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Errorf("failed to remove temporary directory: %v", err)
		}
	}()

	validTime := time.Now().UTC()
	testCases := []struct {
		name    string
		genesis Genesis
		wantErr bool
	}{
		{
			name: "valid genesis",
			genesis: Genesis{
				ChainID:              "test-chain-1",
				InitialHeight:        1,
				GenesisDAStartHeight: validTime,
				ProposerAddress:      []byte("proposer-address"),
				AppState:             json.RawMessage(`{"key": "value"}`),
			},
			wantErr: false,
		},
		{
			name: "valid genesis - minimal",
			genesis: Genesis{
				ChainID:              "test-chain-2",
				InitialHeight:        1,
				GenesisDAStartHeight: validTime,
				ProposerAddress:      []byte("proposer-address"),
				AppState:             json.RawMessage(`{}`),
			},
			wantErr: false,
		},
		{
			name: "invalid genesis - empty chain ID",
			genesis: Genesis{
				ChainID:              "",
				InitialHeight:        1,
				GenesisDAStartHeight: validTime,
				ProposerAddress:      []byte("proposer-address"),
			},
			wantErr: true,
		},
		{
			name: "invalid genesis - zero initial height",
			genesis: Genesis{
				ChainID:              "test-chain",
				InitialHeight:        0,
				GenesisDAStartHeight: validTime,
				ProposerAddress:      []byte("proposer-address"),
			},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a temporary file path for each test
			tmpFile := filepath.Join(tmpDir, "genesis.json")

			// Test SaveGenesis
			err := SaveGenesis(tc.genesis, tmpFile)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			// Test LoadGenesis
			loaded, err := LoadGenesis(tmpFile)
			assert.NoError(t, err)

			// Compare AppState as JSON objects instead of raw bytes
			if len(tc.genesis.AppState) > 0 {
				var expectedAppState, actualAppState interface{}
				err = json.Unmarshal(tc.genesis.AppState, &expectedAppState)
				assert.NoError(t, err)
				err = json.Unmarshal(loaded.AppState, &actualAppState)
				assert.NoError(t, err)
				assert.Equal(t, expectedAppState, actualAppState, "AppState contents should match")

				// Set AppState to nil for the remaining comparison
				tc.genesis.AppState = nil
				loaded.AppState = nil
			}

			// Compare the rest of the fields
			assert.Equal(t, tc.genesis, loaded)

			// Verify file contents are valid JSON
			fileContent, err := os.ReadFile(filepath.Clean(tmpFile))
			assert.NoError(t, err)
			var jsonContent map[string]interface{}
			err = json.Unmarshal(fileContent, &jsonContent)
			assert.NoError(t, err)
		})
	}
}

func TestLoadGenesis_FileNotFound(t *testing.T) {
	_, err := LoadGenesis("nonexistent.json")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "genesis file not found")
}

func TestLoadGenesis_InvalidJSON(t *testing.T) {
	// Create a temporary file with invalid JSON
	tmpFile := filepath.Join(t.TempDir(), "invalid.json")
	err := os.WriteFile(filepath.Clean(tmpFile), []byte("{invalid json}"), 0600)
	require.NoError(t, err)

	_, err = LoadGenesis(tmpFile)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid genesis file")
}

func TestSaveGenesis_InvalidPath(t *testing.T) {
	genesis := Genesis{
		ChainID:              "test-chain",
		InitialHeight:        1,
		GenesisDAStartHeight: time.Now().UTC(),
		ProposerAddress:      []byte("proposer-address"),
	}
	err := SaveGenesis(genesis, "/nonexistent/directory/genesis.json")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to write genesis file")
}
