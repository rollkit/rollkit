package node

import (
	"context"
	"fmt"
	"testing"

	"github.com/cometbft/cometbft/crypto/ed25519"
	proxy "github.com/cometbft/cometbft/proxy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/config"
	test "github.com/rollkit/rollkit/test/log"
	"github.com/rollkit/rollkit/types"
)

var MockServerAddr = ":7980"

// cleanUpNode stops the node and checks if it is running
func cleanUpNode(node Node, t *testing.T) {
	assert.NoError(t, node.Stop())
	assert.False(t, node.IsRunning())
}

func initializeAndStartFullNode(ctx context.Context, t *testing.T) *FullNode {
	node := initializeAndStartNode(ctx, t, "full")
	return node.(*FullNode)
}

func initializeAndStartLightNode(ctx context.Context, t *testing.T) *LightNode {
	node := initializeAndStartNode(ctx, t, "light")
	return node.(*LightNode)
}

// initializeAndStartNode initializes and starts a test node
func initializeAndStartNode(ctx context.Context, t *testing.T, nodeType string) Node {
	node, _ := setupTestNode(ctx, t, nodeType)
	require.False(t, node.IsRunning())
	err := node.Start()
	require.NoError(t, err)
	require.True(t, node.IsRunning())
	return node
}

func newTestNode(ctx context.Context, t *testing.T, nodeType string) (Node, error, ed25519.PrivKey) {
	config := config.NodeConfig{DAAddress: MockServerAddr}
	switch nodeType {
	case "light":
		config.Light = true
	case "full":
		config.Light = false
	default:
		panic(fmt.Sprint("invalid node type", nodeType))
	}
	app := setupMockApplication()
	genesis, genesisValidatorKey := types.GetGenesisWithPrivkey()
	signingKey, err := types.PrivKeyToSigningKey(genesisValidatorKey)
	if err != nil {
		return nil, err, nil
	}
	key := generateSingleKey()
	logger := test.NewFileLogger(t)
	node, err := NewNode(ctx, config, key, signingKey, proxy.NewLocalClientCreator(app), genesis, logger)
	return node, err, genesisValidatorKey
}

// setupTestNode sets up a test node
func setupTestNode(ctx context.Context, t *testing.T, nodeType string) (Node, ed25519.PrivKey) {
	node, err, privKey := newTestNode(ctx, t, nodeType)
	require.NoError(t, err)
	require.NotNil(t, node)
	return node, privKey
}

func TestNewNode(t *testing.T) {
	ctx := context.Background()

	ln := initializeAndStartLightNode(ctx, t)
	cleanUpNode(ln, t)
	fn := initializeAndStartFullNode(ctx, t)
	cleanUpNode(fn, t)
}
