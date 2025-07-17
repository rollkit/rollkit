//go:build evm
// +build evm

package evm

import (
	"context"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	ethTypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/require"
)

// TestEngineClient_RollbackIntegration tests the rollback functionality using a real Reth engine.
// This test builds a chain with real blocks and transactions, then tests rollback to ensure
// it correctly reverts the execution state to the previous block.
func TestEngineClient_RollbackIntegration(t *testing.T) {
	// Setup test environment
	jwtSecret := SetupTestRethEngine(t, DOCKER_PATH, JWT_FILENAME)

	executionClient, err := NewEngineExecutionClient(
		TEST_ETH_URL,
		TEST_ENGINE_URL,
		jwtSecret,
		common.HexToHash(GENESIS_HASH),
		common.Address{},
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	// Initialize chain
	initialHeight := uint64(1)
	genesisTime := time.Now().UTC().Truncate(time.Second)
	genesisStateRoot := common.HexToHash(GENESIS_STATEROOT)
	rollkitGenesisStateRoot := genesisStateRoot[:]

	stateRoot, gasLimit, err := executionClient.InitChain(ctx, genesisTime, initialHeight, CHAIN_ID)
	require.NoError(t, err)
	require.Equal(t, rollkitGenesisStateRoot, stateRoot)
	require.NotZero(t, gasLimit)

	// Build chain with multiple blocks
	var allStateRoots [][]byte
	var lastNonce uint64
	prevStateRoot := rollkitGenesisStateRoot
	baseTimestamp := time.Now()

	// Store genesis state root
	allStateRoots = append(allStateRoots, prevStateRoot)

	// Build blocks 1, 2, and 3
	for blockHeight := initialHeight; blockHeight <= 3; blockHeight++ {
		nTxs := 2 // Use 2 transactions per block for simplicity
		
		// Create and submit transactions
		txs := make([]*ethTypes.Transaction, nTxs)
		for i := range txs {
			txs[i] = GetRandomTransaction(t, TEST_PRIVATE_KEY, TEST_TO_ADDRESS, CHAIN_ID, 22000, &lastNonce)
			SubmitTransaction(t, txs[i])
		}

		// Get payload from mempool
		payload, err := executionClient.GetTxs(ctx)
		require.NoError(t, err)
		require.Len(t, payload, nTxs)

		// Execute block
		blockTimestamp := baseTimestamp.Add(time.Duration(blockHeight-initialHeight) * time.Second)
		newStateRoot, maxBytes, err := executionClient.ExecuteTxs(ctx, payload, blockHeight, blockTimestamp, prevStateRoot)
		require.NoError(t, err)
		require.NotZero(t, maxBytes)
		require.NotEqual(t, prevStateRoot, newStateRoot, "State root should change when transactions are executed")

		// Finalize block
		err = executionClient.SetFinal(ctx, blockHeight)
		require.NoError(t, err)

		// Store state root and verify block was processed
		allStateRoots = append(allStateRoots, newStateRoot)
		lastHeight, lastHash, lastTxs := checkLatestBlock(t, ctx)
		require.Equal(t, blockHeight, lastHeight)
		require.NotEmpty(t, lastHash.Hex())
		require.Equal(t, nTxs, lastTxs)

		t.Logf("Built block %d: state root %x", blockHeight, newStateRoot)
		prevStateRoot = newStateRoot
	}

	// Test rollback validation - should fail for invalid heights
	t.Run("Rollback validation", func(t *testing.T) {
		// Test rollback from height 1 (should fail)
		_, err := executionClient.Rollback(ctx, 1)
		require.Error(t, err)
		require.Contains(t, err.Error(), "cannot rollback from height 1: must be > 1")

		// Test rollback from height 0 (should fail)
		_, err = executionClient.Rollback(ctx, 0)
		require.Error(t, err)
		require.Contains(t, err.Error(), "cannot rollback from height 0: must be > 1")
	})

	// Test successful rollback from height 3 to height 2
	t.Run("Successful rollback", func(t *testing.T) {
		// Verify we're currently at block 3
		currentHeight, currentHash, currentTxs := checkLatestBlock(t, ctx)
		require.Equal(t, uint64(3), currentHeight)
		require.NotEmpty(t, currentHash.Hex())
		require.Equal(t, 2, currentTxs)

		// Perform rollback from block 3 to block 2
		prevStateRoot, err := executionClient.Rollback(ctx, 3)
		require.NoError(t, err)

		// Verify rollback returned the correct previous state root (block 2)
		expectedPrevStateRoot := allStateRoots[2] // State root of block 2
		require.Equal(t, expectedPrevStateRoot, prevStateRoot, 
			"Rollback should return state root of block 2. Expected: %x, Got: %x", 
			expectedPrevStateRoot, prevStateRoot)

		t.Logf("Successfully performed rollback from block 3 to block 2")
		t.Logf("Returned state root: %x", prevStateRoot)

		// Test that we can continue building on the rolled-back state
		// Create and submit a new transaction
		newTx := GetRandomTransaction(t, TEST_PRIVATE_KEY, TEST_TO_ADDRESS, CHAIN_ID, 22000, &lastNonce)
		SubmitTransaction(t, newTx)

		// Get payload from mempool
		payload, err := executionClient.GetTxs(ctx)
		require.NoError(t, err)
		require.Len(t, payload, 1)

		// Execute new block building on the rolled-back state
		blockTimestamp := baseTimestamp.Add(4 * time.Second)
		newStateRoot, maxBytes, err := executionClient.ExecuteTxs(ctx, payload, 4, blockTimestamp, prevStateRoot)
		require.NoError(t, err)
		require.NotZero(t, maxBytes)
		require.NotEqual(t, prevStateRoot, newStateRoot, "State root should change when building new block")

		// Finalize the new block
		err = executionClient.SetFinal(ctx, 4)
		require.NoError(t, err)

		// Verify we can query the new block
		finalHeight, finalHash, finalTxs := checkLatestBlock(t, ctx)
		require.Equal(t, uint64(4), finalHeight)
		require.NotEmpty(t, finalHash.Hex())
		require.Equal(t, 1, finalTxs)

		t.Logf("Successfully built new block 4 on rolled-back state")
		t.Logf("New block hash: %s, state root: %x", finalHash.Hex(), newStateRoot)
	})

	// Test rollback functionality without checking chain height 
	t.Run("Rollback functionality verification", func(t *testing.T) {
		// Perform rollback from current height to height 3
		prevStateRoot, err := executionClient.Rollback(ctx, 4)
		require.NoError(t, err)

		// The previous state root should be from block 3 (since we're rolling back from block 4)
		expectedPrevStateRoot := allStateRoots[3] // State root of block 3
		require.Equal(t, expectedPrevStateRoot, prevStateRoot,
			"Rollback should return state root of block 3. Expected: %x, Got: %x",
			expectedPrevStateRoot, prevStateRoot)

		t.Logf("Successfully performed rollback from block 4 to block 3")
		t.Logf("Returned state root: %x", prevStateRoot)

		// Verify we can continue building on this rolled-back state
		newTx := GetRandomTransaction(t, TEST_PRIVATE_KEY, TEST_TO_ADDRESS, CHAIN_ID, 22000, &lastNonce)
		SubmitTransaction(t, newTx)

		payload, err := executionClient.GetTxs(ctx)
		require.NoError(t, err)
		require.Len(t, payload, 1)

		blockTimestamp := baseTimestamp.Add(5 * time.Second)
		newStateRoot, maxBytes, err := executionClient.ExecuteTxs(ctx, payload, 5, blockTimestamp, prevStateRoot)
		require.NoError(t, err)
		require.NotZero(t, maxBytes)
		require.NotEqual(t, prevStateRoot, newStateRoot)

		t.Logf("Successfully built new block on rolled-back state with state root: %x", newStateRoot)
	})
}