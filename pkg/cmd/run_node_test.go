package cmd

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log/v2"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	coreda "github.com/rollkit/rollkit/core/da"
	coreexecutor "github.com/rollkit/rollkit/core/execution"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	rollconf "github.com/rollkit/rollkit/pkg/config"
	"github.com/rollkit/rollkit/pkg/p2p"
	"github.com/rollkit/rollkit/pkg/signer"
	filesigner "github.com/rollkit/rollkit/pkg/signer/file"
)

const MockDANamespace = "test"

func createTestComponents(_ context.Context, t *testing.T) (coreexecutor.Executor, coresequencer.Sequencer, coreda.DA, signer.Signer, *p2p.Client, datastore.Batching, func()) {
	executor := coreexecutor.NewDummyExecutor()
	sequencer := coresequencer.NewDummySequencer()
	dummyDA := coreda.NewDummyDA(100_000, 0, 0, 10*time.Second)
	dummyDA.StartHeightTicker()
	stopDAHeightTicker := func() {
		dummyDA.StopHeightTicker()
	}
	tmpDir := t.TempDir()
	keyProvider, err := filesigner.CreateFileSystemSigner(filepath.Join(tmpDir, "config"), []byte{})
	if err != nil {
		panic(err)
	}
	// Create a dummy P2P client and datastore for testing
	p2pClient := &p2p.Client{}
	ds := datastore.NewMapDatastore()

	return executor, sequencer, dummyDA, keyProvider, p2pClient, ds, stopDAHeightTicker
}

func TestParseFlags(t *testing.T) {
	flags := []string{
		"--home", "custom/root/dir",
		"--rollkit.db_path", "custom/db/path",

		// P2P flags
		"--rollkit.p2p.listen_address", "tcp://127.0.0.1:27000",
		"--rollkit.p2p.peers", "node1@127.0.0.1:27001,node2@127.0.0.1:27002",
		"--rollkit.p2p.blocked_peers", "node3@127.0.0.1:27003,node4@127.0.0.1:27004",
		"--rollkit.p2p.allowed_peers", "node5@127.0.0.1:27005,node6@127.0.0.1:27006",

		// Node flags
		"--rollkit.node.aggregator=false",
		"--rollkit.node.block_time", "2s",
		"--rollkit.da.address", "http://127.0.0.1:27005",
		"--rollkit.da.auth_token", "token",
		"--rollkit.da.block_time", "20s",
		"--rollkit.da.gas_multiplier", "1.5",
		"--rollkit.da.gas_price", "1.5",
		"--rollkit.da.mempool_ttl", "10",
		"--rollkit.da.namespace", "namespace",
		"--rollkit.da.start_height", "100",
		"--rollkit.node.lazy_mode",
		"--rollkit.node.lazy_block_interval", "2m",
		"--rollkit.node.light",
		"--rollkit.node.max_pending_headers_and_data", "100",
		"--rollkit.node.trusted_hash", "abcdef1234567890",
		"--rollkit.da.submit_options", "custom-options",
		// Instrumentation flags
		"--rollkit.instrumentation.prometheus", "true",
		"--rollkit.instrumentation.prometheus_listen_addr", ":26665",
		"--rollkit.instrumentation.max_open_connections", "1",
	}

	args := append([]string{"start"}, flags...)

	executor, sequencer, dac, keyProvider, p2pClient, ds, stopDAHeightTicker := createTestComponents(context.Background(), t)
	defer stopDAHeightTicker()

	nodeConfig := rollconf.DefaultConfig
	nodeConfig.RootDir = t.TempDir()

	newRunNodeCmd := newRunNodeCmd(t.Context(), executor, sequencer, dac, keyProvider, p2pClient, ds, nodeConfig)
	_ = newRunNodeCmd.Flags().Set(rollconf.FlagRootDir, "custom/root/dir")
	if err := newRunNodeCmd.ParseFlags(args); err != nil {
		t.Errorf("Error: %v", err)
	}

	nodeConfig, err := ParseConfig(newRunNodeCmd)
	if err != nil {
		t.Errorf("Error: %v", err)
	}

	testCases := []struct {
		name     string
		got      any
		expected any
	}{
		{"RootDir", nodeConfig.RootDir, "custom/root/dir"},
		{"DBPath", nodeConfig.DBPath, "custom/db/path"},

		// P2P fields
		{"ListenAddress", nodeConfig.P2P.ListenAddress, "tcp://127.0.0.1:27000"},
		{"Peers", nodeConfig.P2P.Peers, "node1@127.0.0.1:27001,node2@127.0.0.1:27002"},
		{"BlockedPeers", nodeConfig.P2P.BlockedPeers, "node3@127.0.0.1:27003,node4@127.0.0.1:27004"},
		{"AllowedPeers", nodeConfig.P2P.AllowedPeers, "node5@127.0.0.1:27005,node6@127.0.0.1:27006"},

		// Node fields
		{"Aggregator", nodeConfig.Node.Aggregator, false},
		{"BlockTime", nodeConfig.Node.BlockTime.Duration, 2 * time.Second},
		{"DAAddress", nodeConfig.DA.Address, "http://127.0.0.1:27005"},
		{"DAAuthToken", nodeConfig.DA.AuthToken, "token"},
		{"DABlockTime", nodeConfig.DA.BlockTime.Duration, 20 * time.Second},
		{"DAGasMultiplier", nodeConfig.DA.GasMultiplier, 1.5},
		{"DAGasPrice", nodeConfig.DA.GasPrice, 1.5},
		{"DAMempoolTTL", nodeConfig.DA.MempoolTTL, uint64(10)},
		{"DANamespace", nodeConfig.DA.Namespace, "namespace"},
		{"DAStartHeight", nodeConfig.DA.StartHeight, uint64(100)},
		{"LazyAggregator", nodeConfig.Node.LazyMode, true},
		{"LazyBlockTime", nodeConfig.Node.LazyBlockInterval.Duration, 2 * time.Minute},
		{"Light", nodeConfig.Node.Light, true},
		{"MaxPendingHeadersAndData", nodeConfig.Node.MaxPendingHeadersAndData, uint64(100)},
		{"TrustedHash", nodeConfig.Node.TrustedHash, "abcdef1234567890"},
		{"DASubmitOptions", nodeConfig.DA.SubmitOptions, "custom-options"},
		{"Prometheus", nodeConfig.Instrumentation.Prometheus, true},
		{"PrometheusListenAddr", nodeConfig.Instrumentation.PrometheusListenAddr, ":26665"},
		{"MaxOpenConnections", nodeConfig.Instrumentation.MaxOpenConnections, 1},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if !reflect.DeepEqual(tc.got, tc.expected) {
				t.Errorf("Expected %v, got %v", tc.expected, tc.got)
			}
		})
	}
}

