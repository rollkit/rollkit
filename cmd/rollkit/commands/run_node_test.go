package commands

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"

	rollconf "github.com/rollkit/rollkit/config"
)

func TestParseFlags(t *testing.T) {
	flags := []string{
		"--consensus.create_empty_blocks", "true",
		"--consensus.create_empty_blocks_interval", "10s",
		"--consensus.double_sign_check_height", "10",
		"--db_backend", "cleverdb",
		"--db_dir", "data2",
		"--moniker", "yarik-playground2",
		"--p2p.laddr", "tcp://127.0.0.1:27000",
		"--p2p.pex",
		"--p2p.private_peer_ids", "1,2,3",
		"--p2p.seed_mode",
		"--p2p.unconditional_peer_ids", "4,5,6",
		"--priv_validator_laddr", "tcp://127.0.0.1:27003",
		"--rollkit.aggregator=false",
		"--rollkit.block_time", "2s",
		"--rollkit.da_address", "http://127.0.0.1:27005",
		"--rollkit.da_auth_token", "token",
		"--rollkit.da_block_time", "20s",
		"--rollkit.da_gas_multiplier", "1.5",
		"--rollkit.da_gas_price", "1.5",
		"--rollkit.da_mempool_ttl", "10",
		"--rollkit.da_namespace", "namespace",
		"--rollkit.da_start_height", "100",
		"--rollkit.lazy_aggregator",
		"--rollkit.lazy_block_time", "2m",
		"--rollkit.light",
		"--rollkit.max_pending_blocks", "100",
		"--rpc.grpc_laddr", "tcp://127.0.0.1:27006",
		"--rpc.laddr", "tcp://127.0.0.1:27007",
		"--rpc.pprof_laddr", "tcp://127.0.0.1:27008",
		"--rpc.unsafe",
	}

	args := append([]string{"start"}, flags...)

	newRunNodeCmd := NewRunNodeCmd()

	if err := newRunNodeCmd.ParseFlags(args); err != nil {
		t.Errorf("Error: %v", err)
	}

	if err := parseFlags(newRunNodeCmd); err != nil {
		t.Errorf("Error: %v", err)
	}

	testCases := []struct {
		name     string
		got      interface{}
		expected interface{}
	}{
		// CometBFT fields, available in viper but not in nodeConfig
		// TODO: decide if we want to add them to nodeConfig
		{"CreateEmptyBlocks", viper.GetBool("consensus.create_empty_blocks"), true},
		{"CreateEmptyBlocksInterval", viper.GetDuration("consensus.create_empty_blocks_interval"), 10 * time.Second},
		{"DoubleSignCheckHeight", viper.GetInt64("consensus.double_sign_check_height"), int64(10)},
		{"DBBackend", viper.GetString("db_backend"), "cleverdb"},
		{"DBDir", viper.GetString("db_dir"), "data2"},
		{"Moniker", viper.GetString("moniker"), "yarik-playground2"},
		{"PexReactor", viper.GetBool("p2p.pex"), true},
		{"PrivatePeerIDs", viper.GetString("p2p.private_peer_ids"), "1,2,3"},
		{"SeedMode", viper.GetBool("p2p.seed_mode"), true},
		{"UnconditionalPeerIDs", viper.GetString("p2p.unconditional_peer_ids"), "4,5,6"},
		{"PrivValidatorListenAddr", viper.GetString("priv_validator_laddr"), "tcp://127.0.0.1:27003"},

		// Rollkit fields
		{"Aggregator", nodeConfig.Aggregator, false},
		{"BlockTime", nodeConfig.BlockManagerConfig.BlockTime, 2 * time.Second},
		{"DAAddress", nodeConfig.DAAddress, "http://127.0.0.1:27005"},
		{"DAAuthToken", nodeConfig.DAAuthToken, "token"},
		{"DABlockTime", nodeConfig.BlockManagerConfig.DABlockTime, 20 * time.Second},
		{"DAGasMultiplier", nodeConfig.DAGasMultiplier, 1.5},
		{"DAGasPrice", nodeConfig.DAGasPrice, 1.5},
		{"DAMempoolTTL", nodeConfig.BlockManagerConfig.DAMempoolTTL, uint64(10)},
		{"DANamespace", nodeConfig.DANamespace, "namespace"},
		{"DAStartHeight", nodeConfig.BlockManagerConfig.DAStartHeight, uint64(100)},
		{"LazyAggregator", nodeConfig.BlockManagerConfig.LazyAggregator, true},
		{"LazyBlockTime", nodeConfig.BlockManagerConfig.LazyBlockTime, 2 * time.Minute},
		{"Light", nodeConfig.Light, true},
		{"ListenAddress", nodeConfig.P2P.ListenAddress, "tcp://127.0.0.1:27000"},
		{"MaxPendingBlocks", nodeConfig.BlockManagerConfig.MaxPendingBlocks, uint64(100)},

		// RPC fields, available in viper but not in nodeConfig
		// TODO: decide if we want to add them to nodeConfig
		{"GRPCListenAddress", viper.GetString("rpc.grpc_laddr"), "tcp://127.0.0.1:27006"},
		{"RPCListenAddress", viper.GetString("rpc.laddr"), "tcp://127.0.0.1:27007"},
		{"PprofListenAddress", viper.GetString("rpc.pprof_laddr"), "tcp://127.0.0.1:27008"},
		{"Unsafe", viper.GetBool("rpc.unsafe"), true},
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
		"--rollkit.aggregator=false",
	}, {
		"--rollkit.aggregator=true",
	}, {
		"--rollkit.aggregator",
	}}

	validValues := []bool{false, true, true}

	for i, flags := range flagVariants {
		args := append([]string{"start"}, flags...)

		newRunNodeCmd := NewRunNodeCmd()

		if err := newRunNodeCmd.ParseFlags(args); err != nil {
			t.Errorf("Error: %v", err)
		}

		if err := parseFlags(newRunNodeCmd); err != nil {
			t.Errorf("Error: %v", err)
		}

		if nodeConfig.Aggregator != validValues[i] {
			t.Errorf("Expected %v, got %v", validValues[i], nodeConfig.Aggregator)
		}
	}
}

