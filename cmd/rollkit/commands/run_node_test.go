package commands

import (
	"context"
	"crypto/rand"
	"encoding/binary"
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

	rollconf "github.com/rollkit/rollkit/pkg/config"
)

func TestParseFlags(t *testing.T) {
	// Initialize nodeConfig with default values to avoid issues with instrument
	nodeConfig = rollconf.DefaultNodeConfig

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

	newRunNodeCmd := NewRunNodeCmd()

	// Register root flags to be able to use --home flag
	registerFlagsRootCmd(newRunNodeCmd)

	if err := newRunNodeCmd.ParseFlags(args); err != nil {
		t.Errorf("Error: %v", err)
	}

	if err := parseConfig(newRunNodeCmd); err != nil {
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

		newRunNodeCmd := NewRunNodeCmd()

		if err := newRunNodeCmd.ParseFlags(args); err != nil {
			t.Errorf("Error: %v", err)
		}

		if err := parseConfig(newRunNodeCmd); err != nil {
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
	newRunNodeCmd := NewRunNodeCmd()

	if err := newRunNodeCmd.ParseFlags(args); err != nil {
		t.Errorf("Error parsing flags: %v", err)
	}

	if err := parseConfig(newRunNodeCmd); err != nil {
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

	cmd := NewRunNodeCmd()
	if err := cmd.ParseFlags(args); err != nil {
		t.Fatalf("ParseFlags error: %v", err)
	}
	if err := parseConfig(cmd); err != nil {
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

func TestStartMockSequencerServer(t *testing.T) {
	// Use a random base port to avoid conflicts
	var randomBytes [2]byte
	if _, err := rand.Read(randomBytes[:]); err != nil {
		t.Fatalf("Failed to generate random port: %v", err)
	}
	basePort := 60000 + int(binary.LittleEndian.Uint16(randomBytes[:]))%5000

	tests := []struct {
		name        string
		seqAddress  string
		expectedErr error
		setup       func() (*net.Listener, error)
		teardown    func(*net.Listener)
	}{
		{
			name:        "Success",
			seqAddress:  fmt.Sprintf("localhost:%d", basePort),
			expectedErr: nil,
		},
		{
			name:        "Invalid URL",
			seqAddress:  "://invalid",
			expectedErr: &net.OpError{},
		},
		{
			name:        "Server Already Running",
			seqAddress:  fmt.Sprintf("localhost:%d", basePort+1),
			expectedErr: errSequencerAlreadyRunning,
			setup: func() (*net.Listener, error) {
				// Start a TCP listener to simulate a running server
				l, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", basePort+1))
				if err != nil {
					return nil, err
				}
				// Keep the connection open
				go func() {
					for {
						conn, err := l.Accept()
						if err != nil {
							return
						}
						if err := conn.Close(); err != nil {
							t.Errorf("Failed to close connection: %v", err)
							return
						}
					}
				}()
				return &l, nil
			},
			teardown: func(l *net.Listener) {
				if l != nil {
					if err := (*l).Close(); err != nil {
						t.Errorf("Failed to close listener: %v", err)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var listener *net.Listener
			if tt.setup != nil {
				var err error
				listener, err = tt.setup()
				if err != nil {
					t.Fatal(err)
				}
			}

			if tt.teardown != nil && listener != nil {
				defer tt.teardown(listener)
			}

			srv, err := tryStartMockSequencerServerGRPC(tt.seqAddress, "test-rollup-id")
			if srv != nil {
				srv.Stop()
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
	err = os.MkdirAll(configDir, rollconf.DefaultDirPerm)
	assert.NoError(t, err)
	err = os.MkdirAll(dataDir, rollconf.DefaultDirPerm)
	assert.NoError(t, err)

	// Set the nodeConfig to use the temporary directory
	nodeConfig = rollconf.Config{
		RootDir:   tempDir,
		ConfigDir: "config",
		DBPath:    "data",
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
		filepath.Join(tempDir, "config", "genesis.json"),
	}

	for _, file := range files {
		assert.FileExists(t, file)
	}
}

// TestKVExecutorHTTPServerShutdown tests that the KVExecutor HTTP server properly
// shuts down when the context is cancelled
func TestKVExecutorHTTPServerShutdown(t *testing.T) {
	// Create a temporary directory for test
	tempDir, err := os.MkdirTemp("", "kvexecutor-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Logf("Failed to remove temp dir: %v", err)
		}
	}()

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
		t.Logf("Failed to close response body: %v", err)
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