func TestAggregatorFlagInvariants(t *testing.T) {
	flagVariants := [][]string{{
		"--rollkit.node.aggregator=false",
	}, {
		"--rollkit.node.aggregator=true",
	}, {
		"--rollkit.node.aggregator",
	}}

	validValues := []bool{false, true, true}

	for i, flags := range flagVariants {
		args := append([]string{"start"}, flags...)

		executor, sequencer, dac, keyProvider, p2pClient, ds, stopDAHeightTicker := createTestComponents(context.Background(), t)
		defer stopDAHeightTicker()

		nodeConfig := rollconf.DefaultConfig
		nodeConfig.RootDir = t.TempDir()

		newRunNodeCmd := newRunNodeCmd(t.Context(), executor, sequencer, dac, keyProvider, p2pClient, ds, nodeConfig)
		_ = newRunNodeCmd.Flags().Set(rollconf.FlagRootDir, "custom/root/dir")

		if err := newRunNodeCmd.ParseFlags(args); err != nil {
			t.Errorf("Error: %v", err)
		}

		nodeConfig, err := ParseConfig(newRunNodeCmd)
		if err != nil {
			t.Errorf("Error: %v", err)
		}

		if nodeConfig.Node.Aggregator != validValues[i] {
			t.Errorf("Expected %v, got %v", validValues[i], nodeConfig.Node.Aggregator)
		}
	}
}

