package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	// Test that default config has expected values
	def := DefaultConfig()
	def.RootDir = t.TempDir()

	assert.Equal(t, "data", def.DBPath)
	assert.Equal(t, false, def.Node.Aggregator)
	assert.Equal(t, false, def.Node.Promotable)
	assert.Equal(t, false, def.Node.Light)
	assert.Empty(t, def.DA.Address)
	assert.False(t, def.DAEnabled())
	assert.Equal(t, "", def.DA.AuthToken)
	assert.Equal(t, "", def.DA.SubmitOptions)
	assert.NotEmpty(t, def.DA.Namespace)
	assert.Equal(t, 60*time.Second, def.DA.RequestTimeout.Duration)
	assert.Equal(t, 1*time.Second, def.Node.BlockTime.Duration)
	assert.Equal(t, 6*time.Second, def.DA.BlockTime.Duration)
	assert.Equal(t, uint64(0), def.DA.MempoolTTL)
	assert.Equal(t, uint64(0), def.Node.MaxPendingHeadersAndData)
	assert.Equal(t, false, def.Node.LazyMode)
	assert.Equal(t, 60*time.Second, def.Node.LazyBlockInterval.Duration)
	assert.Equal(t, "file", def.Signer.SignerType)
	assert.Equal(t, "config", def.Signer.SignerPath)
	assert.Equal(t, "127.0.0.1:7331", def.RPC.Address)
	assert.Equal(t, PruningModeDisabled, def.Pruning.Mode)
	assert.Equal(t, DATxSignerConfig{GRPCTLS: true, KeyringDir: "keys", KeyringBackend: "file"}, def.DA.TxSigner)
	assert.NoError(t, def.Validate())
}

func TestDAEnabled(t *testing.T) {
	tests := []struct {
		name            string
		configure       func(*Config)
		expectedEnabled bool
		expectedAddress string
	}{
		{
			name:            "follower without DA address",
			expectedEnabled: false,
			expectedAddress: "",
		},
		{
			name: "follower with DA address",
			configure: func(cfg *Config) {
				cfg.DA.Address = "http://da.example:7980"
			},
			expectedEnabled: true,
			expectedAddress: "http://da.example:7980",
		},
		{
			name: "follower with whitespace around DA address",
			configure: func(cfg *Config) {
				cfg.DA.Address = "  http://da.example:7980\n"
			},
			expectedEnabled: true,
			expectedAddress: "http://da.example:7980",
		},
		{
			name: "aggregator without DA address does not initialize DA",
			configure: func(cfg *Config) {
				cfg.Node.Aggregator = true
			},
			expectedEnabled: false,
			expectedAddress: "",
		},
		{
			name: "light node does not initialize DA",
			configure: func(cfg *Config) {
				cfg.Node.Light = true
				cfg.DA.Address = "http://da.example:7980"
			},
			expectedEnabled: false,
			expectedAddress: "http://da.example:7980",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			if tt.configure != nil {
				tt.configure(&cfg)
			}

			assert.Equal(t, tt.expectedEnabled, cfg.DAEnabled())
			assert.Equal(t, tt.expectedAddress, cfg.GetDAAddress())
		})
	}
}

func TestConfigValidate_AggregatorCanBeInitializedWithoutDAAddress(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RootDir = t.TempDir()
	cfg.Node.Aggregator = true

	require.NoError(t, cfg.Validate())
}

