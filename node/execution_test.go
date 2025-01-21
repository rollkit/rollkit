package node

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBasicExecutionFlow(t *testing.T) {
	require := require.New(t)

	// Setup node
	node, cleanup := setupTestNodeWithCleanup(t)
	defer cleanup()

	//startNodeWithCleanup(t, node)

	ctx := context.Background()

	// Give node time to initialize
	time.Sleep(100 * time.Millisecond)

	// Test InitChain - we don't need to call this explicitly as it's done during node start
	executor := node.blockManager.GetExecutor()
	require.NotNil(executor)

	// Test GetTxs
	txs, err := executor.GetTxs(ctx)
	require.NoError(err)

	lastState, err := node.Store.GetState(ctx)
	require.NoError(err)

	// Test ExecuteTxs
	newStateRoot, newMaxBytes, err := executor.ExecuteTxs(ctx, txs, 1, time.Now(), lastState.LastResultsHash)
	require.NoError(err)
	require.Greater(newMaxBytes, uint64(0))
	t.Logf("newStateRoot: %s %d", newStateRoot, newMaxBytes)
	require.NotEmpty(newStateRoot)

	// Test SetFinal
	err = executor.SetFinal(ctx, 1)
	require.NoError(err)
}

func TestExecutionWithDASync(t *testing.T) {
	t.Run("basic DA sync", func(t *testing.T) {
		require := require.New(t)

		// Setup node with mock DA
		node, cleanup := setupTestNodeWithCleanup(t)
		defer cleanup()

		// Start node
		err := node.Start()
		require.NoError(err)
		defer func() {
			err := node.Stop()
			require.NoError(err)
		}()

		// Give node time to initialize
		time.Sleep(100 * time.Millisecond)

		// Verify DA client is working
		require.NotNil(node.dalc)

		// Wait for first block to be produced
		err = waitForFirstBlock(node, Header)
		require.NoError(err)

		// Get height and verify it's greater than 0
		height, err := getNodeHeight(node, Header)
		require.NoError(err)
		require.Greater(height, uint64(0))
	})
}
