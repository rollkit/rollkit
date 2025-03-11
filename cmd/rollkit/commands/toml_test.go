package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	rollconf "github.com/rollkit/rollkit/config"
)

func TestInitTomlCommand(t *testing.T) {
	// Create a temporary directory for testing
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)

	// Save current directory to restore it later
	originalDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() {
		require.NoError(t, os.Chdir(originalDir))
	}()

	// Change to the temporary directory
	require.NoError(t, os.Chdir(dir))

	// Execute the init command
	cmd := NewTomlCmd()
	cmd.SetArgs([]string{"init"})
	err = cmd.Execute()
	require.NoError(t, err)

	// Verify the file was created
	configPath := filepath.Join(dir, rollconf.RollkitConfigToml)
	_, err = os.Stat(configPath)
	require.NoError(t, err)

	// Read the config back from the file
	readConfig, err := rollconf.ReadToml()
	require.NoError(t, err)

	// Read the file content directly to verify the TOML structure
	content, err := os.ReadFile(configPath) //nolint:gosec // This is a test file with a controlled path
	require.NoError(t, err)
	tomlContent := string(content)

	// Verify that the TOML file content contains the expected values
	// Group verifications by category

	// Verify time values
	require.Contains(t, tomlContent, "block_time = ")
	require.Contains(t, tomlContent, "1s")
	require.Contains(t, tomlContent, "[da]")
	require.Contains(t, tomlContent, "block_time = ")
	require.Contains(t, tomlContent, "15s")
	require.Contains(t, tomlContent, "lazy_block_time = ")
	require.Contains(t, tomlContent, "1m0s")

	// Verify addresses
	require.Contains(t, tomlContent, "address = ")
	require.Contains(t, tomlContent, "http://localhost:26658")
	require.Contains(t, tomlContent, "sequencer_address = ")
	require.Contains(t, tomlContent, "localhost:50051")
	require.Contains(t, tomlContent, "executor_address = ")
	require.Contains(t, tomlContent, "localhost:40041")

	// Verify other values
	require.Contains(t, tomlContent, "sequencer_rollup_id = ")
	require.Contains(t, tomlContent, "mock-rollup")

	// Verify boolean values
	require.Contains(t, tomlContent, "aggregator = false")
	require.Contains(t, tomlContent, "light = false")
	require.Contains(t, tomlContent, "lazy_aggregator = false")

	// Verify the root directory
	require.Equal(t, dir, readConfig.RootDir)

	// Verify that the entire Rollkit configuration matches the default values
	// This verification is sufficient to cover all individual fields
	require.Equal(t, rollconf.DefaultNodeConfig.Node, readConfig.Node)
}
