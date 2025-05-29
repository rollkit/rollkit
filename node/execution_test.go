package node

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	coreexecutor "github.com/rollkit/rollkit/core/execution"
)

func TestBasicExecutionFlow(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	// Create and start a real node
	node, cleanup := createNodeWithCleanup(t, getTestConfig(t, 1))
	defer cleanup()

	// Wait for node initialization
	err := waitForNodeInitialization(node)
	require.NoError(err)

	// Get the executor from the node
	executor := getExecutorFromNode(t, node)

	// Test the full execution flow with the real executor
	stateRoot, maxBytes := initializeChain(t, executor, ctx)
	require.NotEmpty(stateRoot, "Initial state root should not be empty")
	require.Greater(maxBytes, uint64(0), "Max bytes should be greater than 0")

	// Get transactions from the executor
	txs := getTransactions(t, executor, ctx)
	t.Logf("Retrieved %d transactions for execution", len(txs))

	// Execute the transactions
	newStateRoot, newMaxBytes := executeTransactions(t, executor, ctx, txs, stateRoot)
	require.NotEmpty(newStateRoot, "New state root should not be empty")
	require.Greater(newMaxBytes, uint64(0), "New max bytes should be greater than 0")

	// Finalize the execution
	finalizeExecution(t, executor, ctx)

	// Verify state changes if transactions were executed
	if len(txs) > 0 {
		require.NotEqual(stateRoot, newStateRoot, "State root should change after executing transactions")
	}
}

func waitForNodeInitialization(node *FullNode) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if node.IsRunning() && node.blockManager != nil {
				return nil
			}
		case <-ctx.Done():
			return errors.New("timeout waiting for node initialization")
		}
	}
}

func getExecutorFromNode(t *testing.T, node *FullNode) coreexecutor.Executor {
	require.NotNil(t, node.blockManager, "Block manager should not be nil")
	executor := node.blockManager.GetExecutor()
	require.NotNil(t, executor, "Executor should not be nil")
	return executor
}

func getTransactions(t *testing.T, executor coreexecutor.Executor, ctx context.Context) [][]byte {
	txs, err := executor.GetTxs(ctx)
	require.NoError(t, err, "GetTxs should not return an error")
	return txs
}

func initializeChain(t *testing.T, executor coreexecutor.Executor, ctx context.Context) ([]byte, uint64) {
	genesisTime := time.Now()
	initialHeight := uint64(1)
	chainID := "test-chain"

	stateRoot, maxBytes, err := executor.InitChain(ctx, genesisTime, initialHeight, chainID)
	require.NoError(t, err, "InitChain should not return an error")
	require.Greater(t, maxBytes, uint64(0))
	return stateRoot, maxBytes
}

func executeTransactions(t *testing.T, executor coreexecutor.Executor, ctx context.Context, txs [][]byte, prevStateRoot []byte) ([]byte, uint64) {
	blockHeight := uint64(1)
	timestamp := time.Now()
	metadata := make(map[string]interface{})

	newStateRoot, newMaxBytes, err := executor.ExecuteTxs(ctx, txs, blockHeight, timestamp, prevStateRoot, metadata)
	require.NoError(t, err, "ExecuteTxs should not return an error")
	require.Greater(t, newMaxBytes, uint64(0))
	return newStateRoot, newMaxBytes
}

func finalizeExecution(t *testing.T, executor coreexecutor.Executor, ctx context.Context) {
	blockHeight := uint64(1)
	err := executor.SetFinal(ctx, blockHeight)
	require.NoError(t, err, "SetFinal should not return an error")
}
