package node

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"cosmossdk.io/log"
	"github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	seqGRPC "github.com/rollkit/go-sequencing/proxy/grpc"
	seqTest "github.com/rollkit/go-sequencing/test"

	coreda "github.com/rollkit/rollkit/core/da"
	coreexecutor "github.com/rollkit/rollkit/core/execution"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	rollkitconfig "github.com/rollkit/rollkit/pkg/config"
	"github.com/rollkit/rollkit/pkg/p2p"
	"github.com/rollkit/rollkit/pkg/p2p/key"
	remote_signer "github.com/rollkit/rollkit/pkg/signer/noop"
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

// createTestComponents creates test components for node initialization
func createTestComponents(t *testing.T) (coreexecutor.Executor, coresequencer.Sequencer, coreda.Client, *p2p.Client, datastore.Batching) {
	executor := coreexecutor.NewDummyExecutor()
	sequencer := coresequencer.NewDummySequencer()
	dummyDA := coreda.NewDummyDA(100_000, 0, 0)
	dummyClient := coreda.NewDummyClient(dummyDA, []byte(MockDANamespace))

	// Create genesis and keys for P2P client
	_, genesisValidatorKey, _ := types.GetGenesisWithPrivkey("test-chain")
	nodeKey := &key.NodeKey{
		PrivKey: genesisValidatorKey,
		PubKey:  genesisValidatorKey.GetPublic(),
	}
	p2pClient, err := p2p.NewClient(rollkitconfig.DefaultConfig, nodeKey, dssync.MutexWrap(datastore.NewMapDatastore()), log.NewNopLogger(), p2p.NopMetrics())
	require.NoError(t, err)
	require.NotNil(t, p2pClient)
	ds := dssync.MutexWrap(datastore.NewMapDatastore())

	return executor, sequencer, dummyClient, p2pClient, ds
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
		cleanUpNode(cancel, &wg, errCh, t)
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
func cleanUpNode(cancel context.CancelFunc, wg *sync.WaitGroup, errCh chan error, t *testing.T) {
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

// newTestNode creates a new test node based on the NodeType.
func newTestNode(ctx context.Context, t *testing.T, nodeType NodeType, chainID string) (Node, crypto.PrivKey, error) {
	config := rollkitconfig.Config{
		RootDir: t.TempDir(),
		Node: rollkitconfig.NodeConfig{
			Light: nodeType == Light,
		},
		DA: rollkitconfig.DAConfig{
			Address:   MockDAAddress,
			Namespace: MockDANamespace,
		},
	}

	genesis, genesisValidatorKey, _ := types.GetGenesisWithPrivkey(chainID)
	remoteSigner, err := remote_signer.NewNoopSigner(genesisValidatorKey)
	require.NoError(t, err)

	executor, sequencer, dac, p2pClient, ds := createTestComponents(t)

	err = InitFiles(config.RootDir)
	require.NoError(t, err)

	nodeKey, err := key.GenerateNodeKey()
	require.NoError(t, err)

	logger := log.NewTestLogger(t)

	node, err := NewNode(
		ctx,
		config,
		executor,
		sequencer,
		dac,
		remoteSigner,
		*nodeKey,
		p2pClient,
		genesis,
		ds,
		DefaultMetricsProvider(rollkitconfig.DefaultInstrumentationConfig()),
		logger,
	)
	return node, genesisValidatorKey, err
}

func TestNewNode(t *testing.T) {
	ctx := context.Background()
	chainID := "TestNewNode"

	// ln := initAndStartNodeWithCleanup(ctx, t, Light, chainID)
	// require.IsType(t, new(LightNode), ln)
	node, _, err := newTestNode(ctx, t, Full, chainID)
	require.NoError(t, err)
	require.NotNil(t, node)
	runner := startNodeWithCleanup(t, node)
	require.IsType(t, new(FullNode), runner.Node)
}
