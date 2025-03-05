package node

import (
	"context"
	"errors"
	"net"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"cosmossdk.io/log"
	cmcrypto "github.com/cometbft/cometbft/crypto"
	"github.com/stretchr/testify/require"
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

	rollkitconfig "github.com/rollkit/rollkit/config"
	coreexecutor "github.com/rollkit/rollkit/core/execution"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	"github.com/rollkit/rollkit/types"
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

	execSrv := startMockExecutorServerGRPC(MockExecutorAddress)
	if execSrv == nil {
		os.Exit(1)
	}

	exitCode := m.Run()

	// teardown servers
	daSrv.GracefulStop()
	execSrv.GracefulStop()

	os.Exit(exitCode)
}

func startMockDAGRPCServ() *grpc.Server {
	srv := goDAproxy.NewServer(goDATest.NewDummyDA(), grpc.Creds(insecure.NewCredentials()))
	addr, err := url.Parse(MockDAAddress)
	if err != nil {
		panic(err)
	}
	lis, err := net.Listen("tcp", addr.Host)
	if err != nil {
		panic(err)
	}
	go func() {
		err = srv.Serve(lis)
		if err != nil {
			panic(err)
		}
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
	_, _, err := dummyExec.InitChain(context.Background(), time.Now(), 1, "test-chain")
	if err != nil {
		panic(err)
	}

	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		i := 0
		for range ticker.C {
			dummyExec.InjectTx(execTypes.Tx{byte(3*i + 1), byte(3*i + 2), byte(3*i + 3)})
			i++
		}
	}()

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

// NodeRunner contains a node and its running context
type NodeRunner struct {
	Node      Node
	Ctx       context.Context
	Cancel    context.CancelFunc
	ErrCh     chan error
	WaitGroup *sync.WaitGroup
}

// startNodeWithCleanup starts the node using the service pattern and registers cleanup
func startNodeWithCleanup(t *testing.T, node Node) *NodeRunner {
	// Create a cancellable context
	ctx, cancel := context.WithCancel(context.Background())

	// Create error channel and wait group
	errCh := make(chan error, 1)
	var wg sync.WaitGroup

	// Start the node in a goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := node.Run(ctx)
		select {
		case errCh <- err:
		default:
			t.Logf("Error channel full, discarding error: %v", err)
		}
	}()

	// Give the node time to initialize
	time.Sleep(100 * time.Millisecond)

	// Check if the node has stopped unexpectedly
	select {
	case err := <-errCh:
		t.Fatalf("Node stopped unexpectedly with error: %v", err)
	default:
		// This is expected - node is still running
	}

	// Register cleanup function
	t.Cleanup(func() {
		cleanUpNode(ctx, cancel, &wg, errCh, t)
	})

	return &NodeRunner{
		Node:      node,
		Ctx:       ctx,
		Cancel:    cancel,
		ErrCh:     errCh,
		WaitGroup: &wg,
	}
}

// cleanUpNode stops the node using context cancellation
func cleanUpNode(ctx context.Context, cancel context.CancelFunc, wg *sync.WaitGroup, errCh chan error, t *testing.T) {
	// Cancel the context to stop the node
	cancel()

	// Wait for the node to stop with a timeout
	waitCh := make(chan struct{})
	go func() {
		wg.Wait()
		close(waitCh)
	}()

	select {
	case <-waitCh:
		// Node stopped successfully
	case <-time.After(5 * time.Second):
		t.Log("Warning: Node did not stop gracefully within timeout")
	}

	// Check for any errors during shutdown
	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Logf("Error stopping node: %v", err)
		}
	default:
		// No error
	}
}

// initAndStartNodeWithCleanup initializes and starts a node of the specified type.
func initAndStartNodeWithCleanup(ctx context.Context, t *testing.T, nodeType NodeType, chainID string) Node {
	node, _ := setupTestNode(ctx, t, nodeType, chainID)
	runner := startNodeWithCleanup(t, node)

	return runner.Node
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
	config := rollkitconfig.NodeConfig{
		DAAddress:        MockDAAddress,
		DANamespace:      MockDANamespace,
		ExecutorAddress:  MockExecutorAddress,
		SequencerAddress: MockSequencerAddress,
		Light:            nodeType == Light,
	}

	genesis, genesisValidatorKey := types.GetGenesisWithPrivkey(types.DefaultSigningKeyType, chainID)
	signingKey, err := types.PrivKeyToSigningKey(genesisValidatorKey)
	if err != nil {
		return nil, nil, err
	}

	key := generateSingleKey()

	dummyExec := coreexecutor.NewDummyExecutor()
	dummySequencer := coresequencer.NewDummySequencer()

	logger := log.NewTestLogger(t)
	node, err := NewNode(
		ctx,
		config,
		dummyExec,
		dummySequencer,
		key,
		signingKey,
		genesis,
		DefaultMetricsProvider(rollkitconfig.DefaultInstrumentationConfig()),
		logger,
	)
	return node, genesisValidatorKey, err
}

func TestNewNode(t *testing.T) {
	ctx := context.Background()
	chainID := "TestNewNode"
	//ln := initAndStartNodeWithCleanup(ctx, t, Light, chainID)
	//require.IsType(t, new(LightNode), ln)
	fn := initAndStartNodeWithCleanup(ctx, t, Full, chainID)
	require.IsType(t, new(FullNode), fn)
}