// TestDefaultAggregatorValue verifies that the default value of Aggregator is true
// when no flag is specified
func TestDefaultAggregatorValue(t *testing.T) {
	testCases := []struct {
		name     string
		expected bool
	}{
		{"DefaultAggregatorTrue", true},
		{"DefaultAggregatorFalse", false},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			executor, sequencer, dac, keyProvider, p2pClient, ds, stopDAHeightTicker := createTestComponents(context.Background(), t)
			defer stopDAHeightTicker()

			nodeConfig := rollconf.DefaultConfig

			newRunNodeCmd := newRunNodeCmd(t.Context(), executor, sequencer, dac, keyProvider, p2pClient, ds, nodeConfig)
			_ = newRunNodeCmd.Flags().Set(rollconf.FlagRootDir, "custom/root/dir")

			// Create a new command without specifying any flags
			var args []string
			if tc.expected {
				args = []string{"start", "--rollkit.node.aggregator"}
			} else {
				args = []string{"start", "--rollkit.node.aggregator=false"}
			}

			if err := newRunNodeCmd.ParseFlags(args); err != nil {
				t.Errorf("Error parsing flags: %v", err)
			}

			nodeConfig, err := ParseConfig(newRunNodeCmd)
			if err != nil {
				t.Errorf("Error parsing config: %v", err)
			}

			if tc.expected {
				// Verify that Aggregator is true by default
				assert.True(t, nodeConfig.Node.Aggregator, "Expected Aggregator to be true by default")
			} else {
				// Verify that Aggregator is false when explicitly set
				assert.False(t, nodeConfig.Node.Aggregator)
			}
		})
	}
}

func TestSetupLogger(t *testing.T) {
	testCases := []struct {
		name        string
		config      rollconf.LogConfig
		expectPanic bool // We can't easily inspect the logger internals, so we check for panics
	}{
		{"DefaultInfoText", rollconf.LogConfig{Format: "text", Level: "info", Trace: false}, false},
		{"JSONFormat", rollconf.LogConfig{Format: "json", Level: "info", Trace: false}, false},
		{"DebugLevel", rollconf.LogConfig{Format: "text", Level: "debug", Trace: false}, false},
		{"WarnLevel", rollconf.LogConfig{Format: "text", Level: "warn", Trace: false}, false},
		{"ErrorLevel", rollconf.LogConfig{Format: "text", Level: "error", Trace: false}, false},
		{"UnknownLevelDefaultsToInfo", rollconf.LogConfig{Format: "text", Level: "unknown", Trace: false}, false},
		{"TraceEnabled", rollconf.LogConfig{Format: "text", Level: "info", Trace: true}, false},
		{"JSONWithTrace", rollconf.LogConfig{Format: "json", Level: "debug", Trace: true}, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.expectPanic {
				assert.Panics(t, func() {
					_ = SetupLogger(tc.config)
				})
			} else {
				assert.NotPanics(t, func() {
					logger := SetupLogger(tc.config)
					assert.NotNil(t, logger)
					// Basic check to ensure logger works
					logger.Info("Test log message")
				})
			}
		})
	}
}

// TestCentralizedAddresses verifies that when centralized service flags are provided,
// the configuration fields in nodeConfig are updated accordingly, ensuring that mocks are skipped.
func TestCentralizedAddresses(t *testing.T) {
	nodeConfig := rollconf.DefaultConfig

	args := []string{
		"start",
		"--rollkit.da.address=http://central-da:26657",
	}

	executor, sequencer, dac, keyProvider, p2pClient, ds, stopDAHeightTicker := createTestComponents(context.Background(), t)
	defer stopDAHeightTicker()

	cmd := newRunNodeCmd(t.Context(), executor, sequencer, dac, keyProvider, p2pClient, ds, nodeConfig)
	_ = cmd.Flags().Set(rollconf.FlagRootDir, "custom/root/dir")
	if err := cmd.ParseFlags(args); err != nil {
		t.Fatalf("ParseFlags error: %v", err)
	}

	nodeConfig, err := ParseConfig(cmd)
	if err != nil {
		t.Fatalf("parseConfig error: %v", err)
	}

	if nodeConfig.DA.Address != "http://central-da:26657" {
		t.Errorf("Expected nodeConfig.Rollkit.DAAddress to be 'http://central-da:26657', got '%s'", nodeConfig.DA.Address)
	}
}

