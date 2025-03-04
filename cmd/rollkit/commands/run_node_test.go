package commands

import (
	"context"
	"errors"
	"net"
	"net/url"
	"reflect"
	"syscall"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"

	"github.com/rollkit/go-da"
	proxy "github.com/rollkit/go-da/proxy/jsonrpc"

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
		"--p2p.external-address", "127.0.0.0:26000",
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
		{"ExternalAddress", viper.GetString("p2p.external-address"), "127.0.0.0:26000"},
		{"ListenAddress", viper.GetString("p2p.laddr"), "tcp://127.0.0.1:27000"},
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