func TestAddFlags(t *testing.T) {
	// AddFlags and the assertions below each call DefaultConfig(), whose DA.Namespace
	// is randString-seeded by time.Now().Unix(); run in a synctest bubble so both calls observe the same fake clock.
	synctest.Test(t, func(t *testing.T) {
		// Create a command with flags
		cmd := &cobra.Command{Use: "test"}
		AddGlobalFlags(cmd, "test") // Add basic flags first
		AddFlags(cmd)

		// Get both persistent and regular flags
		flags := cmd.Flags()
		persistentFlags := cmd.PersistentFlags()

		// Test specific flags
		assertFlagValue(t, flags, FlagDBPath, DefaultConfig().DBPath)
		assertFlagValue(t, flags, FlagClearCache, DefaultConfig().ClearCache)

		// Node flags
		assertFlagValue(t, flags, FlagAggregator, DefaultConfig().Node.Aggregator)
		assertFlagValue(t, flags, FlagPromotable, DefaultConfig().Node.Promotable)
		assertFlagValue(t, flags, FlagBasedSequencer, DefaultConfig().Node.BasedSequencer)
		assertFlagValue(t, flags, FlagLight, DefaultConfig().Node.Light)
		assertFlagValue(t, flags, FlagBlockTime, DefaultConfig().Node.BlockTime.Duration)
		assertFlagValue(t, flags, FlagLazyAggregator, DefaultConfig().Node.LazyMode)
		assertFlagValue(t, flags, FlagMaxPendingHeadersAndData, DefaultConfig().Node.MaxPendingHeadersAndData)
		assertFlagValue(t, flags, FlagLazyBlockTime, DefaultConfig().Node.LazyBlockInterval.Duration)
		assertFlagValue(t, flags, FlagReadinessWindowSeconds, DefaultConfig().Node.ReadinessWindowSeconds)
		assertFlagValue(t, flags, FlagReadinessMaxBlocksBehind, DefaultConfig().Node.ReadinessMaxBlocksBehind)
		assertFlagValue(t, flags, FlagScrapeInterval, DefaultConfig().Node.ScrapeInterval)

		// DA flags
		assertFlagValue(t, flags, FlagDAAddress, DefaultConfig().DA.Address)
		assertFlagValue(t, flags, FlagDAAuthToken, DefaultConfig().DA.AuthToken)
		assertFlagValue(t, flags, FlagDABlockTime, DefaultConfig().DA.BlockTime.Duration)
		assertFlagValue(t, flags, FlagDANamespace, DefaultConfig().DA.Namespace)
		assertFlagValue(t, flags, FlagDADataNamespace, DefaultConfig().DA.DataNamespace)
		assertFlagValue(t, flags, FlagDAForcedInclusionNamespace, DefaultConfig().DA.ForcedInclusionNamespace)
		assertFlagValue(t, flags, FlagDASubmitOptions, DefaultConfig().DA.SubmitOptions)
		assertFlagValue(t, flags, FlagDASigningAddresses, DefaultConfig().DA.SigningAddresses)
		assertFlagValue(t, flags, FlagDAMempoolTTL, DefaultConfig().DA.MempoolTTL)
		assertFlagValue(t, flags, FlagDAMaxSubmitAttempts, DefaultConfig().DA.MaxSubmitAttempts)
		assertFlagValue(t, flags, FlagDARequestTimeout, DefaultConfig().DA.RequestTimeout.Duration)
		assertFlagValue(t, flags, FlagDATxSignerEnabled, false)
		assertFlagValue(t, flags, FlagDATxSignerGRPCAddress, "")
		assertFlagValue(t, flags, FlagDATxSignerGRPCAuthToken, "")
		assertFlagValue(t, flags, FlagDATxSignerGRPCTLS, true)
		assertFlagValue(t, flags, FlagDATxSignerKeyringDir, "keys")
		assertFlagValue(t, flags, FlagDATxSignerKeyringBackend, "file")
		assertFlagValue(t, flags, FlagDATxSignerKeyName, "")
		assertFlagValue(t, flags, FlagDATxSignerPassphraseFile, "")

		// P2P flags
		assertFlagValue(t, flags, FlagP2PListenAddress, DefaultConfig().P2P.ListenAddress)
		assertFlagValue(t, flags, FlagP2PPeers, DefaultConfig().P2P.Peers)
		assertFlagValue(t, flags, FlagP2PBlockedPeers, DefaultConfig().P2P.BlockedPeers)
		assertFlagValue(t, flags, FlagP2PAllowedPeers, DefaultConfig().P2P.AllowedPeers)
		assertFlagValue(t, flags, FlagP2PDisableConnectionGater, DefaultConfig().P2P.DisableConnectionGater)

		// Instrumentation flags
		instrDef := DefaultInstrumentationConfig()
		assertFlagValue(t, flags, FlagPrometheus, instrDef.Prometheus)
		assertFlagValue(t, flags, FlagPrometheusListenAddr, instrDef.PrometheusListenAddr)
		assertFlagValue(t, flags, FlagMaxOpenConnections, instrDef.MaxOpenConnections)
		assertFlagValue(t, flags, FlagPprof, instrDef.Pprof)
		assertFlagValue(t, flags, FlagPprofListenAddr, instrDef.PprofListenAddr)
		assertFlagValue(t, flags, FlagTracing, instrDef.Tracing)
		assertFlagValue(t, flags, FlagTracingEndpoint, instrDef.TracingEndpoint)
		assertFlagValue(t, flags, FlagTracingSampleRate, instrDef.TracingSampleRate)
		assertFlagValue(t, flags, FlagTracingServiceName, instrDef.TracingServiceName)

		// Logging flags (in persistent flags)
		assertFlagValue(t, persistentFlags, FlagLogLevel, DefaultConfig().Log.Level)
		assertFlagValue(t, persistentFlags, FlagLogFormat, "text")
		assertFlagValue(t, persistentFlags, FlagLogTrace, false)
		assertFlagValue(t, persistentFlags, FlagRootDir, DefaultRootDirWithName("test"))

		// Signer flags
		assertFlagValue(t, flags, FlagSignerPassphraseFile, "")
		assertFlagValue(t, flags, FlagSignerType, "file")
		assertFlagValue(t, flags, FlagSignerPath, DefaultConfig().Signer.SignerPath)
		assertFlagValue(t, flags, FlagSignerKmsProvider, DefaultConfig().Signer.KMS.Provider)
		assertFlagValue(t, flags, FlagSignerKmsAwsKeyID, DefaultConfig().Signer.KMS.AWS.KeyID)
		assertFlagValue(t, flags, FlagSignerKmsAwsRegion, DefaultConfig().Signer.KMS.AWS.Region)
		assertFlagValue(t, flags, FlagSignerKmsAwsProfile, DefaultConfig().Signer.KMS.AWS.Profile)
		assertFlagValue(t, flags, FlagSignerKmsAwsTimeout, DefaultConfig().Signer.KMS.AWS.Timeout.Duration)
		assertFlagValue(t, flags, FlagSignerKmsAwsMaxRetries, DefaultConfig().Signer.KMS.AWS.MaxRetries)
		assertFlagValue(t, flags, FlagSignerKmsGcpKeyName, DefaultConfig().Signer.KMS.GCP.KeyName)
		assertFlagValue(t, flags, FlagSignerKmsGcpCredentialsFile, DefaultConfig().Signer.KMS.GCP.CredentialsFile)
		assertFlagValue(t, flags, FlagSignerKmsGcpTimeout, DefaultConfig().Signer.KMS.GCP.Timeout.Duration)
		assertFlagValue(t, flags, FlagSignerKmsGcpMaxRetries, DefaultConfig().Signer.KMS.GCP.MaxRetries)

		// RPC flags
		assertFlagValue(t, flags, FlagRPCAddress, DefaultConfig().RPC.Address)
		assertFlagValue(t, flags, FlagRPCEnableDAVisualization, DefaultConfig().RPC.EnableDAVisualization)

		// Raft flags
		assertFlagValue(t, flags, FlagRaftEnable, DefaultConfig().Raft.Enable)
		assertFlagValue(t, flags, FlagRaftNodeID, DefaultConfig().Raft.NodeID)
		assertFlagValue(t, flags, FlagRaftAddr, DefaultConfig().Raft.RaftAddr)
		assertFlagValue(t, flags, FlagRaftDir, DefaultConfig().Raft.RaftDir)
		assertFlagValue(t, flags, FlagRaftBootstrap, DefaultConfig().Raft.Bootstrap)
		assertFlagValue(t, flags, FlagRaftPeers, DefaultConfig().Raft.Peers)
		assertFlagValue(t, flags, FlagRaftSnapCount, DefaultConfig().Raft.SnapCount)
		assertFlagValue(t, flags, FlagRaftSendTimeout, DefaultConfig().Raft.SendTimeout)
		assertFlagValue(t, flags, FlagRaftHeartbeatTimeout, DefaultConfig().Raft.HeartbeatTimeout)
		assertFlagValue(t, flags, FlagRaftLeaderLeaseTimeout, DefaultConfig().Raft.LeaderLeaseTimeout)
		assertFlagValue(t, flags, FlagRaftElectionTimeout, DefaultConfig().Raft.ElectionTimeout)
		assertFlagValue(t, flags, FlagRaftSnapshotThreshold, DefaultConfig().Raft.SnapshotThreshold)
		assertFlagValue(t, flags, FlagRaftTrailingLogs, DefaultConfig().Raft.TrailingLogs)

		// Pruning flags
		assertFlagValue(t, flags, FlagPruningMode, DefaultConfig().Pruning.Mode)
		assertFlagValue(t, flags, FlagPruningKeepRecent, DefaultConfig().Pruning.KeepRecent)
		assertFlagValue(t, flags, FlagPruningInterval, DefaultConfig().Pruning.Interval.Duration)

		// Count the number of flags we're explicitly checking
		expectedFlagCount := 91 // Update this number if you add more flag checks above

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
	})
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
		"--evnode.signer.signer_path", "/path/to/signer",
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
	assert.Equal(t, DefaultConfig().DA.BlockTime.Duration, config.DA.BlockTime.Duration, "DABlockTime should remain as default")

	// 5. Signer values should be set from flags
	assert.Equal(t, "file", config.Signer.SignerType, "SignerType should be gotten from config")
	assert.Equal(t, "/path/to/signer", config.Signer.SignerPath, "SignerPath should be set from flag")

	assert.Equal(t, "127.0.0.1:7332", config.RPC.Address, "RPCAddress should be set from flag")
}

