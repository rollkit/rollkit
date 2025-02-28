package node

import (
	"context"
	"testing"

	"cosmossdk.io/log"
	cmconfig "github.com/cometbft/cometbft/config"
	cmcrypto "github.com/cometbft/cometbft/crypto"
	ds "github.com/ipfs/go-datastore"
	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/config"
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

type NodeType int

const (
	Full NodeType = iota
	Light
)

// startNode starts the full node and stops it when the test is done
func startNodeWithCleanup(t *testing.T, node Node) {
	require.False(t, node.IsRunning())
	require.NoError(t, node.Start(t.Context()))
	require.True(t, node.IsRunning())
	t.Cleanup(func() {
		cleanUpNode(node, t)
	})
}

// cleanUpNode stops the node and checks if it is running
func cleanUpNode(node Node, t *testing.T) {
	// Attempt to stop the node
	err := node.Stop(t.Context())
	require.NoError(t, err)
	// Now verify that the node is no longer running
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
		Light:            nodeType == Light,
	}

	genesis, genesisValidatorKey := types.GetGenesisWithPrivkey(types.DefaultSigningKeyType, chainID)
	signingKey, err := types.PrivKeyToSigningKey(genesisValidatorKey)
	if err != nil {
		return nil, nil, err
	}

	key := generateSingleKey()

	dummyExec := NewDummyExecutor()
	dummySequencer := NewDummySequencer()
	database := ds.NewMapDatastore()

	logger := log.NewTestLogger(t)
	node, err := NewNode(
		ctx,
		config,
		dummyExec,
		dummySequencer,
		key,
		signingKey,
		genesis,
		database,
		DefaultMetricsProvider(cmconfig.DefaultInstrumentationConfig()),
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
