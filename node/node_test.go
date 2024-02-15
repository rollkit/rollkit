package node

import (
	"context"
	"fmt"
	"testing"

	cmconfig "github.com/cometbft/cometbft/config"
	"github.com/cometbft/cometbft/crypto/ed25519"
	proxy "github.com/cometbft/cometbft/proxy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/config"
	test "github.com/rollkit/rollkit/test/log"
	"github.com/rollkit/rollkit/types"
)

var MockServerAddr = ":7980"

var MockNamespace = "00000000000000000000000000000000000000000000000000deadbeef"

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
func initAndStartNodeWithCleanup(ctx context.Context, t *testing.T, nodeType string) Node {
	node, _ := setupTestNode(ctx, t, nodeType)
	startNodeWithCleanup(t, node)

	return node
}

// setupTestNode sets up a test node based on the NodeType.
func setupTestNode(ctx context.Context, t *testing.T, nodeType string) (Node, ed25519.PrivKey) {
	node, privKey, err := newTestNode(ctx, t, nodeType)
	require.NoError(t, err)
	require.NotNil(t, node)

	return node, privKey
}

// newTestNode creates a new test node based on the NodeType.
func newTestNode(ctx context.Context, t *testing.T, nodeType string) (Node, ed25519.PrivKey, error) {
	config := config.NodeConfig{DAAddress: MockServerAddr, DANamespace: MockNamespace}
	switch nodeType {
	case "light":
		config.Light = true
	case "full":
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

	_ = initAndStartNodeWithCleanup(ctx, t, "light").(*LightNode)
	_ = initAndStartNodeWithCleanup(ctx, t, "full").(*FullNode)
}