// TestCentralizedAddresses verifies that when centralized service flags are provided,
// the configuration fields in nodeConfig are updated accordingly, ensuring that mocks are skipped.
func TestCentralizedAddresses(t *testing.T) {
	args := []string{
		"start",
		"--rollkit.da_address=http://central-da:26657",
		"--rollkit.sequencer_address=central-seq:26659",
		"--rollkit.sequencer_rollup_id=centralrollup",
	}

	cmd := NewRunNodeCmd()
	if err := cmd.ParseFlags(args); err != nil {
		t.Fatalf("ParseFlags error: %v", err)
	}
	if err := parseFlags(cmd); err != nil {
		t.Fatalf("parseFlags error: %v", err)
	}

	if nodeConfig.DAAddress != "http://central-da:26657" {
		t.Errorf("Expected nodeConfig.DAAddress to be 'http://central-da:26657', got '%s'", nodeConfig.DAAddress)
	}

	if nodeConfig.SequencerAddress != "central-seq:26659" {
		t.Errorf("Expected nodeConfig.SequencerAddress to be 'central-seq:26659', got '%s'", nodeConfig.SequencerAddress)
	}

	// Also confirm that the sequencer rollup id flag is marked as changed
	if !cmd.Flags().Lookup(rollconf.FlagSequencerRollupID).Changed {
		t.Error("Expected flag \"rollkit.sequencer_rollup_id\" to be marked as changed")
	}
}

func TestStartMockSequencerServer(t *testing.T) {
	tests := []struct {
		name        string
		seqAddress  string
		expectedErr error
	}{
		{
			name:        "Success",
			seqAddress:  "localhost:50051",
			expectedErr: nil,
		},
		{
			name:        "Invalid URL",
			seqAddress:  "://invalid",
			expectedErr: &net.OpError{},
		},
		{
			name:        "Server Already Running",
			seqAddress:  "localhost:50051",
			expectedErr: errSequencerAlreadyRunning,
		},
		{
			name:        "Other Server Error",
			seqAddress:  "localhost:50051",
			expectedErr: errors.New("other error"),
		},
	}

	stopFns := []func(){}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, err := tryStartMockSequencerServerGRPC(tt.seqAddress, "test-rollup-id")
			if srv != nil {
				stopFns = append(stopFns, func() {
					srv.Stop()
				})
			}

			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.IsType(t, tt.expectedErr, err)
				assert.Nil(t, srv)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, srv)
			}
		})
	}

	for _, fn := range stopFns {
		fn()
	}
}

