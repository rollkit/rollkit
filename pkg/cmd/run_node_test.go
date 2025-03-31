package cmd

import (
	"context"
	"os"
	"reflect"
	"testing"
	"time"

	"cosmossdk.io/log"
	"github.com/stretchr/testify/assert"

	coreda "github.com/rollkit/rollkit/core/da"
	coreexecutor "github.com/rollkit/rollkit/core/execution"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	"github.com/rollkit/rollkit/da"
	rollconf "github.com/rollkit/rollkit/pkg/config"
	testExecutor "github.com/rollkit/rollkit/rollups/testapp/kv"
)

func createTestComponents(ctx context.Context) (coreexecutor.Executor, coresequencer.Sequencer, coreda.Client) {
	executor := testExecutor.CreateDirectKVExecutor(ctx)
	sequencer := coresequencer.NewDummySequencer()
	dummyDA := coreda.NewDummyDA(100_000, 0, 0)
	logger := log.NewLogger(os.Stdout)
	dac := da.NewDAClient(dummyDA, 0, 1.0, []byte("test"), []byte(""), logger)

	return executor, sequencer, dac
}

func TestParseFlags(t *testing.T) {
	// Initialize nodeConfig with default values to avoid issues with instrument
	nodeConfig := rollconf.DefaultNodeConfig

	flags := []string{
		"--home", "custom/root/dir",
		"--db_path", "custom/db/path",

		// P2P flags
		"--p2p.listen_address", "tcp://127.0.0.1:27000",
		"--p2p.seeds", "node1@127.0.0.1:27001,node2@127.0.0.1:27002",
		"--p2p.blocked_peers", "node3@127.0.0.1:27003,node4@127.0.0.1:27004",
		"--p2p.allowed_peers", "node5@127.0.0.1:27005,node6@127.0.0.1:27006",

		// Node flags
		"--node.aggregator=false",
		"--node.block_time", "2s",
		"--da.address", "http://127.0.0.1:27005",
		"--da.auth_token", "token",
		"--da.block_time", "20s",
		"--da.gas_multiplier", "1.5",
		"--da.gas_price", "1.5",
		"--da.mempool_ttl", "10",
		"--da.namespace", "namespace",
		"--da.start_height", "100",
		"--node.lazy_aggregator",
		"--node.lazy_block_time", "2m",
		"--node.light",
		"--node.max_pending_blocks", "100",
		"--node.trusted_hash", "abcdef1234567890",
		"--node.sequencer_address", "seq@127.0.0.1:27007",
		"--node.sequencer_rollup_id", "test-rollup",
		"--node.executor_address", "exec@127.0.0.1:27008",
		"--da.submit_options", "custom-options",

		// Instrumentation flags
		"--instrumentation.prometheus", "true",
		"--instrumentation.prometheus_listen_addr", ":26665",
		"--instrumentation.max_open_connections", "1",
	}

	args := append([]string{"start"}, flags...)

	executor, sequencer, dac := createTestComponents(context.Background())

	newRunNodeCmd := NewRunNodeCmd(executor, sequencer, dac)

	// Register root flags to be able to use --home flag
	rollconf.AddBasicFlags(newRunNodeCmd, "testapp")

	if err := newRunNodeCmd.ParseFlags(args); err != nil {
		t.Errorf("Error: %v", err)
	}

	nodeConfig, err := parseConfig(newRunNodeCmd)
	if err != nil {
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
		"--node.aggregator=false",
	}, {
		"--node.aggregator=true",
	}, {
		"--node.aggregator",
	}}

	validValues := []bool{false, true, true}

	for i, flags := range flagVariants {
		args := append([]string{"start"}, flags...)

		executor, sequencer, dac := createTestComponents(context.Background())

		newRunNodeCmd := NewRunNodeCmd(executor, sequencer, dac)

		if err := newRunNodeCmd.ParseFlags(args); err != nil {
			t.Errorf("Error: %v", err)
		}

		nodeConfig, err := parseConfig(newRunNodeCmd)
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
	// Reset nodeConfig to default values
	nodeConfig := rollconf.DefaultNodeConfig

	// Create a new command without specifying any flags
	args := []string{"start"}
	executor, sequencer, dac := createTestComponents(context.Background())

	newRunNodeCmd := NewRunNodeCmd(executor, sequencer, dac)

	if err := newRunNodeCmd.ParseFlags(args); err != nil {
		t.Errorf("Error parsing flags: %v", err)
	}

	nodeConfig, err := parseConfig(newRunNodeCmd)
	if err != nil {
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
		"--da.address=http://central-da:26657",
		"--node.sequencer_address=central-seq:26659",
		"--node.sequencer_rollup_id=centralrollup",
	}

	executor, sequencer, dac := createTestComponents(context.Background())

	cmd := NewRunNodeCmd(executor, sequencer, dac)
	if err := cmd.ParseFlags(args); err != nil {
		t.Fatalf("ParseFlags error: %v", err)
	}
	nodeConfig, err := parseConfig(cmd)
	if err != nil {
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