func TestLoadFromViper(t *testing.T) {
	tempDir := t.TempDir()

	// Create a YAML file in the temporary directory
	yamlPath := filepath.Join(tempDir, AppConfigDir, ConfigName)
	yamlContent := `
node:
  aggregator: true
  block_time: "5s"

da:
  address: "http://yaml-da:26657"

signer:
  signer_type: "file"
  signer_path: "something/config"
`
	err := os.MkdirAll(filepath.Dir(yamlPath), 0o700)
	require.NoError(t, err)
	err = os.WriteFile(yamlPath, []byte(yamlContent), 0o600)
	require.NoError(t, err)

	// Create a command to load the configs
	cmd := &cobra.Command{Use: "test"}
	AddFlags(cmd)
	AddGlobalFlags(cmd, "test-app")

	// Set some flags through the command line
	cmd.SetArgs([]string{
		"--home=" + tempDir,
		"--rollkit.node.lazy_mode=true",
	})
	err = cmd.Execute()
	require.NoError(t, err)

	// Load configuration using the standard Load method
	cfgFromLoad, err := Load(cmd)
	require.NoError(t, err)

	// Now create a Viper instance with the same flags
	v := viper.New()
	v.Set(FlagRootDir, tempDir)
	v.Set("rollkit.node.lazy_mode", true)

	// Load configuration using the new LoadFromViper method
	cfgFromViper, err := LoadFromViper(v)
	require.NoError(t, err)

	// Compare the results - they should be identical
	require.Equal(t, cfgFromLoad.RootDir, cfgFromViper.RootDir, "RootDir should match")
	require.Equal(t, cfgFromLoad.Node.LazyMode, cfgFromViper.Node.LazyMode, "Node.LazyMode should match")
	require.Equal(t, cfgFromLoad.Node.Aggregator, cfgFromViper.Node.Aggregator, "Node.Aggregator should match")
	require.Equal(t, cfgFromLoad.Node.BlockTime, cfgFromViper.Node.BlockTime, "Node.BlockTime should match")
	require.Equal(t, cfgFromLoad.DA.Address, cfgFromViper.DA.Address, "DA.Address should match")
	require.Equal(t, cfgFromLoad.Signer.SignerType, cfgFromViper.Signer.SignerType, "Signer.SignerType should match")
	require.Equal(t, cfgFromLoad.Signer.SignerPath, cfgFromViper.Signer.SignerPath, "Signer.SignerPath should match")

	// Test that the new LoadFromViper properly handles YAML file loading
	v = viper.New()
	v.Set(FlagRootDir, tempDir)

	cfgFromViper, err = LoadFromViper(v)
	require.NoError(t, err)

	// Check that the values from the YAML file were loaded
	require.True(t, cfgFromViper.Node.Aggregator, "Node.Aggregator should be true from YAML")
	require.Equal(t, "5s", cfgFromViper.Node.BlockTime.String(), "Node.BlockTime should be 5s from YAML")
	require.Equal(t, "http://yaml-da:26657", cfgFromViper.DA.Address, "DA.Address should match YAML")
	require.Equal(t, "file", cfgFromViper.Signer.SignerType, "Signer.SignerType should match YAML")
	require.Equal(t, "something/config", cfgFromViper.Signer.SignerPath, "Signer.SignerPath should match YAML")

	// Test that default flag values don't override config values when not explicitly set
	t.Run("default flag values don't override config when not explicitly set", func(t *testing.T) {
		// Create a viper instance that simulates having default values but IsSet() returns false
		flagViper := viper.New()
		flagViper.Set(FlagRootDir, tempDir)

		// Create a command and parse empty args to simulate flags with defaults but not explicitly set
		cmd = &cobra.Command{Use: "test"}
		AddFlags(cmd)
		AddGlobalFlags(cmd, "test")

		// Bind the flags to viper - this populates viper with default values
		// but since no args were provided, IsSet() will return false for these flags
		err = flagViper.BindPFlags(cmd.Flags())
		require.NoError(t, err)
		err = flagViper.BindPFlags(cmd.PersistentFlags())
		require.NoError(t, err)

		// Parse empty command line - this means all flags have default values but IsSet() = false
		err = cmd.ParseFlags([]string{})
		require.NoError(t, err)

		// Sanity check.
		// Verify that IsSet returns false for flag values (this is the key behavior we're testing)
		require.False(t, flagViper.IsSet("rollkit.node.aggregator"), "Flag should not be considered 'set' when using default")
		require.False(t, flagViper.IsSet("rollkit.node.block_time"), "Flag should not be considered 'set' when using default")
		require.False(t, flagViper.IsSet("rollkit.da.address"), "Flag should not be considered 'set' when using default")

		cfgFromViper, err = LoadFromViper(flagViper)
		require.NoError(t, err)

		// These values should come from YAML, not be overridden by flag defaults
		require.True(t, cfgFromViper.Node.Aggregator, "Node.Aggregator should remain true from YAML, not overridden by flag default")
		require.Equal(t, "5s", cfgFromViper.Node.BlockTime.String(), "Node.BlockTime should remain 5s from YAML, not overridden by flag default")
		require.Equal(t, "http://yaml-da:26657", cfgFromViper.DA.Address, "DA.Address should remain from YAML, not overridden by flag default")
		require.Equal(t, "something/config", cfgFromViper.Signer.SignerPath, "Signer.SignerPath should remain from YAML, not overridden by flag default")
	})
}

