package node

import (
	"context"
	"fmt"
	"net"
	"os"
	"testing"

	cmconfig "github.com/cometbft/cometbft/config"
	"github.com/cometbft/cometbft/crypto/ed25519"
	proxy "github.com/cometbft/cometbft/proxy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/config"
	test "github.com/rollkit/rollkit/test/log"
	"github.com/rollkit/rollkit/test/mocks"
	"github.com/rollkit/rollkit/types"

	"google.golang.org/grpc"

	"google.golang.org/grpc/credentials/insecure"

	goDAproxy "github.com/rollkit/go-da/proxy"
	goDATest "github.com/rollkit/go-da/test"
)

// MockServerAddr is the address used by the mock gRPC service
// NOTE: this should be unique per test package to avoid
// "bind: listen address already in use" because multiple packages
// are tested in parallel
var MockServerAddr = "127.0.0.1:7990"

// MockNamespace is a sample namespace used by the mock DA client
var MockNamespace = "00000000000000000000000000000000000000000000000000deadbeef"

// TestMain does setup and teardown on the test package
// to make the mock gRPC service available to the nodes
func TestMain(m *testing.M) {
	srv := startMockGRPCServ()
	if srv == nil {
		os.Exit(1)
	}
	exitCode := m.Run()

	// teardown servers
	srv.GracefulStop()

	os.Exit(exitCode)
}

func startMockGRPCServ() *grpc.Server {
	srv := goDAproxy.NewServer(goDATest.NewDummyDA(), grpc.Creds(insecure.NewCredentials()))
	lis, err := net.Listen("tcp", MockServerAddr)
	if err != nil {
		panic(err)
	}
	go func() {
		_ = srv.Serve(lis)
	}()
	return srv
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
	assert.NoError(t, node.Stop())
	assert.False(t, node.IsRunning())
}

// initializeAndStartNode initializes and starts a node of the specified type.
func initAndStartNodeWithCleanup(ctx context.Context, t *testing.T, nodeType NodeType) Node {
	node, _ := setupTestNode(ctx, t, nodeType)
	startNodeWithCleanup(t, node)

	return node
}

// setupTestNode sets up a test node based on the NodeType.
func setupTestNode(ctx context.Context, t *testing.T, nodeType NodeType) (Node, ed25519.PrivKey) {
	node, privKey, err := newTestNode(ctx, t, nodeType)
	require.NoError(t, err)
	require.NotNil(t, node)

	return node, privKey
}

// newTestNode creates a new test node based on the NodeType.
func newTestNode(ctx context.Context, t *testing.T, nodeType NodeType) (Node, ed25519.PrivKey, error) {
	config := config.NodeConfig{DAAddress: mocks.MockDAAddress, DANamespace: mocks.MockDANamespace}
	switch nodeType {
	case Light:
		config.Light = true
	case Full:
		config.Light = false
	default:
		panic(fmt.Sprintf("invalid node type: %v", nodeType))
	}
	app := setupMockApplication()
	genesis, genesisValidatorKey := types.GetGenesisWithPrivkey()
	signingKey, err := types.PrivKeyToSigningKey(genesisValidatorKey)
	if err != nil {
		return nil, nil, err
	}

	key := generateSingleKey()

	logger := test.NewFileLogger(t)
	node, err := NewNode(ctx, config, key, signingKey, proxy.NewLocalClientCreator(app), genesis, DefaultMetricsProvider(cmconfig.DefaultInstrumentationConfig()), logger)
	return node, genesisValidatorKey, err
}

func TestNewNode(t *testing.T) {
	ctx := context.Background()

	ln := initAndStartNodeWithCleanup(ctx, t, Light)
	require.IsType(t, new(LightNode), ln)
	fn := initAndStartNodeWithCleanup(ctx, t, Full)
	require.IsType(t, new(FullNode), fn)
}