func TestRollkitGenesisDocProviderFunc(t *testing.T) {
	// Create a temporary directory for the test
	tempDir, err := os.MkdirTemp("", "rollkit-test")
	assert.NoError(t, err)
	defer func() {
		err := os.RemoveAll(tempDir)
		assert.NoError(t, err)
	}()

	// Create the config directory
	configDir := filepath.Join(tempDir, "config")
	err = os.MkdirAll(configDir, 0750)
	assert.NoError(t, err)

	// Create a simple test genesis file
	testChainID := "test-chain-id"
	genFileContent := fmt.Sprintf(`{
		"chain_id": "%s",
		"genesis_time": "2023-01-01T00:00:00Z",
		"consensus_params": {
			"block": {
				"max_bytes": "22020096",
				"max_gas": "-1"
			},
			"evidence": {
				"max_age_num_blocks": "100000",
				"max_age_duration": "172800000000000"
			},
			"validator": {
				"pub_key_types": ["ed25519"]
			}
		}
	}`, testChainID)

	genFile := filepath.Join(configDir, "genesis.json")
	err = os.WriteFile(genFile, []byte(genFileContent), 0600)
	assert.NoError(t, err)

	// Create a test node config
	testNodeConfig := rollconf.NodeConfig{
		RootDir: tempDir,
	}

	// Get the genesis doc provider function
	genDocProvider := RollkitGenesisDocProviderFunc(testNodeConfig)
	assert.NotNil(t, genDocProvider)

	// Call the provider function and verify the result
	loadedGenDoc, err := genDocProvider()
	assert.NoError(t, err)
	assert.NotNil(t, loadedGenDoc)
	assert.Equal(t, testChainID, loadedGenDoc.ChainID)
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
	assert.NoError(t, err)
	err = os.MkdirAll(dataDir, 0750)
	assert.NoError(t, err)

	// Set the nodeConfig to use the temporary directory
	nodeConfig = rollconf.NodeConfig{
		RootDir: tempDir,
	}

	// Restore the original nodeConfig when the test completes
	defer func() {
		nodeConfig = origNodeConfig
	}()

	// Call initFiles
	err = initFiles()
	assert.NoError(t, err)

	// Verify that the expected files were created
	files := []string{
		filepath.Join(tempDir, "config", "priv_validator_key.json"),
		filepath.Join(tempDir, "data", "priv_validator_state.json"),
		filepath.Join(tempDir, "config", "node_key.json"),
		filepath.Join(tempDir, "config", "genesis.json"),
	}

	for _, file := range files {
		assert.FileExists(t, file)
	}
}

// TestKVExecutorHTTPServerShutdown tests that the KVExecutor HTTP server properly
// shuts down when the context is cancelled
func TestKVExecutorHTTPServerShutdown(t *testing.T) {

	// Find an available port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to find available port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("Failed to close listener: %v", err)
	}

	// Set up the KV executor HTTP address
	httpAddr := fmt.Sprintf("127.0.0.1:%d", port)
	viper.Set("kv-executor-http", httpAddr)
	defer viper.Set("kv-executor-http", "") // Reset after test

	// Create a context with cancel function
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create the KV executor with the context
	kvExecutor := createDirectKVExecutor(ctx)
	if kvExecutor == nil {
		t.Fatal("Failed to create KV executor")
	}

	// Wait a moment for the server to start
	time.Sleep(300 * time.Millisecond)

	// Send a request to confirm it's running
	client := &http.Client{Timeout: 1 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://%s/store", httpAddr))
	if err != nil {
		t.Fatalf("Failed to connect to server: %v", err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatalf("Failed to close response body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	// Cancel the context to shut down the server
	cancel()

	// Wait for shutdown to complete
	time.Sleep(300 * time.Millisecond)

	// Verify server is actually shutdown by attempting a new connection
	_, err = client.Get(fmt.Sprintf("http://%s/store", httpAddr))
	if err == nil {
		t.Fatal("Expected connection error after shutdown, but got none")
	}
}
