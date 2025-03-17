package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	rollconf "github.com/rollkit/rollkit/config"
)

func TestInitYamlCommand(t *testing.T) {
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
	cmd := NewYamlCmd()
	cmd.SetArgs([]string{"init"})
	err = cmd.Execute()
	require.NoError(t, err)

	// Verify the file was created
	configPath := filepath.Join(dir, rollconf.RollkitConfigYaml)
	_, err = os.Stat(configPath)
	require.NoError(t, err)

	// Verify the config can be read
	_, err = rollconf.ReadYaml(dir)
	require.NoError(t, err)

	// Read the file content directly to verify the YAML structure
	//nolint:gosec // This is a test file and we control the input
	content, err := os.ReadFile(configPath)
	require.NoError(t, err)
	yamlContent := string(content)

	// Verify that the YAML file content contains the expected values
	// Group verifications by category

	// Verify time values
	require.Contains(t, yamlContent, "block_time: ")
	require.Contains(t, yamlContent, "1s")

	// Verify that the YAML contains the da section
	require.Contains(t, yamlContent, "da:")
	require.Contains(t, yamlContent, "block_time: ")
	require.Contains(t, yamlContent, "15s")
	require.Contains(t, yamlContent, "lazy_block_time: ")
	require.Contains(t, yamlContent, "1m0s")

	// Verify addresses
	require.Contains(t, yamlContent, "address: ")
	require.Contains(t, yamlContent, "http://localhost:26658")
}
