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
	configPath := filepath.Join(dir, rollconf.RollkitToml)
	_, err = os.Stat(configPath)
	require.NoError(t, err)

	// Read the config back from the file
	readConfig, err := rollconf.ReadToml()
	require.NoError(t, err)

	// Read the file content directly to verify the TOML structure
	content, err := os.ReadFile(configPath) //nolint:gosec // This is a test file with a controlled path
	require.NoError(t, err)

	// Check that the content contains the expected default values
	tomlContent := string(content)

	// Verify specific default values in the TOML file
	// Accept both single and double quotes for string values
	require.Contains(t, tomlContent, "block_time = ")
	require.Contains(t, tomlContent, "1s")
	require.Contains(t, tomlContent, "da_address = ")
	require.Contains(t, tomlContent, "http://localhost:26658")
	require.Contains(t, tomlContent, "sequencer_address = ")
	require.Contains(t, tomlContent, "localhost:50051")
	require.Contains(t, tomlContent, "sequencer_rollup_id = ")
	require.Contains(t, tomlContent, "mock-rollup")
	require.Contains(t, tomlContent, "da_block_time = ")
	require.Contains(t, tomlContent, "15s")
	require.Contains(t, tomlContent, "lazy_block_time = ")
	require.Contains(t, tomlContent, "1m0s")
	require.Contains(t, tomlContent, "executor_address = ")
	require.Contains(t, tomlContent, "localhost:40041")

	// Verify default boolean values with exact values
	require.Contains(t, tomlContent, "aggregator = false")
	require.Contains(t, tomlContent, "light = false")
	require.Contains(t, tomlContent, "lazy_aggregator = false")

	// Verify the root directory is set correctly
	require.Equal(t, dir, readConfig.RootDir)

	// Verify Rollkit config values match the defaults by comparing specific fields
	require.Equal(t, rollconf.DefaultNodeConfig.Rollkit.BlockTime, readConfig.Rollkit.BlockTime)
	require.Equal(t, rollconf.DefaultNodeConfig.Rollkit.DABlockTime, readConfig.Rollkit.DABlockTime)
	require.Equal(t, rollconf.DefaultNodeConfig.Rollkit.LazyBlockTime, readConfig.Rollkit.LazyBlockTime)
	require.Equal(t, rollconf.DefaultNodeConfig.Rollkit.DAAddress, readConfig.Rollkit.DAAddress)
	require.Equal(t, rollconf.DefaultNodeConfig.Rollkit.SequencerAddress, readConfig.Rollkit.SequencerAddress)
	require.Equal(t, rollconf.DefaultNodeConfig.Rollkit.SequencerRollupID, readConfig.Rollkit.SequencerRollupID)
	require.Equal(t, rollconf.DefaultNodeConfig.Rollkit.ExecutorAddress, readConfig.Rollkit.ExecutorAddress)
	require.Equal(t, rollconf.DefaultNodeConfig.Rollkit.Aggregator, readConfig.Rollkit.Aggregator)
	require.Equal(t, rollconf.DefaultNodeConfig.Rollkit.Light, readConfig.Rollkit.Light)
	require.Equal(t, rollconf.DefaultNodeConfig.Rollkit.LazyAggregator, readConfig.Rollkit.LazyAggregator)

	// Also verify the complete Rollkit config structure matches
	require.Equal(t, rollconf.DefaultNodeConfig.Rollkit, readConfig.Rollkit)
}
