package node

import (
	"context"
	"testing"
	"time"

	"github.com/rollkit/go-execution"
	execTest "github.com/rollkit/go-execution/test"
	execTypes "github.com/rollkit/go-execution/types"
	"github.com/rollkit/rollkit/types"
	"github.com/stretchr/testify/require"
)

func TestBasicExecutionFlow(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	node, cleanup := setupTestNodeWithCleanup(t)
	defer cleanup()

	// Wait for node initialization
	waitForNodeInitialization()

	executor := getExecutorFromNode(t, node)
	txs := getTransactions(t, executor, ctx)

	mockExec := execTest.NewDummyExecutor()
	stateRoot, maxBytes := initializeChain(t, mockExec, ctx)
	executor = mockExec

	newStateRoot, _ := executeTransactions(t, executor, ctx, txs, stateRoot, maxBytes)

	finalizeExecution(t, executor, ctx)

	require.NotEmpty(newStateRoot)
}

func waitForNodeInitialization() {
	time.Sleep(1 * time.Second)
}

func getExecutorFromNode(t *testing.T, node *FullNode) execution.Executor {
	executor := node.blockManager.GetExecutor()
	require.NotNil(t, executor)
	return executor
}

func getTransactions(t *testing.T, executor execution.Executor, ctx context.Context) []execTypes.Tx {
	txs, err := executor.GetTxs(ctx)
	require.NoError(t, err)
	return txs
}

func initializeChain(t *testing.T, executor execution.Executor, ctx context.Context) (types.Hash, uint64) {
	stateRoot, maxBytes, err := executor.InitChain(ctx, time.Now(), 1, "test-chain")
	require.NoError(t, err)
	require.Greater(t, maxBytes, uint64(0))
	return stateRoot, maxBytes
}

func executeTransactions(t *testing.T, executor execution.Executor, ctx context.Context, txs []execTypes.Tx, stateRoot types.Hash, maxBytes uint64) (types.Hash, uint64) {
	newStateRoot, newMaxBytes, err := executor.ExecuteTxs(ctx, txs, 1, time.Now(), stateRoot)
	require.NoError(t, err)
	require.Greater(t, newMaxBytes, uint64(0))
	require.Equal(t, maxBytes, newMaxBytes)
	return newStateRoot, newMaxBytes
}

func finalizeExecution(t *testing.T, executor execution.Executor, ctx context.Context) {
	err := executor.SetFinal(ctx, 1)
	require.NoError(t, err)
}

func TestExecutionWithDASync(t *testing.T) {
	t.Run("basic DA sync with transactions", func(t *testing.T) {
		require := require.New(t)
		ctx := context.Background()

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

		// Give node time to initialize and submit blocks to DA
		time.Sleep(2 * time.Second)

		// Verify DA client is working
		require.NotNil(node.dalc)

		// Get the executor from the node
		executor := node.blockManager.GetExecutor()
		require.NotNil(executor)

		// Wait for first block to be produced with a shorter timeout
		err = waitForFirstBlock(node, Header)
		require.NoError(err)

		// Get height and verify it's greater than 0
		height, err := getNodeHeight(node, Header)
		require.NoError(err)
		require.Greater(height, uint64(0))

		// Get the block data and verify transactions were included
		header, data, err := node.Store.GetBlockData(ctx, height)
		require.NoError(err)
		require.NotNil(header)
		require.NotNil(data)
	})
}