func TestDAConfig_GetDataNamespace(t *testing.T) {
	tests := []struct {
		name              string
		defaultNamespace  string
		dataNamespace     string
		expectedNamespace string
	}{
		{
			name:              "DataNamespace set",
			dataNamespace:     "custom-data",
			defaultNamespace:  "namespace",
			expectedNamespace: "custom-data",
		},
		{
			name:              "DataNamespace empty, fallback to default namespace",
			dataNamespace:     "",
			defaultNamespace:  "namespace",
			expectedNamespace: "namespace",
		},
		{
			name: "Both empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			daConfig := &DAConfig{
				DataNamespace: tt.dataNamespace,
				Namespace:     tt.defaultNamespace,
			}

			result := daConfig.GetDataNamespace()
			assert.Equal(t, tt.expectedNamespace, result)
		})
	}
}

func TestRaftConfig_Validate(t *testing.T) {
	// helper to build a valid base config per test (temp dir varies per subtest)
	newValid := func() RaftConfig {
		return RaftConfig{
			Enable:             true,
			NodeID:             "node-1",
			RaftAddr:           "127.0.0.1:9000",
			RaftDir:            t.TempDir(),
			Bootstrap:          false,
			Peers:              "",
			SnapCount:          1,
			SendTimeout:        1 * time.Second,
			HeartbeatTimeout:   1 * time.Second,
			LeaderLeaseTimeout: 1 * time.Second,
			ElectionTimeout:    2 * time.Second,
		}
	}

	specs := map[string]struct {
		mutate func(c *RaftConfig)
		expErr string
	}{
		"disabled": {
			mutate: func(c *RaftConfig) { c.Enable = false },
		},
		"valid": {
			mutate: func(c *RaftConfig) {},
		},
		"empty node id": {
			mutate: func(c *RaftConfig) { c.NodeID = "" },
			expErr: "node ID is required",
		},
		"empty raft addr": {
			mutate: func(c *RaftConfig) { c.RaftAddr = "" },
			expErr: "raft address is required",
		},
		"empty raft dir": {
			mutate: func(c *RaftConfig) { c.RaftDir = "" },
			expErr: "raft directory is required",
		},
		"non-positive send timeout": {
			mutate: func(c *RaftConfig) { c.SendTimeout = 0 },
			expErr: "send timeout must be positive",
		},
		"non-positive heartbeat timeout": {
			mutate: func(c *RaftConfig) { c.HeartbeatTimeout = 0 },
			expErr: "heartbeat timeout must be positive",
		},
		"non-positive leader lease timeout": {
			mutate: func(c *RaftConfig) { c.LeaderLeaseTimeout = 0 },
			expErr: "leader lease timeout must be positive",
		},
		"negative election timeout rejected": {
			mutate: func(c *RaftConfig) { c.ElectionTimeout = -1 * time.Second },
			expErr: "election timeout (-1s) must be >= 0",
		},
		"election timeout less than heartbeat timeout": {
			mutate: func(c *RaftConfig) { c.ElectionTimeout = 500 * time.Millisecond },
			expErr: "election timeout (500ms) must be >= heartbeat timeout (1s)",
		},
		"zero election timeout skips check": {
			mutate: func(c *RaftConfig) { c.ElectionTimeout = 0 },
		},
		"multiple invalid returns last": {
			mutate: func(c *RaftConfig) {
				c.NodeID = ""
				c.RaftAddr = ""
				c.HeartbeatTimeout = 0
				c.LeaderLeaseTimeout = 0
			},
			expErr: "node ID is required\nraft address is required\nheartbeat timeout must be positive\nleader lease timeout must be positive",
		},
	}

	for name, spec := range specs {
		t.Run(name, func(t *testing.T) {
			cfg := newValid()
			spec.mutate(&cfg)
			err := cfg.Validate()

			if spec.expErr != "" {
				require.Error(t, err)
				assert.Equal(t, spec.expErr, err.Error())
				return
			}
			require.NoError(t, err)
		})
	}
}