func TestStartNodeErrors(t *testing.T) {
	baseCtx := context.Background()

	executor, sequencer, dac, _, p2pClient, ds, stopDAHeightTicker := createTestComponents(baseCtx, t)
	defer stopDAHeightTicker()

	tmpDir := t.TempDir()

	dummyConfigDir := filepath.Join(tmpDir, "config")
	err := os.MkdirAll(dummyConfigDir, 0o755)
	assert.NoError(t, err)
	dummyGenesisPath := filepath.Join(dummyConfigDir, "genesis.json")
	err = os.WriteFile(dummyGenesisPath, []byte(`{"chain_id":"test","initial_height":"1"}`), 0o600)
	assert.NoError(t, err)

	// Create a dummy signer file path
	dummySignerPath := filepath.Join(tmpDir, "signer")
	_, err = filesigner.CreateFileSystemSigner(dummySignerPath, []byte("password"))
	assert.NoError(t, err)

	testCases := []struct {
		name           string
		configModifier func(cfg *rollconf.Config)
		cmdModifier    func(cmd *cobra.Command)
		expectedError  string
		expectPanic    bool
	}{
		{
			name: "GRPCSignerPanic",
			configModifier: func(cfg *rollconf.Config) {
				cfg.RootDir = tmpDir
				cfg.Signer.SignerType = "grpc"
				cfg.Node.Aggregator = true
			},
			expectPanic: true,
		},
		{
			name: "UnknownSignerError",
			configModifier: func(cfg *rollconf.Config) {
				cfg.RootDir = tmpDir
				cfg.Signer.SignerType = "unknown"
				cfg.Node.Aggregator = true
			},
			expectedError: "unknown remote signer type: unknown",
		},
		{
			name: "LoadGenesisError",
			configModifier: func(cfg *rollconf.Config) {
				cfg.RootDir = filepath.Join(tmpDir, "nonexistent_root")
				err := os.MkdirAll(filepath.Join(cfg.RootDir, "config"), 0o755)
				assert.NoError(t, err)
			},
			expectedError: "failed to load genesis:",
		},
		{
			name: "LoadFileSystemSignerError",
			configModifier: func(cfg *rollconf.Config) {
				cfg.RootDir = tmpDir
				cfg.Node.Aggregator = true
				cfg.Signer.SignerType = "file"
				cfg.Signer.SignerPath = filepath.Join(tmpDir, "nonexistent_signer")
			},
			cmdModifier:   nil,
			expectedError: "no such file or directory",
		},
		// TODO: Add test case for node.NewNode error if possible with mocks
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			nodeConfig := rollconf.DefaultConfig

			if tc.configModifier != nil {
				tc.configModifier(&nodeConfig)
			}

			dummySigner, _ := filesigner.CreateFileSystemSigner(dummySignerPath, []byte("password"))

			cmd := newRunNodeCmd(baseCtx, executor, sequencer, dac, dummySigner, p2pClient, ds, nodeConfig)

			cmd.SetContext(baseCtx)

			if tc.cmdModifier != nil {
				tc.cmdModifier(cmd)
			}
			_ = logging.SetLogLevel("test", "FATAL")

			runFunc := func() {
				currentTestLogger := logging.Logger("TestStartNodeErrors")
				_ = logging.SetLogLevel("TestStartNodeErrors", "FATAL")
				err := StartNode(currentTestLogger, cmd, executor, sequencer, dac, p2pClient, ds, nodeConfig, nil)
				if tc.expectedError != "" {
					assert.ErrorContains(t, err, tc.expectedError)
				} else {
					if !tc.expectPanic {
						assert.NoError(t, err)
					}
				}
			}

			if tc.expectPanic {
				assert.Panics(t, runFunc)
			} else {
				assert.NotPanics(t, runFunc)
				checkLogger := logging.Logger("TestStartNodeErrors-check")
				_ = logging.SetLogLevel("TestStartNodeErrors-check", "FATAL")
				err := StartNode(checkLogger, cmd, executor, sequencer, dac, p2pClient, ds, nodeConfig, nil)
				if tc.expectedError != "" {
					assert.ErrorContains(t, err, tc.expectedError)
				}
			}
		})
	}
}

// newRunNodeCmd returns the command that allows the CLI to start a node.
func newRunNodeCmd(
	ctx context.Context,
	executor coreexecutor.Executor,
	sequencer coresequencer.Sequencer,
	dac coreda.DA,
	remoteSigner signer.Signer,
	p2pClient *p2p.Client,
	datastore datastore.Batching,
	nodeConfig rollconf.Config,
) *cobra.Command {
	if executor == nil {
		panic("executor cannot be nil")
	}
	if sequencer == nil {
		panic("sequencer cannot be nil")
	}
	if dac == nil {
		panic("da client cannot be nil")
	}

	cmd := &cobra.Command{
		Use:     "start",
		Aliases: []string{"node", "run"},
		Short:   "Run the rollkit node",
		RunE: func(cmd *cobra.Command, args []string) error {
			runNodeLogger := logging.Logger("runNodeCmd")
			_ = logging.SetLogLevel("runNodeCmd", "FATAL")
			return StartNode(runNodeLogger, cmd, executor, sequencer, dac, p2pClient, datastore, nodeConfig, nil)
		},
	}

	rollconf.AddFlags(cmd)
	rollconf.AddGlobalFlags(cmd, "")

	return cmd
}
