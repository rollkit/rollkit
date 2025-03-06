package commands

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/rollkit/go-da"
	proxy "github.com/rollkit/go-da/proxy/jsonrpc"

	rollconf "github.com/rollkit/rollkit/config"
)

func TestParseFlags(t *testing.T) {
	// Initialize nodeConfig with default values to avoid issues with instrument
	nodeConfig = rollconf.DefaultNodeConfig

	flags := []string{
		// P2P flags
		"--p2p.listen_address", "tcp://127.0.0.1:27000",
		"--p2p.seeds", "node1@127.0.0.1:27001,node2@127.0.0.1:27002",
		"--p2p.blocked_peers", "node3@127.0.0.1:27003,node4@127.0.0.1:27004",
		"--p2p.allowed_peers", "node5@127.0.0.1:27005,node6@127.0.0.1:27006",

		// Rollkit flags
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
		"--rollkit.trusted_hash", "abcdef1234567890",
		"--rollkit.db_path", "custom/db/path",
		"--rollkit.sequencer_address", "seq@127.0.0.1:27007",
		"--rollkit.sequencer_rollup_id", "test-rollup",
		"--rollkit.executor_address", "exec@127.0.0.1:27008",
		"--rollkit.da_submit_options", "custom-options",

		"--instrumentation.prometheus", "true",
		"--instrumentation.prometheus_listen_addr", ":26665",
		"--instrumentation.max_open_connections", "1",
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
		// P2P fields
		{"ListenAddress", nodeConfig.P2P.ListenAddress, "tcp://127.0.0.1:27000"},
		{"Seeds", nodeConfig.P2P.Seeds, "node1@127.0.0.1:27001,node2@127.0.0.1:27002"},
		{"BlockedPeers", nodeConfig.P2P.BlockedPeers, "node3@127.0.0.1:27003,node4@127.0.0.1:27004"},
		{"AllowedPeers", nodeConfig.P2P.AllowedPeers, "node5@127.0.0.1:27005,node6@127.0.0.1:27006"},

		// Rollkit fields
		{"Aggregator", nodeConfig.Rollkit.Aggregator, false},
		{"BlockTime", nodeConfig.Rollkit.BlockTime, 2 * time.Second},
		{"DAAddress", nodeConfig.Rollkit.DAAddress, "http://127.0.0.1:27005"},
		{"DAAuthToken", nodeConfig.Rollkit.DAAuthToken, "token"},
		{"DABlockTime", nodeConfig.Rollkit.DABlockTime, 20 * time.Second},
		{"DAGasMultiplier", nodeConfig.Rollkit.DAGasMultiplier, 1.5},
		{"DAGasPrice", nodeConfig.Rollkit.DAGasPrice, 1.5},
		{"DAMempoolTTL", nodeConfig.Rollkit.DAMempoolTTL, uint64(10)},
		{"DANamespace", nodeConfig.Rollkit.DANamespace, "namespace"},
		{"DAStartHeight", nodeConfig.Rollkit.DAStartHeight, uint64(100)},
		{"LazyAggregator", nodeConfig.Rollkit.LazyAggregator, true},
		{"LazyBlockTime", nodeConfig.Rollkit.LazyBlockTime, 2 * time.Minute},
		{"Light", nodeConfig.Rollkit.Light, true},
		{"MaxPendingBlocks", nodeConfig.Rollkit.MaxPendingBlocks, uint64(100)},
		{"TrustedHash", nodeConfig.Rollkit.TrustedHash, "abcdef1234567890"},
		{"DBPath", nodeConfig.Rollkit.DBPath, "custom/db/path"},
		{"SequencerAddress", nodeConfig.Rollkit.SequencerAddress, "seq@127.0.0.1:27007"},
		{"SequencerRollupID", nodeConfig.Rollkit.SequencerRollupID, "test-rollup"},
		{"ExecutorAddress", nodeConfig.Rollkit.ExecutorAddress, "exec@127.0.0.1:27008"},
		{"DASubmitOptions", nodeConfig.Rollkit.DASubmitOptions, "custom-options"},

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

		if nodeConfig.Rollkit.Aggregator != validValues[i] {
			t.Errorf("Expected %v, got %v", validValues[i], nodeConfig.Rollkit.Aggregator)
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

	if nodeConfig.Rollkit.DAAddress != "http://central-da:26657" {
		t.Errorf("Expected nodeConfig.Rollkit.DAAddress to be 'http://central-da:26657', got '%s'", nodeConfig.Rollkit.DAAddress)
	}

	if nodeConfig.Rollkit.SequencerAddress != "central-seq:26659" {
		t.Errorf("Expected nodeConfig.Rollkit.SequencerAddress to be 'central-seq:26659', got '%s'", nodeConfig.Rollkit.SequencerAddress)
	}

	// Also confirm that the sequencer rollup id flag is marked as changed
	if !cmd.Flags().Lookup(rollconf.FlagSequencerRollupID).Changed {
		t.Error("Expected flag \"rollkit.sequencer_rollup_id\" to be marked as changed")
	}
}

// MockServer wraps proxy.Server to allow us to control its behavior in tests
type MockServer struct {
	*proxy.Server
	StartFunc func(context.Context) error
	StopFunc  func(context.Context) error
}

func (m *MockServer) Start(ctx context.Context) error {
	if m.StartFunc != nil {
		return m.StartFunc(ctx)
	}
	return m.Server.Start(ctx)
}

func (m *MockServer) Stop(ctx context.Context) error {
	if m.StopFunc != nil {
		return m.StopFunc(ctx)
	}
	return m.Server.Stop(ctx)
}

func TestStartMockDAServJSONRPC(t *testing.T) {
	tests := []struct {
		name          string
		daAddress     string
		mockServerErr error
		expectedErr   error
	}{
		{
			name:          "Success",
			daAddress:     "http://localhost:26657",
			mockServerErr: nil,
			expectedErr:   nil,
		},
		{
			name:          "Invalid URL",
			daAddress:     "://invalid",
			mockServerErr: nil,
			expectedErr:   &url.Error{},
		},
		{
			name:          "Server Already Running",
			daAddress:     "http://localhost:26657",
			mockServerErr: syscall.EADDRINUSE,
			expectedErr:   errDAServerAlreadyRunning,
		},
		{
			name:          "Other Server Error",
			daAddress:     "http://localhost:26657",
			mockServerErr: errors.New("other error"),
			expectedErr:   errors.New("other error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newServerFunc := func(hostname, port string, da da.DA) *proxy.Server {
				mockServer := &MockServer{
					Server: proxy.NewServer(hostname, port, da),
				}
				return mockServer.Server
			}

			srv, err := tryStartMockDAServJSONRPC(context.Background(), tt.daAddress, newServerFunc)

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

func TestStartMockExecutorServerGRPC(t *testing.T) {
	tests := []struct {
		name        string
		execAddress string
		expectedErr error
	}{
		{
			name:        "Success",
			execAddress: "localhost:50052",
			expectedErr: nil,
		},
		{
			name:        "Invalid URL",
			execAddress: "://invalid",
			expectedErr: &net.OpError{},
		},
		{
			name:        "Server Already Running",
			execAddress: "localhost:50052",
			expectedErr: errExecutorAlreadyRunning,
		},
	}

	stopFns := []func(){}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, err := tryStartMockExecutorServerGRPC(tt.execAddress)
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
