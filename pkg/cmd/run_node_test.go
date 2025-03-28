package cmd

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"cosmossdk.io/log"
	"github.com/ipfs/go-datastore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	coreda "github.com/rollkit/rollkit/core/da"
	coreexecutor "github.com/rollkit/rollkit/core/execution"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	"github.com/rollkit/rollkit/da"
	rollconf "github.com/rollkit/rollkit/pkg/config"
	"github.com/rollkit/rollkit/pkg/p2p"
	"github.com/rollkit/rollkit/pkg/signer"
	testExecutor "github.com/rollkit/rollkit/rollups/testapp/kv"
)

func createTestComponents(ctx context.Context) (coreexecutor.Executor, coresequencer.Sequencer, coreda.Client, signer.KeyProvider, *p2p.Client, datastore.Batching) {
	executor := testExecutor.CreateDirectKVExecutor(ctx)
	sequencer := coresequencer.NewDummySequencer()
	dummyDA := coreda.NewDummyDA(100_000, 0, 0)
	logger := log.NewLogger(os.Stdout)
	dac := da.NewDAClient(dummyDA, 0, 1.0, []byte("test"), []byte(""), logger)
	keyProvider := signer.NewFileKeyProvider("", "config", "data")

	// Create a dummy P2P client and datastore for testing
	p2pClient := &p2p.Client{}
	ds := datastore.NewMapDatastore()

	return executor, sequencer, dac, keyProvider, p2pClient, ds
}

