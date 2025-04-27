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
				ChainID:            "test-chain-1",
				InitialHeight:      1,
				GenesisDAStartTime: validTime,
				ProposerAddress:    []byte("proposer-address"),
			},
			wantErr: false,
		},
		{
			name: "valid genesis - minimal",
			genesis: Genesis{
				ChainID:            "test-chain-2",
				InitialHeight:      1,
				GenesisDAStartTime: validTime,
				ProposerAddress:    []byte("proposer-address"),
			},
			wantErr: false,
		},
		{
			name: "invalid genesis - empty chain ID",
			genesis: Genesis{
				ChainID:            "",
				InitialHeight:      1,
				GenesisDAStartTime: validTime,
				ProposerAddress:    []byte("proposer-address"),
			},
			wantErr: true,
		},
		{
			name: "invalid genesis - zero initial height",
			genesis: Genesis{
				ChainID:            "test-chain",
				InitialHeight:      0,
				GenesisDAStartTime: validTime,
				ProposerAddress:    []byte("proposer-address"),
			},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a temporary file path for each test
			tmpFile := filepath.Join(tmpDir, "genesis.json")

			// Test SaveGenesis
			err := tc.genesis.Save(tmpFile)
			require.NoError(t, err)
			err = tc.genesis.Validate()
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			// Test LoadGenesis
			loaded, err := LoadGenesis(tmpFile)
			assert.NoError(t, err)

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
	err := os.WriteFile(filepath.Clean(tmpFile), []byte("{invalid json}"), 0o600)
	require.NoError(t, err)

	_, err = LoadGenesis(tmpFile)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid genesis file")
}

func TestLoadGenesis_ReadError(t *testing.T) {
	// Create a temporary directory
	tmpDir := t.TempDir()

	// Create a directory with the same name as our intended file
	// This will cause ReadFile to fail with "is a directory" error
	tmpFilePath := filepath.Join(tmpDir, "genesis.json")
	err := os.Mkdir(tmpFilePath, 0o755)
	require.NoError(t, err)

	// Try to load from a path that is actually a directory
	_, err = LoadGenesis(tmpFilePath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read genesis file")
}

func TestLoadGenesis_InvalidGenesisContent(t *testing.T) {
	// Create a temporary file with valid JSON but invalid Genesis content
	tmpFile := filepath.Join(t.TempDir(), "valid_json_invalid_genesis.json")
	// This is valid JSON but doesn't have required Genesis fields
	err := os.WriteFile(filepath.Clean(tmpFile), []byte(`{"foo": "bar"}`), 0o600)
	require.NoError(t, err)

	_, err = LoadGenesis(tmpFile)
	assert.Error(t, err)
	// This should fail validation since required fields are missing
	assert.Contains(t, err.Error(), "invalid or missing chain_id")
}

func TestSaveGenesis_InvalidPath(t *testing.T) {
	// Test different invalid paths for better coverage
	testCases := []struct {
		name       string
		path       string
		createPath bool
	}{
		{
			name: "nonexistent directory",
			path: "/nonexistent/directory/genesis.json",
		},
		{
			name:       "invalid directory permissions",
			path:       filepath.Join(t.TempDir(), "readonly", "genesis.json"),
			createPath: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.createPath {
				// Create a read-only directory if needed
				dirPath := filepath.Dir(tc.path)
				err := os.MkdirAll(dirPath, 0o500) // read-only directory
				if err != nil {
					t.Skip("Failed to create directory with specific permissions, skipping:", err)
				}
			}

			genesis := Genesis{
				ChainID:            "test-chain",
				InitialHeight:      1,
				GenesisDAStartTime: time.Now().UTC(),
				ProposerAddress:    []byte("proposer-address"),
			}

			err := genesis.Save(tc.path)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "failed to write genesis file")
		})
	}
}
