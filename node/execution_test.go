//go:build !integration

package node

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	coreexecutor "github.com/rollkit/rollkit/core/execution"
	testmocks "github.com/rollkit/rollkit/test/mocks"
	"github.com/rollkit/rollkit/types"
)

func TestBasicExecutionFlow(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	node, cleanup := createNodeWithCleanup(t, getTestConfig(t, 1))
	defer cleanup()

	// Wait for node initialization
	err := waitForNodeInitialization(node)
	require.NoError(err)

	// Get the original executor to retrieve transactions
	originalExecutor := getExecutorFromNode(t, node)
	txs := getTransactions(t, originalExecutor, ctx)

	// Use the generated mock executor for testing execution steps
	mockExec := testmocks.NewMockExecutor(t)

	// Define expected state and parameters
	expectedInitialStateRoot := []byte("initial state root")
	expectedMaxBytes := uint64(1024)
	expectedNewStateRoot := []byte("new state root")
	blockHeight := uint64(1)
	chainID := "test-chain"

	// Set expectations on the mock executor
	mockExec.On("InitChain", mock.Anything, mock.AnythingOfType("time.Time"), blockHeight, chainID).
		Return(expectedInitialStateRoot, expectedMaxBytes, nil).Once()
	mockExec.On("ExecuteTxs", mock.Anything, txs, blockHeight, mock.AnythingOfType("time.Time"), expectedInitialStateRoot).
		Return(expectedNewStateRoot, expectedMaxBytes, nil).Once()
	mockExec.On("SetFinal", mock.Anything, blockHeight).
		Return(nil).Once()

	// Call helper functions with the mock executor
	stateRoot, maxBytes := initializeChain(t, mockExec, ctx)
	require.Equal(expectedInitialStateRoot, stateRoot)
	require.Equal(expectedMaxBytes, maxBytes)

	newStateRoot, newMaxBytes := executeTransactions(t, mockExec, ctx, txs, stateRoot, maxBytes)
	require.Equal(expectedNewStateRoot, newStateRoot)
	require.Equal(expectedMaxBytes, newMaxBytes)

	finalizeExecution(t, mockExec, ctx)

	require.NotEmpty(newStateRoot)
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
	executor := node.blockManager.GetExecutor()
	require.NotNil(t, executor)
	return executor
}

func getTransactions(t *testing.T, executor coreexecutor.Executor, ctx context.Context) [][]byte {
	txs, err := executor.GetTxs(ctx, 1000000)
	require.NoError(t, err)
	return txs
}

func initializeChain(t *testing.T, executor coreexecutor.Executor, ctx context.Context) ([]byte, uint64) {
	genesisTime := time.Now()
	initialHeight := uint64(1)
	chainID := "test-chain"
	stateRoot, maxBytes, err := executor.InitChain(ctx, genesisTime, initialHeight, chainID)
	require.NoError(t, err)
	require.Greater(t, maxBytes, uint64(0))
	return stateRoot, maxBytes
}

func executeTransactions(t *testing.T, executor coreexecutor.Executor, ctx context.Context, txs [][]byte, stateRoot types.Hash, maxBytes uint64) ([]byte, uint64) {
	blockHeight := uint64(1)
	timestamp := time.Now()
	newStateRoot, newMaxBytes, err := executor.ExecuteTxs(ctx, txs, blockHeight, timestamp, stateRoot)
	require.NoError(t, err)
	require.Greater(t, newMaxBytes, uint64(0))
	return newStateRoot, newMaxBytes
}

func finalizeExecution(t *testing.T, executor coreexecutor.Executor, ctx context.Context) {
	err := executor.SetFinal(ctx, 1)
	require.NoError(t, err)
}