func TestParseFlags(t *testing.T) {
	// Initialize nodeConfig with default values to avoid issues with instrument
	nodeConfig = rollconf.DefaultNodeConfig

	flags := []string{
		"--home", "custom/root/dir",
		"--rollkit.db_path", "custom/db/path",

		// P2P flags
		"--rollkit.p2p.listen_address", "tcp://127.0.0.1:27000",
		"--rollkit.p2p.seeds", "node1@127.0.0.1:27001,node2@127.0.0.1:27002",
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
		"--rollkit.node.lazy_aggregator",
		"--rollkit.node.lazy_block_time", "2m",
		"--rollkit.node.light",
		"--rollkit.node.max_pending_blocks", "100",
		"--rollkit.node.trusted_hash", "abcdef1234567890",
		"--rollkit.node.sequencer_address", "seq@127.0.0.1:27007",
		"--rollkit.node.sequencer_rollup_id", "test-rollup",
		"--rollkit.node.executor_address", "exec@127.0.0.1:27008",
		"--rollkit.da.submit_options", "custom-options",

		// Instrumentation flags
		"--rollkit.instrumentation.prometheus", "true",
		"--rollkit.instrumentation.prometheus_listen_addr", ":26665",
		"--rollkit.instrumentation.max_open_connections", "1",
	}

	args := append([]string{"start"}, flags...)

	executor, sequencer, dac, keyProvider, p2pClient, ds := createTestComponents(context.Background())

	newRunNodeCmd := NewRunNodeCmd(executor, sequencer, dac, keyProvider, p2pClient, ds)

	// Register root flags to be able to use --home flag
	rollconf.AddBasicFlags(newRunNodeCmd, "testapp")

	if err := newRunNodeCmd.ParseFlags(args); err != nil {
		t.Errorf("Error: %v", err)
	}

	if err := parseConfig(newRunNodeCmd, "custom/root/dir"); err != nil {
		t.Errorf("Error: %v", err)
	}

	testCases := []struct {
		name     string
		got      interface{}
		expected interface{}
	}{
		{"RootDir", nodeConfig.RootDir, "custom/root/dir"},
		{"DBPath", nodeConfig.DBPath, "custom/db/path"},

		// P2P fields
		{"ListenAddress", nodeConfig.P2P.ListenAddress, "tcp://127.0.0.1:27000"},
		{"Seeds", nodeConfig.P2P.Seeds, "node1@127.0.0.1:27001,node2@127.0.0.1:27002"},
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
		{"LazyAggregator", nodeConfig.Node.LazyAggregator, true},
		{"LazyBlockTime", nodeConfig.Node.LazyBlockTime.Duration, 2 * time.Minute},
		{"Light", nodeConfig.Node.Light, true},
		{"MaxPendingBlocks", nodeConfig.Node.MaxPendingBlocks, uint64(100)},
		{"TrustedHash", nodeConfig.Node.TrustedHash, "abcdef1234567890"},
		{"SequencerAddress", nodeConfig.Node.SequencerAddress, "seq@127.0.0.1:27007"},
		{"SequencerRollupID", nodeConfig.Node.SequencerRollupID, "test-rollup"},
		{"ExecutorAddress", nodeConfig.Node.ExecutorAddress, "exec@127.0.0.1:27008"},
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

		executor, sequencer, dac, keyProvider, p2pClient, ds := createTestComponents(context.Background())

		newRunNodeCmd := NewRunNodeCmd(executor, sequencer, dac, keyProvider, p2pClient, ds)

		if err := newRunNodeCmd.ParseFlags(args); err != nil {
			t.Errorf("Error: %v", err)
		}

		if err := parseConfig(newRunNodeCmd, "custom/root/dir"); err != nil {
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
	// Reset nodeConfig to default values
	nodeConfig = rollconf.DefaultNodeConfig

	// Create a new command without specifying any flags
	args := []string{"start"}
	executor, sequencer, dac, keyProvider, p2pClient, ds := createTestComponents(context.Background())

	newRunNodeCmd := NewRunNodeCmd(executor, sequencer, dac, keyProvider, p2pClient, ds)

	if err := newRunNodeCmd.ParseFlags(args); err != nil {
		t.Errorf("Error parsing flags: %v", err)
	}

	if err := parseConfig(newRunNodeCmd, "custom/root/dir"); err != nil {
		t.Errorf("Error parsing config: %v", err)
	}

	// Verify that Aggregator is true by default
	assert.True(t, nodeConfig.Node.Aggregator, "Expected Aggregator to be true by default")
}

// TestCentralizedAddresses verifies that when centralized service flags are provided,
// the configuration fields in nodeConfig are updated accordingly, ensuring that mocks are skipped.
func TestCentralizedAddresses(t *testing.T) {
	args := []string{
		"start",
		"--rollkit.da.address=http://central-da:26657",
		"--rollkit.node.sequencer_address=central-seq:26659",
		"--rollkit.node.sequencer_rollup_id=centralrollup",
	}

	executor, sequencer, dac, keyProvider, p2pClient, ds := createTestComponents(context.Background())

	cmd := NewRunNodeCmd(executor, sequencer, dac, keyProvider, p2pClient, ds)
	if err := cmd.ParseFlags(args); err != nil {
		t.Fatalf("ParseFlags error: %v", err)
	}
	if err := parseConfig(cmd, "custom/root/dir"); err != nil {
		t.Fatalf("parseConfig error: %v", err)
	}

	if nodeConfig.DA.Address != "http://central-da:26657" {
		t.Errorf("Expected nodeConfig.Rollkit.DAAddress to be 'http://central-da:26657', got '%s'", nodeConfig.DA.Address)
	}

	if nodeConfig.Node.SequencerAddress != "central-seq:26659" {
		t.Errorf("Expected nodeConfig.Rollkit.SequencerAddress to be 'central-seq:26659', got '%s'", nodeConfig.Node.SequencerAddress)
	}

	// Also confirm that the sequencer rollup id flag is marked as changed
	if !cmd.Flags().Lookup(rollconf.FlagSequencerRollupID).Changed {
		t.Error("Expected flag \"rollkit.sequencer_rollup_id\" to be marked as changed")
	}
}

func TestInitFiles(t *testing.T) {
	// Save the original nodeConfig
	origNodeConfig := nodeConfig

	// Create a temporary directory for the test
	tempDir, err := os.MkdirTemp("", "rollkit-test")
	assert.NoError(t, err)
	defer func() {
		err := os.RemoveAll(tempDir)
		assert.NoError(t, err)
	}()

	// Create the necessary subdirectories
	configDir := filepath.Join(tempDir, "config")
	dataDir := filepath.Join(tempDir, "data")
	err = os.MkdirAll(configDir, 0750)
	require.NoError(t, err)
	err = os.MkdirAll(dataDir, 0750)
	assert.NoError(t, err)

	// Set up test configuration
	nodeConfig = rollconf.DefaultNodeConfig
	nodeConfig.RootDir = tempDir
	nodeConfig.ConfigDir = "config"
	nodeConfig.DBPath = "data"

	// Test initialization
	err = initConfigFiles()
	assert.NoError(t, err)

	// Verify that the necessary files were created
	privValKeyFile := filepath.Join(configDir, "priv_validator_key.json")
	privValStateFile := filepath.Join(dataDir, "priv_validator_state.json")
	genFile := filepath.Join(configDir, "genesis.json")

	assert.FileExists(t, privValKeyFile, "Private validator key file should exist")
	assert.FileExists(t, privValStateFile, "Private validator state file should exist")
	assert.FileExists(t, genFile, "Genesis file should exist")

	// Restore original configuration
	nodeConfig = origNodeConfig
}
