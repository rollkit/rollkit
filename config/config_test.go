package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultNodeConfig(t *testing.T) {
	// Test that default config has expected values
	def := DefaultNodeConfig

	assert.Equal(t, DefaultRootDir(), def.RootDir)
	assert.Equal(t, "data", def.DBPath)
	assert.Equal(t, true, def.Node.Aggregator)
	assert.Equal(t, false, def.Node.Light)
	assert.Equal(t, DefaultDAAddress, def.Node.DAAddress)
	assert.Equal(t, "", def.Node.DAAuthToken)
	assert.Equal(t, float64(-1), def.Node.DAGasPrice)
	assert.Equal(t, float64(0), def.Node.DAGasMultiplier)
	assert.Equal(t, "", def.Node.DASubmitOptions)
	assert.Equal(t, "", def.Node.DANamespace)
	assert.Equal(t, 1*time.Second, def.Node.BlockTime)
	assert.Equal(t, 15*time.Second, def.Node.DABlockTime)
	assert.Equal(t, uint64(0), def.Node.DAStartHeight)
	assert.Equal(t, uint64(0), def.Node.DAMempoolTTL)
	assert.Equal(t, uint64(0), def.Node.MaxPendingBlocks)
	assert.Equal(t, false, def.Node.LazyAggregator)
	assert.Equal(t, 60*time.Second, def.Node.LazyBlockTime)
	assert.Equal(t, "", def.Node.TrustedHash)
	assert.Equal(t, DefaultSequencerAddress, def.Node.SequencerAddress)
	assert.Equal(t, DefaultSequencerRollupID, def.Node.SequencerRollupID)
	assert.Equal(t, DefaultExecutorAddress, def.Node.ExecutorAddress)
}

func TestAddFlags(t *testing.T) {
	cmd := &cobra.Command{}
	AddFlags(cmd)

	// Test that all flags are added
	flags := cmd.Flags()

	// Test root flags
	assert.NotNil(t, flags.Lookup(FlagRootDir))
	assert.NotNil(t, flags.Lookup(FlagDBPath))

	// Test Rollkit flags
	assert.NotNil(t, flags.Lookup(FlagAggregator))
	assert.NotNil(t, flags.Lookup(FlagLazyAggregator))
	assert.NotNil(t, flags.Lookup(FlagDAAddress))
	assert.NotNil(t, flags.Lookup(FlagDAAuthToken))
	assert.NotNil(t, flags.Lookup(FlagBlockTime))
	assert.NotNil(t, flags.Lookup(FlagDABlockTime))
	assert.NotNil(t, flags.Lookup(FlagDAGasPrice))
	assert.NotNil(t, flags.Lookup(FlagDAGasMultiplier))
	assert.NotNil(t, flags.Lookup(FlagDAStartHeight))
	assert.NotNil(t, flags.Lookup(FlagDANamespace))
	assert.NotNil(t, flags.Lookup(FlagDASubmitOptions))
	assert.NotNil(t, flags.Lookup(FlagLight))
	assert.NotNil(t, flags.Lookup(FlagTrustedHash))
	assert.NotNil(t, flags.Lookup(FlagMaxPendingBlocks))
	assert.NotNil(t, flags.Lookup(FlagDAMempoolTTL))
	assert.NotNil(t, flags.Lookup(FlagLazyBlockTime))
	assert.NotNil(t, flags.Lookup(FlagSequencerAddress))
	assert.NotNil(t, flags.Lookup(FlagSequencerRollupID))
	assert.NotNil(t, flags.Lookup(FlagExecutorAddress))

	// Test instrumentation flags
	assert.NotNil(t, flags.Lookup(FlagPrometheus))
	assert.NotNil(t, flags.Lookup(FlagPrometheusListenAddr))
	assert.NotNil(t, flags.Lookup(FlagMaxOpenConnections))

	// Test P2P flags
	assert.NotNil(t, flags.Lookup(FlagP2PListenAddress))
	assert.NotNil(t, flags.Lookup(FlagP2PSeeds))
	assert.NotNil(t, flags.Lookup(FlagP2PBlockedPeers))
	assert.NotNil(t, flags.Lookup(FlagP2PAllowedPeers))

	// Test TOML config flags
	assert.NotNil(t, flags.Lookup(FlagEntrypoint))
	assert.NotNil(t, flags.Lookup(FlagChainConfigDir))

	// Verify that there are no additional flags
	// Count the number of flags we're explicitly checking
	expectedFlagCount := 30 // Update this number if you add more flag checks above

	// Get the actual number of flags
	actualFlagCount := 0
	flags.VisitAll(func(flag *pflag.Flag) {
		actualFlagCount++
	})

	// Verify that the counts match
	assert.Equal(t, expectedFlagCount, actualFlagCount, "Number of flags doesn't match. If you added a new flag, please update the test.")
}

func TestLoadNodeConfig(t *testing.T) {
	// Create a temporary directory for the test
	tempDir := t.TempDir()

	// Create a TOML file in the temporary directory
	tomlPath := filepath.Join(tempDir, RollkitConfigToml)
	tomlContent := `
entrypoint = "./cmd/app/main.go"

[rollkit]
aggregator = true
block_time = "5s"
da_address = "http://toml-da:26657"

[chain]
config_dir = "config"
`
	err := os.WriteFile(tomlPath, []byte(tomlContent), 0600)
	require.NoError(t, err)

	// Change to the temporary directory so the config file can be found
	originalDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() {
		err := os.Chdir(originalDir)
		if err != nil {
			t.Logf("Failed to change back to original directory: %v", err)
		}
	}()
	err = os.Chdir(tempDir)
	require.NoError(t, err)

	// Verify that the TOML file exists
	_, err = os.Stat(tomlPath)
	require.NoError(t, err, "TOML file should exist at %s", tomlPath)

	// Create a command with flags
	cmd := &cobra.Command{Use: "test"}
	AddFlags(cmd)

	// Set some flags that should override TOML values
	flagArgs := []string{
		"--rollkit.block_time", "10s",
		"--rollkit.da_address", "http://flag-da:26657",
		"--rollkit.light", "true", // This is not in TOML, should be set from flag
	}
	cmd.SetArgs(flagArgs)
	err = cmd.ParseFlags(flagArgs)
	require.NoError(t, err)

	// Load the configuration
	config, err := LoadNodeConfig(cmd)
	require.NoError(t, err)

	// Verify the order of precedence:
	// 1. Default values should be overridden by TOML
	assert.Equal(t, "./cmd/app/main.go", config.Entrypoint, "Entrypoint should be set from TOML")
	assert.Equal(t, true, config.Node.Aggregator, "Aggregator should be set from TOML")

	// 2. TOML values should be overridden by flags
	assert.Equal(t, 10*time.Second, config.Node.BlockTime, "BlockTime should be overridden by flag")
	assert.Equal(t, "http://flag-da:26657", config.Node.DAAddress, "DAAddress should be overridden by flag")

	// 3. Flags not in TOML should be set
	assert.Equal(t, true, config.Node.Light, "Light should be set from flag")

	// 4. Values not in flags or TOML should remain as default
	assert.Equal(t, DefaultNodeConfig.Node.DABlockTime, config.Node.DABlockTime, "DABlockTime should remain as default")
}
