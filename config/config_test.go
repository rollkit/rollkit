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
	assert.Equal(t, DefaultDAAddress, def.DA.Address)
	assert.Equal(t, "", def.DA.AuthToken)
	assert.Equal(t, float64(-1), def.DA.GasPrice)
	assert.Equal(t, float64(0), def.DA.GasMultiplier)
	assert.Equal(t, "", def.DA.SubmitOptions)
	assert.Equal(t, "", def.DA.Namespace)
	assert.Equal(t, 1*time.Second, def.Node.BlockTime.Duration)
	assert.Equal(t, 15*time.Second, def.DA.BlockTime.Duration)
	assert.Equal(t, uint64(0), def.DA.StartHeight)
	assert.Equal(t, uint64(0), def.DA.MempoolTTL)
	assert.Equal(t, uint64(0), def.Node.MaxPendingBlocks)
	assert.Equal(t, false, def.Node.LazyAggregator)
	assert.Equal(t, 60*time.Second, def.Node.LazyBlockTime.Duration)
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
	// Note: FlagRootDir is added by registerFlagsRootCmd in root.go, not by AddFlags
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

	// Test YAML config flags
	assert.NotNil(t, flags.Lookup(FlagEntrypoint))
	assert.NotNil(t, flags.Lookup(FlagChainConfigDir))

	// Verify that there are no additional flags
	// Count the number of flags we're explicitly checking
	expectedFlagCount := 34 // Update this number if you add more flag checks above

	// Get the actual number of flags
	actualFlagCount := 0
	flags.VisitAll(func(flag *pflag.Flag) {
		actualFlagCount++
	})

	// Verify that the counts match
	assert.Equal(
		t,
		expectedFlagCount,
		actualFlagCount,
		"Number of flags doesn't match. If you added a new flag, please update the test.",
	)
}

func TestLoadNodeConfig(t *testing.T) {
	// Create a temporary directory for the test
	tempDir := t.TempDir()

	// Create a YAML file in the temporary directory
	yamlPath := filepath.Join(tempDir, RollkitConfigYaml)
	yamlContent := `
entrypoint: "./cmd/app/main.go"

node:
  aggregator: true
  block_time: "5s"

da:
  address: "http://yaml-da:26657"

config_dir: "config"
`
	err := os.WriteFile(yamlPath, []byte(yamlContent), 0600)
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

	// Verify that the YAML file exists
	_, err = os.Stat(yamlPath)
	require.NoError(t, err, "YAML file should exist at %s", yamlPath)

	// Create a command with flags
	cmd := &cobra.Command{Use: "test"}
	AddFlags(cmd)

	// Set some flags that should override YAML values
	flagArgs := []string{
		"--node.block_time", "10s",
		"--da.address", "http://flag-da:26657",
		"--node.light", "true", // This is not in YAML, should be set from flag
	}
	cmd.SetArgs(flagArgs)
	err = cmd.ParseFlags(flagArgs)
	require.NoError(t, err)

	// Load the configuration
	config, err := LoadNodeConfig(cmd)
	require.NoError(t, err)

	// Verify the order of precedence:
	// 1. Default values should be overridden by YAML
	assert.Equal(t, "./cmd/app/main.go", config.Entrypoint, "Entrypoint should be set from YAML")
	assert.Equal(t, true, config.Node.Aggregator, "Aggregator should be set from YAML")

	// 2. YAML values should be overridden by flags
	assert.Equal(t, 10*time.Second, config.Node.BlockTime.Duration, "BlockTime should be overridden by flag")
	assert.Equal(t, "http://flag-da:26657", config.DA.Address, "DAAddress should be overridden by flag")

	// 3. Flags not in YAML should be set
	assert.Equal(t, true, config.Node.Light, "Light should be set from flag")

	// 4. Values not in flags or YAML should remain as default
	assert.Equal(t, DefaultNodeConfig.DA.BlockTime.Duration, config.DA.BlockTime.Duration, "DABlockTime should remain as default")
}
