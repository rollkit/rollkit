package node

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"testing"

	cmconfig "github.com/cometbft/cometbft/config"
	cmcrypto "github.com/cometbft/cometbft/crypto"
	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/config"
	test "github.com/rollkit/rollkit/test/log"
	"github.com/rollkit/rollkit/types"

	"google.golang.org/grpc"

	"google.golang.org/grpc/credentials/insecure"

	goDAproxy "github.com/rollkit/go-da/proxy/grpc"
	goDATest "github.com/rollkit/go-da/test"
	execGRPC "github.com/rollkit/go-execution/proxy/grpc"
	execTest "github.com/rollkit/go-execution/test"
	execTypes "github.com/rollkit/go-execution/types"
	pb "github.com/rollkit/go-execution/types/pb/execution"
	seqGRPC "github.com/rollkit/go-sequencing/proxy/grpc"
	seqTest "github.com/rollkit/go-sequencing/test"
)

const (
	// MockDAAddress is the address used by the mock gRPC service
	// NOTE: this should be unique per test package to avoid
	// "bind: listen address already in use" because multiple packages
	// are tested in parallel
	MockDAAddress = "grpc://localhost:7990"

	// MockDANamespace is a sample namespace used by the mock DA client
	MockDANamespace = "00000000000000000000000000000000000000000000000000deadbeef"

	// MockSequencerAddress is a sample address used by the mock sequencer
	MockSequencerAddress = "127.0.0.1:50051"

	// MockExecutorAddress is a sample address used by the mock executor
	MockExecutorAddress = "127.0.0.1:40041"
)

// TestMain does setup and teardown on the test package
// to make the mock gRPC service available to the nodes
func TestMain(m *testing.M) {
	daSrv := startMockDAGRPCServ()
	if daSrv == nil {
		os.Exit(1)
	}

	seqSrv := startMockSequencerServerGRPC(MockSequencerAddress)
	if seqSrv == nil {
		os.Exit(1)
	}

	execSrv := startMockExecutorServerGRPC(MockExecutorAddress)
	if execSrv == nil {
		os.Exit(1)
	}

	exitCode := m.Run()

	// teardown servers
	daSrv.GracefulStop()
	seqSrv.GracefulStop()
	execSrv.GracefulStop()

	os.Exit(exitCode)
}

func startMockDAGRPCServ() *grpc.Server {
	srv := goDAproxy.NewServer(goDATest.NewDummyDA(), grpc.Creds(insecure.NewCredentials()))
	addr, _ := url.Parse(MockDAAddress)
	lis, err := net.Listen("tcp", addr.Host)
	if err != nil {
		panic(err)
	}
	go func() {
		_ = srv.Serve(lis)
	}()
	return srv
}

// startMockSequencerServerGRPC starts a mock gRPC server with the given listenAddress.
func startMockSequencerServerGRPC(listenAddress string) *grpc.Server {
	dummySeq := seqTest.NewMultiRollupSequencer()
	server := seqGRPC.NewServer(dummySeq, dummySeq, dummySeq)
	lis, err := net.Listen("tcp", listenAddress)
	if err != nil {
		panic(err)
	}
	go func() {
		_ = server.Serve(lis)
	}()
	return server
}

// startMockExecutorServerGRPC starts a mock gRPC server with the given listenAddress.
func startMockExecutorServerGRPC(listenAddress string) *grpc.Server {
	dummyExec := execTest.NewDummyExecutor()

	dummyExec.InjectTx(execTypes.Tx{1, 2, 3})
	dummyExec.InjectTx(execTypes.Tx{4, 5, 6})
	dummyExec.InjectTx(execTypes.Tx{7, 8, 9})

	execServer := execGRPC.NewServer(dummyExec, nil)
	server := grpc.NewServer()
	pb.RegisterExecutionServiceServer(server, execServer)
	lis, err := net.Listen("tcp", listenAddress)
	if err != nil {
		panic(err)
	}
	go func() {
		_ = server.Serve(lis)
	}()
	return server
}

type NodeType int

const (
	Full NodeType = iota
	Light
)

// startNode starts the full node and stops it when the test is done
func startNodeWithCleanup(t *testing.T, node Node) {
	require.False(t, node.IsRunning())
	require.NoError(t, node.Start())
	require.True(t, node.IsRunning())
	t.Cleanup(func() {
		cleanUpNode(node, t)
	})
}

// cleanUpNode stops the node and checks if it is running
func cleanUpNode(node Node, t *testing.T) {
	require.False(t, node.IsRunning())
}

// initAndStartNodeWithCleanup initializes and starts a node of the specified type.
func initAndStartNodeWithCleanup(ctx context.Context, t *testing.T, nodeType NodeType, chainID string) Node {
	node, _ := setupTestNode(ctx, t, nodeType, chainID)
	startNodeWithCleanup(t, node)

	return node
}

// setupTestNode sets up a test node based on the NodeType.
func setupTestNode(ctx context.Context, t *testing.T, nodeType NodeType, chainID string) (Node, cmcrypto.PrivKey) {
	node, privKey, err := newTestNode(ctx, t, nodeType, chainID)
	require.NoError(t, err)
	require.NotNil(t, node)

	return node, privKey
}

// newTestNode creates a new test node based on the NodeType.
func newTestNode(ctx context.Context, t *testing.T, nodeType NodeType, chainID string) (Node, cmcrypto.PrivKey, error) {
	config := config.NodeConfig{
		DAAddress:        MockDAAddress,
		DANamespace:      MockDANamespace,
		ExecutorAddress:  MockExecutorAddress,
		SequencerAddress: MockSequencerAddress,
	}
	switch nodeType {
	case Light:
		config.Light = true
	case Full:
		config.Light = false
	default:
		panic(fmt.Sprintf("invalid node type: %v", nodeType))
	}
	genesis, genesisValidatorKey := types.GetGenesisWithPrivkey(types.DefaultSigningKeyType, chainID)
	signingKey, err := types.PrivKeyToSigningKey(genesisValidatorKey)
	if err != nil {
		return nil, nil, err
	}

	key := generateSingleKey()

	logger := test.NewFileLogger(t)
	node, err := NewNode(ctx, config, key, signingKey, genesis, DefaultMetricsProvider(cmconfig.DefaultInstrumentationConfig()), logger)
	return node, genesisValidatorKey, err
}

func TestNewNode(t *testing.T) {
	ctx := context.Background()
	chainID := "TestNewNode"
	ln := initAndStartNodeWithCleanup(ctx, t, Light, chainID)
	require.IsType(t, new(LightNode), ln)
	fn := initAndStartNodeWithCleanup(ctx, t, Full, chainID)
	require.IsType(t, new(FullNode), fn)
}
