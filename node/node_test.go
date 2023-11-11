package node

import (
	"context"
	"fmt"
	"testing"

	proxy "github.com/cometbft/cometbft/proxy"
	cmtypes "github.com/cometbft/cometbft/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/config"
	test "github.com/rollkit/rollkit/test/log"
	"github.com/rollkit/rollkit/types"
)

// cleanUpNode stops the node and checks if it is running
func cleanUpNode(node Node, t *testing.T) {
	defer func() {
		node.Cancel()
	}()
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
	node := setupTestNode(ctx, t, nodeType)
	require.False(t, node.IsRunning())
	err := node.Start()
	require.NoError(t, err)
	require.True(t, node.IsRunning())
	return node
}

func newTestNode(ctx context.Context, t *testing.T, nodeType string) (Node, error) {
	config := config.NodeConfig{DALayer: "mock"}
	switch nodeType {
	case "light":
		config.Light = true
	case "full":
		config.Light = false
	default:
		panic(fmt.Sprint("invalid node type", nodeType))
	}
	app := setupMockApplication()
	key, signingKey := generateSingleKey(), generateSingleKey()
	logger := test.NewFileLogger(t)
	return NewNode(ctx, config, key, signingKey, proxy.NewLocalClientCreator(app), &cmtypes.GenesisDoc{ChainID: types.TestChainID}, logger)
}

// setupTestNode sets up a test node
func setupTestNode(ctx context.Context, t *testing.T, nodeType string) Node {
	node, err := newTestNode(ctx, t, nodeType)
	require.NoError(t, err)
	require.NotNil(t, node)
	return node
}

func TestNewNode(t *testing.T) {
	ctx := context.Background()

	ln := initializeAndStartLightNode(ctx, t)
	cleanUpNode(ln, t)
	fn := initializeAndStartFullNode(ctx, t)
	cleanUpNode(fn, t)
}
