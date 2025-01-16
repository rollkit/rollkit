package node

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// simply check that node is starting and stopping without panicking
func TestStartup(t *testing.T) {
	node, cleanup := setupTestNodeWithCleanup(t)
	require.IsType(t, new(FullNode), node)
	require.False(t, node.IsRunning())
	require.NoError(t, node.Start())
	require.True(t, node.IsRunning())
	require.NoError(t, node.Stop())
	require.False(t, node.IsRunning())
	cleanup()
	require.False(t, node.IsRunning())
}
