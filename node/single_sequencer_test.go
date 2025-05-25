package node

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Test that node can start and be shutdown properly using context cancellation
func TestStartup(t *testing.T) {
	// Get the node and cleanup function
	node, cleanup := createNodeWithCleanup(t, getTestConfig(t, 1))
	require.IsType(t, new(FullNode), node)

	// Create a context with cancel function for node operation
	ctx, cancel := context.WithCancel(context.Background())

	// Start the node in a goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- node.Run(ctx)
	}()

	// Allow some time for the node to start
	time.Sleep(100 * time.Millisecond)

	// Node should be running (no error received yet)
	select {
	case err := <-errChan:
		t.Fatalf("Node stopped unexpectedly with error: %v", err)
	default:
		// This is expected - node is still running
	}

	// Cancel the context to stop the node
	cancel()

	// Allow some time for the node to stop and check for errors
	select {
	case err := <-errChan:
		// Context cancellation should result in context.Canceled error
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Node did not stop after context cancellation")
	}

	// Run the cleanup function from setupTestNodeWithCleanup
	cleanup()
}