func assertFlagValue(t *testing.T, flags *pflag.FlagSet, name string, expectedValue any) {
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

func TestBasedSequencerValidation(t *testing.T) {
	tests := []struct {
		name        string
		aggregator  bool
		basedSeq    bool
		expectError bool
		errorMsg    string
	}{
		{
			name:        "based sequencer without aggregator should fail",
			aggregator:  false,
			basedSeq:    true,
			expectError: true,
			errorMsg:    "based sequencer mode requires aggregator mode to be enabled",
		},
		{
			name:        "based sequencer with aggregator should pass",
			aggregator:  true,
			basedSeq:    true,
			expectError: false,
		},
		{
			name:        "aggregator without based sequencer should pass",
			aggregator:  true,
			basedSeq:    false,
			expectError: false,
		},
		{
			name:        "neither aggregator nor based sequencer should pass",
			aggregator:  false,
			basedSeq:    false,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.RootDir = t.TempDir()
			cfg.Node.Aggregator = tt.aggregator
			cfg.Node.BasedSequencer = tt.basedSeq
			if tt.aggregator {
				cfg.DA.Address = "http://da.example:7980"
			}

			err := cfg.Validate()

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestPromotableValidation(t *testing.T) {
	tests := []struct {
		name        string
		aggregator  bool
		promotable  bool
		basedSeq    bool
		light       bool
		raft        bool
		expectError string
	}{
		{
			name:       "promotable full node should pass",
			promotable: true,
		},
		{
			name:       "promotable aggregator should pass",
			aggregator: true,
			promotable: true,
		},
		{
			name:        "promotable with based sequencer should fail",
			promotable:  true,
			basedSeq:    true,
			expectError: "promotable mode cannot be combined with based sequencer mode",
		},
		{
			name:        "promotable with light node should fail",
			promotable:  true,
			light:       true,
			expectError: "promotable mode cannot be combined with light node mode",
		},
		{
			name:        "promotable with raft should fail",
			promotable:  true,
			raft:        true,
			expectError: "promotable mode cannot be combined with Raft consensus",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.RootDir = t.TempDir()
			cfg.Node.Aggregator = tt.aggregator
			cfg.Node.Promotable = tt.promotable
			cfg.Node.BasedSequencer = tt.basedSeq
			cfg.Node.Light = tt.light
			cfg.Raft.Enable = tt.raft
			if tt.aggregator {
				cfg.DA.Address = "http://da.example:7980"
			}

			err := cfg.Validate()

			if tt.expectError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectError)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestSignerValidation(t *testing.T) {
	tests := []struct {
		name        string
		modify      func(cfg *Config)
		expectError string
	}{
		{
			name: "kms aws requires key id",
			modify: func(cfg *Config) {
				cfg.Signer.SignerType = "kms"
				cfg.Signer.KMS.Provider = "aws"
				cfg.Signer.KMS.AWS.KeyID = ""
			},
			expectError: "evnode.signer.kms.aws.key_id is required when signer.signer_type is kms and signer.kms.provider is aws",
		},
		{
			name: "kms aws requires positive timeout",
			modify: func(cfg *Config) {
				cfg.Signer.SignerType = "kms"
				cfg.Signer.KMS.Provider = "aws"
				cfg.Signer.KMS.AWS.KeyID = "arn:aws:kms:eu-central-1:123456789012:key/test"
				cfg.Signer.KMS.AWS.Timeout = DurationWrapper{0}
			},
			expectError: "evnode.signer.kms.aws.timeout must be positive",
		},
		{
			name: "kms aws retries must be non-negative",
			modify: func(cfg *Config) {
				cfg.Signer.SignerType = "kms"
				cfg.Signer.KMS.Provider = "aws"
				cfg.Signer.KMS.AWS.KeyID = "arn:aws:kms:eu-central-1:123456789012:key/test"
				cfg.Signer.KMS.AWS.MaxRetries = -1
			},
			expectError: "evnode.signer.kms.aws.max_retries must be non-negative",
		},
		{
			name: "kms gcp requires key name",
			modify: func(cfg *Config) {
				cfg.Signer.SignerType = "kms"
				cfg.Signer.KMS.Provider = "gcp"
				cfg.Signer.KMS.GCP.KeyName = ""
			},
			expectError: "evnode.signer.kms.gcp.key_name is required when signer.signer_type is kms and signer.kms.provider is gcp",
		},
		{
			name: "kms gcp requires positive timeout",
			modify: func(cfg *Config) {
				cfg.Signer.SignerType = "kms"
				cfg.Signer.KMS.Provider = "gcp"
				cfg.Signer.KMS.GCP.KeyName = "projects/p/locations/global/keyRings/r/cryptoKeys/k/cryptoKeyVersions/1"
				cfg.Signer.KMS.GCP.Timeout = DurationWrapper{0}
			},
			expectError: "evnode.signer.kms.gcp.timeout must be positive",
		},
		{
			name: "kms gcp retries must be non-negative",
			modify: func(cfg *Config) {
				cfg.Signer.SignerType = "kms"
				cfg.Signer.KMS.Provider = "gcp"
				cfg.Signer.KMS.GCP.KeyName = "projects/p/locations/global/keyRings/r/cryptoKeys/k/cryptoKeyVersions/1"
				cfg.Signer.KMS.GCP.MaxRetries = -1
			},
			expectError: "evnode.signer.kms.gcp.max_retries must be non-negative",
		},
		{
			name: "kms requires provider",
			modify: func(cfg *Config) {
				cfg.Signer.SignerType = "kms"
				cfg.Signer.KMS.Provider = ""
			},
			expectError: "evnode.signer.kms.provider must be one of: aws, gcp when signer.signer_type is kms",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.RootDir = t.TempDir()
			tt.modify(&cfg)
			err := cfg.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectError)
		})
	}
}

func TestTxSignerValidation(t *testing.T) {
	enabled := func(cfg *Config) {
		cfg.DA.Address = "https://da.da-proxy-lb.celestia-mocha.com"
		cfg.DA.TxSigner.Enabled = true
		cfg.DA.TxSigner.GRPCAddress = "grpc.da-proxy-lb.celestia-mocha.com:443"
		cfg.DA.TxSigner.KeyName = "ev"
	}
	tests := []struct {
		name        string
		modify      func(cfg *Config)
		expectError string
	}{
		{name: "disabled ignores missing fields", modify: func(cfg *Config) { cfg.DA.TxSigner.KeyringBackend = "os" }},
		{name: "enabled and complete", modify: enabled},
		{name: "test backend allowed", modify: func(cfg *Config) { enabled(cfg); cfg.DA.TxSigner.KeyringBackend = "test" }},
		{
			name:        "requires grpc address",
			modify:      func(cfg *Config) { enabled(cfg); cfg.DA.TxSigner.GRPCAddress = "" },
			expectError: "evnode.da.tx_signer.grpc_address is required",
		},
		{
			name:        "requires da address",
			modify:      func(cfg *Config) { enabled(cfg); cfg.DA.Address = "" },
			expectError: "evnode.da.address is required",
		},
		{
			name:        "requires key name",
			modify:      func(cfg *Config) { enabled(cfg); cfg.DA.TxSigner.KeyName = "" },
			expectError: "evnode.da.tx_signer.key_name is required",
		},
		{
			name:        "rejects os backend",
			modify:      func(cfg *Config) { enabled(cfg); cfg.DA.TxSigner.KeyringBackend = "os" },
			expectError: "evnode.da.tx_signer.keyring_backend must be file or test",
		},
		{
			name: "rejects signing addresses",
			modify: func(cfg *Config) {
				enabled(cfg)
				cfg.DA.SigningAddresses = []string{"celestia1abc"}
			},
			expectError: "evnode.da.signing_addresses must be empty when tx_signer is enabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.RootDir = t.TempDir()
			tt.modify(&cfg)
			err := cfg.Validate()
			if tt.expectError == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.expectError)
		})
	}
}

func TestDAConfig_GRPCAuthTokenOrDefault(t *testing.T) {
	d := DAConfig{AuthToken: "da"}
	assert.Equal(t, "da", d.GRPCAuthTokenOrDefault())
	d.TxSigner.GRPCAuthToken = "grpc"
	assert.Equal(t, "grpc", d.GRPCAuthTokenOrDefault())
}
