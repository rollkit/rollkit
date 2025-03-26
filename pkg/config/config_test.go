package config

import (
	"fmt"
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
	// Create a command with flags
	cmd := &cobra.Command{Use: "test"}
	AddFlags(cmd)

	// Get the flags
	flags := cmd.Flags()

	// Test specific flags
	assertFlagValue(t, flags, FlagDBPath, DefaultNodeConfig.DBPath)
	assertFlagValue(t, flags, FlagEntrypoint, DefaultNodeConfig.Entrypoint)
	assertFlagValue(t, flags, FlagChainConfigDir, DefaultNodeConfig.ConfigDir)

	// Node flags
	assertFlagValue(t, flags, FlagAggregator, DefaultNodeConfig.Node.Aggregator)
	assertFlagValue(t, flags, FlagLight, DefaultNodeConfig.Node.Light)
	assertFlagValue(t, flags, FlagBlockTime, DefaultNodeConfig.Node.BlockTime.Duration)
	assertFlagValue(t, flags, FlagTrustedHash, DefaultNodeConfig.Node.TrustedHash)
	assertFlagValue(t, flags, FlagLazyAggregator, DefaultNodeConfig.Node.LazyAggregator)
	assertFlagValue(t, flags, FlagMaxPendingBlocks, DefaultNodeConfig.Node.MaxPendingBlocks)
	assertFlagValue(t, flags, FlagLazyBlockTime, DefaultNodeConfig.Node.LazyBlockTime.Duration)
	assertFlagValue(t, flags, FlagSequencerAddress, DefaultNodeConfig.Node.SequencerAddress)
	assertFlagValue(t, flags, FlagSequencerRollupID, DefaultNodeConfig.Node.SequencerRollupID)
	assertFlagValue(t, flags, FlagExecutorAddress, DefaultNodeConfig.Node.ExecutorAddress)

	// DA flags
	assertFlagValue(t, flags, FlagDAAddress, DefaultNodeConfig.DA.Address)
	assertFlagValue(t, flags, FlagDAAuthToken, DefaultNodeConfig.DA.AuthToken)
	assertFlagValue(t, flags, FlagDABlockTime, DefaultNodeConfig.DA.BlockTime.Duration)
	assertFlagValue(t, flags, FlagDAGasPrice, DefaultNodeConfig.DA.GasPrice)
	assertFlagValue(t, flags, FlagDAGasMultiplier, DefaultNodeConfig.DA.GasMultiplier)
	assertFlagValue(t, flags, FlagDAStartHeight, DefaultNodeConfig.DA.StartHeight)
	assertFlagValue(t, flags, FlagDANamespace, DefaultNodeConfig.DA.Namespace)
	assertFlagValue(t, flags, FlagDASubmitOptions, DefaultNodeConfig.DA.SubmitOptions)
	assertFlagValue(t, flags, FlagDAMempoolTTL, DefaultNodeConfig.DA.MempoolTTL)

	// P2P flags
	assertFlagValue(t, flags, FlagP2PListenAddress, DefaultNodeConfig.P2P.ListenAddress)
	assertFlagValue(t, flags, FlagP2PSeeds, DefaultNodeConfig.P2P.Seeds)
	assertFlagValue(t, flags, FlagP2PBlockedPeers, DefaultNodeConfig.P2P.BlockedPeers)
	assertFlagValue(t, flags, FlagP2PAllowedPeers, DefaultNodeConfig.P2P.AllowedPeers)

	// Instrumentation flags
	instrDef := DefaultInstrumentationConfig()
	assertFlagValue(t, flags, FlagPrometheus, instrDef.Prometheus)
	assertFlagValue(t, flags, FlagPrometheusListenAddr, instrDef.PrometheusListenAddr)
	assertFlagValue(t, flags, FlagMaxOpenConnections, instrDef.MaxOpenConnections)
	assertFlagValue(t, flags, FlagPprof, instrDef.Pprof)
	assertFlagValue(t, flags, FlagPprofListenAddr, instrDef.PprofListenAddr)

	// Count the number of flags we're explicitly checking
	expectedFlagCount := 31 // Update this number if you add more flag checks above

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

func assertFlagValue(t *testing.T, flags *pflag.FlagSet, name string, expectedValue interface{}) {
	flag := flags.Lookup(name)
	assert.NotNil(t, flag, "Flag %s should exist", name)
	if flag != nil {
		switch v := expectedValue.(type) {
		case bool:
			assert.Equal(t, fmt.Sprintf("%v", v), flag.DefValue, "Flag %s should have default value %v", name, v)
		case time.Duration:
			assert.Equal(t, v.String(), flag.DefValue, "Flag %s should have default value %v", name, v)
		case int:
			assert.Equal(t, fmt.Sprintf("%d", v), flag.DefValue, "Flag %s should have default value %v", name, v)
		case uint64:
			assert.Equal(t, fmt.Sprintf("%d", v), flag.DefValue, "Flag %s should have default value %v", name, v)
		case float64:
			assert.Equal(t, fmt.Sprintf("%g", v), flag.DefValue, "Flag %s should have default value %v", name, v)
		default:
			assert.Equal(t, fmt.Sprintf("%v", v), flag.DefValue, "Flag %s should have default value %v", name, v)
		}
	}
}
