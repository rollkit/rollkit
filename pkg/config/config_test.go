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

func TestDefaultConfig(t *testing.T) {
	// Test that default config has expected values
	def := DefaultConfig
	assert.Equal(t, "data", def.DBPath)
	assert.Equal(t, false, def.Node.Aggregator)
	assert.Equal(t, false, def.Node.Light)
	assert.Equal(t, DefaultConfig.DA.Address, def.DA.Address)
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
	assert.Equal(t, "file", def.Signer.SignerType)
	assert.Equal(t, "config", def.Signer.SignerPath)
	assert.Equal(t, "127.0.0.1:7331", def.RPC.Address)
}

func TestAddFlags(t *testing.T) {
	// Create a command with flags
	cmd := &cobra.Command{Use: "test"}
	AddGlobalFlags(cmd, "test") // Add basic flags first
	AddFlags(cmd)

	// Get both persistent and regular flags
	flags := cmd.Flags()
	persistentFlags := cmd.PersistentFlags()

	// Test specific flags
	assertFlagValue(t, flags, FlagDBPath, DefaultConfig.DBPath)

	// Node flags
	assertFlagValue(t, flags, FlagAggregator, DefaultConfig.Node.Aggregator)
	assertFlagValue(t, flags, FlagLight, DefaultConfig.Node.Light)
	assertFlagValue(t, flags, FlagBlockTime, DefaultConfig.Node.BlockTime.Duration)
	assertFlagValue(t, flags, FlagTrustedHash, DefaultConfig.Node.TrustedHash)
	assertFlagValue(t, flags, FlagLazyAggregator, DefaultConfig.Node.LazyAggregator)
	assertFlagValue(t, flags, FlagMaxPendingBlocks, DefaultConfig.Node.MaxPendingBlocks)
	assertFlagValue(t, flags, FlagLazyBlockTime, DefaultConfig.Node.LazyBlockTime.Duration)

	// DA flags
	assertFlagValue(t, flags, FlagDAAddress, DefaultConfig.DA.Address)
	assertFlagValue(t, flags, FlagDAAuthToken, DefaultConfig.DA.AuthToken)
	assertFlagValue(t, flags, FlagDABlockTime, DefaultConfig.DA.BlockTime.Duration)
	assertFlagValue(t, flags, FlagDAGasPrice, DefaultConfig.DA.GasPrice)
	assertFlagValue(t, flags, FlagDAGasMultiplier, DefaultConfig.DA.GasMultiplier)
	assertFlagValue(t, flags, FlagDAStartHeight, DefaultConfig.DA.StartHeight)
	assertFlagValue(t, flags, FlagDANamespace, DefaultConfig.DA.Namespace)
	assertFlagValue(t, flags, FlagDASubmitOptions, DefaultConfig.DA.SubmitOptions)
	assertFlagValue(t, flags, FlagDAMempoolTTL, DefaultConfig.DA.MempoolTTL)

	// P2P flags
	assertFlagValue(t, flags, FlagP2PListenAddress, DefaultConfig.P2P.ListenAddress)
	assertFlagValue(t, flags, FlagP2PPeers, DefaultConfig.P2P.Peers)
	assertFlagValue(t, flags, FlagP2PBlockedPeers, DefaultConfig.P2P.BlockedPeers)
	assertFlagValue(t, flags, FlagP2PAllowedPeers, DefaultConfig.P2P.AllowedPeers)

	// Instrumentation flags
	instrDef := DefaultInstrumentationConfig()
	assertFlagValue(t, flags, FlagPrometheus, instrDef.Prometheus)
	assertFlagValue(t, flags, FlagPrometheusListenAddr, instrDef.PrometheusListenAddr)
	assertFlagValue(t, flags, FlagMaxOpenConnections, instrDef.MaxOpenConnections)
	assertFlagValue(t, flags, FlagPprof, instrDef.Pprof)
	assertFlagValue(t, flags, FlagPprofListenAddr, instrDef.PprofListenAddr)

	// Logging flags (in persistent flags)
	assertFlagValue(t, persistentFlags, FlagLogLevel, DefaultConfig.Log.Level)
	assertFlagValue(t, persistentFlags, FlagLogFormat, "text")
	assertFlagValue(t, persistentFlags, FlagLogTrace, false)

	// Signer flags
	assertFlagValue(t, flags, FlagSignerPassphrase, "")
	assertFlagValue(t, flags, FlagSignerType, "file")
	assertFlagValue(t, flags, FlagSignerPath, DefaultConfig.Signer.SignerPath)

	// RPC flags
	assertFlagValue(t, flags, FlagRPCAddress, DefaultConfig.RPC.Address)

	// Count the number of flags we're explicitly checking
	expectedFlagCount := 35 // Update this number if you add more flag checks above

	// Get the actual number of flags (both regular and persistent)
	actualFlagCount := 0
	flags.VisitAll(func(flag *pflag.Flag) {
		actualFlagCount++
	})
	persistentFlags.VisitAll(func(flag *pflag.Flag) {
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

func TestLoad(t *testing.T) {
	tempDir := t.TempDir()

	// Create a YAML file in the temporary directory
	yamlPath := filepath.Join(tempDir, AppConfigDir, ConfigName)
	yamlContent := `
node:
  aggregator: true
  block_time: "5s"

da:
  address: "http://yaml-da:26657"

config_dir: "config"

signer:
  signer_type: "file"
  signer_path: "something/config"
`
	err := os.MkdirAll(filepath.Dir(yamlPath), 0o700)
	require.NoError(t, err)
	err = os.WriteFile(yamlPath, []byte(yamlContent), 0o600)
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
	AddGlobalFlags(cmd, "test") // Add basic flags first

	// Set some flags that should override YAML values
	flagArgs := []string{
		"--home", tempDir,
		"--rollkit.node.block_time", "10s",
		"--rollkit.da.address", "http://flag-da:26657",
		"--rollkit.node.light", "true", // This is not in YAML, should be set from flag
		"--rollkit.rpc.address", "127.0.0.1:7332",
	}
	cmd.SetArgs(flagArgs)
	err = cmd.ParseFlags(flagArgs)
	require.NoError(t, err)

	// Load the configuration
	config, err := Load(cmd)
	require.NoError(t, err)
	require.NoError(t, config.Validate())

	// Verify the order of precedence:
	// 1. Default values should be overridden by YAML
	assert.Equal(t, true, config.Node.Aggregator, "Aggregator should be set from YAML")

	// 2. YAML values should be overridden by flags
	assert.Equal(t, 10*time.Second, config.Node.BlockTime.Duration, "BlockTime should be overridden by flag")
	assert.Equal(t, "http://flag-da:26657", config.DA.Address, "DAAddress should be overridden by flag")

	// 3. Flags not in YAML should be set
	assert.Equal(t, true, config.Node.Light, "Light should be set from flag")

	// 4. Values not in flags or YAML should remain as default
	assert.Equal(t, DefaultConfig.DA.BlockTime.Duration, config.DA.BlockTime.Duration, "DABlockTime should remain as default")

	// 5. Signer values should be set from flags
	assert.Equal(t, "file", config.Signer.SignerType, "SignerType should be set from flag")
	assert.Equal(t, "something/config", config.Signer.SignerPath, "SignerPath should be set from flag")

	assert.Equal(t, "127.0.0.1:7332", config.RPC.Address, "RPCAddress should be set from flag")
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
