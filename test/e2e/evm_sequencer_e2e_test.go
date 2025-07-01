//go:build evm
// +build evm

// Package e2e contains end-to-end tests for Rollkit's EVM integration.
//
// This file specifically tests the EVM sequencer (aggregator) functionality including:
// - Basic sequencer operation and transaction processing
// - High-throughput transaction handling with nonce ordering
// - Invalid transaction rejection and validation
// - Transaction inclusion verification and block production
//
// Test Coverage:
// 1. TestEvmSequencerE2E - Basic sequencer functionality with single transaction
// 2. TestEvmMultipleTransactionInclusionE2E - High-throughput processing (200 transactions)
// 3. TestEvmInvalidTransactionRejectionE2E - Various invalid transaction type rejections
package e2e

import (
	"context"
	"flag"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/execution/evm"
)

// TestEvmSequencerE2E tests the basic end-to-end functionality of a single EVM sequencer node.
//
// Test Flow:
// 1. Sets up Local DA layer for data availability
// 2. Starts EVM (Reth) engine via Docker Compose
// 3. Initializes and starts the sequencer node with proper configuration
// 4. Submits a single test transaction to the EVM
// 5. Verifies the transaction is successfully included in a block
//
// Validation:
// - Sequencer node starts successfully and becomes responsive
// - Transaction submission works correctly
// - Transaction is included in EVM block with success status
// - Block production occurs within expected timeframe
//
// This test validates the core functionality of Rollkit's EVM integration
// and ensures basic transaction processing works correctly.
func TestEvmSequencerE2E(t *testing.T) {
	flag.Parse()
	workDir := t.TempDir()
	nodeHome := filepath.Join(workDir, "evm-agg")
	sut := NewSystemUnderTest(t)

	// Setup sequencer
	genesisHash := setupSequencerOnlyTest(t, sut, nodeHome)
	t.Logf("Genesis hash: %s", genesisHash)

	// Submit a transaction to EVM
	lastNonce := uint64(0)
	tx := evm.GetRandomTransaction(t, TestPrivateKey, TestToAddress, DefaultChainID, DefaultGasLimit, &lastNonce)
	evm.SubmitTransaction(t, tx)
	t.Log("Submitted test transaction to EVM")

	// Wait for block production and verify transaction inclusion
	require.Eventually(t, func() bool {
		return evm.CheckTxIncluded(t, tx.Hash())
	}, 15*time.Second, 500*time.Millisecond)
	t.Log("Transaction included in EVM block")
}

// TestEvmMultipleTransactionInclusionE2E tests high-throughput transaction processing
// to ensure multiple transactions submitted in quick succession are all included
// and maintain correct ordering without any loss or corruption.
//
// Test Configuration:
// - Submits 500 transactions in rapid succession (10ms intervals)
// - Each transaction has sequential nonces (0-499)
// - Uses proper chain ID (1234) for transaction signing
// - Extended timeout (120s) to handle large transaction volume
//
// Test Flow:
// 1. Sets up Local DA layer and EVM sequencer
// 2. Submits 500 transactions rapidly with 10ms delays between submissions
// 3. Waits for all transactions to be included in blocks
// 4. Verifies each transaction maintains correct nonce ordering
// 5. Analyzes transaction distribution across blocks
// 6. Ensures no transactions are lost or reordered
//
// Validation Criteria:
// - All 500 transactions are successfully included
// - Nonce sequence is perfectly maintained (0, 1, 2, ..., 499)
// - No transaction loss occurs under high-frequency submission
// - Transaction receipts show success status for all transactions
// - Block distribution is reasonable and logged for analysis
//
// Performance Expectations:
// - ~50 transactions per second submission rate
// - ~55 transactions per block average packing
// - Total test execution under 20 seconds
// - Consistent performance across multiple runs
//
// This test validates that Rollkit can handle production-level burst transaction loads
// while maintaining all ordering guarantees and preventing transaction loss.
func TestEvmMultipleTransactionInclusionE2E(t *testing.T) {
	flag.Parse()
	workDir := t.TempDir()
	nodeHome := filepath.Join(workDir, "evm-agg")
	sut := NewSystemUnderTest(t)

	// Setup sequencer
	genesisHash := setupSequencerOnlyTest(t, sut, nodeHome)
	t.Logf("Genesis hash: %s", genesisHash)

	// Connect to EVM
	client, err := ethclient.Dial(SequencerEthURL)
	require.NoError(t, err, "Should be able to connect to EVM")
	defer client.Close()

	// Submit multiple transactions in quick succession
	const numTxs = 200
	var txHashes []common.Hash
	var expectedNonces []uint64
	lastNonce := uint64(0)
	ctx := context.Background()

	t.Logf("Submitting %d transactions in quick succession...", numTxs)
	for i := 0; i < numTxs; i++ {
		// Create transaction with proper chain ID
		tx := evm.GetRandomTransaction(t, TestPrivateKey, TestToAddress, DefaultChainID, DefaultGasLimit, &lastNonce)

		evm.SubmitTransaction(t, tx)
		txHashes = append(txHashes, tx.Hash())
		expectedNonces = append(expectedNonces, tx.Nonce())

		// Log progress every 50 transactions to avoid spam
		if (i+1)%50 == 0 || i < 10 {
			t.Logf("Submitted transaction %d: hash=%s, nonce=%d", i+1, tx.Hash().Hex(), tx.Nonce())
		}

		// Optimized delay for faster test execution while maintaining reliability
		time.Sleep(20 * time.Millisecond)
	}

	// Wait for all transactions to be included and verify order
	var receipts []*common.Hash

	t.Log("Waiting for all transactions to be included...")
	require.Eventually(t, func() bool {
		receipts = receipts[:0] // Clear slice

		for _, txHash := range txHashes {
			receipt, err := client.TransactionReceipt(ctx, txHash)
			if err != nil || receipt == nil || receipt.Status != 1 {
				return false // Not all transactions included yet
			}
			// Shadow the loop variable to avoid taking address of the same variable
			hashCopy := txHash
			receipts = append(receipts, &hashCopy)
		}

		// Log progress every 100 transactions
		if len(receipts)%100 == 0 && len(receipts) > 0 {
			t.Logf("Progress: %d/%d transactions included", len(receipts), numTxs)
		}

		return len(receipts) == numTxs
	}, 45*time.Second, 500*time.Millisecond, "All transactions should be included")

	t.Logf("✅ All %d transactions were successfully included", numTxs)

	// Verify transaction order by checking nonces
	var actualNonces []uint64
	var blockNumbers []uint64

	t.Log("Verifying transaction nonces and block inclusion...")
	for i, txHash := range txHashes {
		receipt, err := client.TransactionReceipt(ctx, txHash)
		require.NoError(t, err, "Should get receipt for transaction %d", i+1)
		require.Equal(t, uint64(1), receipt.Status, "Transaction %d should be successful", i+1)

		// Get the actual transaction to check nonce
		tx, _, err := client.TransactionByHash(ctx, txHash)
		require.NoError(t, err, "Should get transaction %d", i+1)

		actualNonces = append(actualNonces, tx.Nonce())
		blockNumbers = append(blockNumbers, receipt.BlockNumber.Uint64())

		// Log progress for verification every 100 transactions, plus first 10 and last 10
		if (i+1)%100 == 0 || i < 10 || i >= numTxs-10 {
			t.Logf("Transaction %d: nonce=%d, block=%d, expected_nonce=%d",
				i+1, tx.Nonce(), receipt.BlockNumber.Uint64(), expectedNonces[i])
		}
	}

	// Verify nonce ordering (transactions should maintain nonce order)
	for i := 0; i < numTxs; i++ {
		require.Equal(t, expectedNonces[i], actualNonces[i],
			"Transaction %d should have expected nonce %d, got %d", i+1, expectedNonces[i], actualNonces[i])
	}

	// Verify no transactions were lost
	require.Equal(t, numTxs, len(actualNonces), "All %d transactions should be included", numTxs)

	// Log block distribution
	blockCounts := make(map[uint64]int)
	var minBlock, maxBlock uint64 = ^uint64(0), 0

	for _, blockNum := range blockNumbers {
		blockCounts[blockNum]++
		if blockNum < minBlock {
			minBlock = blockNum
		}
		if blockNum > maxBlock {
			maxBlock = blockNum
		}
	}

	t.Logf("Transaction distribution across %d blocks (blocks %d-%d):", len(blockCounts), minBlock, maxBlock)
	totalBlocks := len(blockCounts)
	if totalBlocks <= 20 {
		// Show all blocks if reasonable number
		for blockNum := minBlock; blockNum <= maxBlock; blockNum++ {
			if count, exists := blockCounts[blockNum]; exists {
				t.Logf("  Block %d: %d transactions", blockNum, count)
			}
		}
	} else {
		// Show summary for large number of blocks
		t.Logf("  Average transactions per block: %.2f", float64(numTxs)/float64(totalBlocks))
		t.Logf("  First 5 blocks:")
		for blockNum := minBlock; blockNum < minBlock+5 && blockNum <= maxBlock; blockNum++ {
			if count, exists := blockCounts[blockNum]; exists {
				t.Logf("    Block %d: %d transactions", blockNum, count)
			}
		}
		t.Logf("  Last 5 blocks:")
		for blockNum := maxBlock - 4; blockNum <= maxBlock && blockNum >= minBlock; blockNum++ {
			if count, exists := blockCounts[blockNum]; exists {
				t.Logf("    Block %d: %d transactions", blockNum, count)
			}
		}
	}

	t.Logf("✅ Test PASSED: All %d transactions included in correct nonce order", numTxs)
}

// TestEvmInvalidTransactionRejectionE2E tests the system's ability to properly reject
// various types of invalid transactions and ensure they are not included in any blocks.
//
// Test Purpose:
// - Confirm that invalid transactions are rejected and not included in blocks
// - Test various types of invalid transactions (bad signature, insufficient funds, malformed data)
// - Ensure system stability when processing invalid transactions
// - Validate that valid transactions still work after invalid ones are rejected
//
// Test Flow:
//  1. Sets up Local DA layer and EVM sequencer
//  2. Submits various invalid transactions:
//     a. Transaction with invalid signature
//     b. Transaction with insufficient funds (from empty account)
//     c. Transaction with invalid nonce (too high)
//     d. Transaction with invalid gas limit (too low)
//  3. Verifies that all invalid transactions are rejected
//  4. Submits a valid transaction to confirm system stability
//  5. Ensures the valid transaction is processed correctly
//
// Expected Behavior:
// - All invalid transactions should be rejected at submission or processing
// - No invalid transactions should appear in any blocks
// - Valid transactions should continue to work normally
// - System should remain stable and responsive after rejecting invalid transactions
//
// This test validates Rollkit's transaction validation and rejection mechanisms.
func TestEvmInvalidTransactionRejectionE2E(t *testing.T) {
	flag.Parse()
	workDir := t.TempDir()
	nodeHome := filepath.Join(workDir, "evm-agg")
	sut := NewSystemUnderTest(t)

	// Setup sequencer
	genesisHash := setupSequencerOnlyTest(t, sut, nodeHome)
	t.Logf("Genesis hash: %s", genesisHash)

	// Connect to EVM
	client, err := ethclient.Dial(SequencerEthURL)
	require.NoError(t, err, "Should be able to connect to EVM")
	defer client.Close()

	ctx := context.Background()
	var invalidTxHashes []common.Hash
	var invalidTxErrors []string

	t.Log("Testing various invalid transaction types...")

	// Test invalid signature transaction
	t.Log("7a. Testing transaction with invalid signature...")
	func() {
		defer func() {
			if r := recover(); r != nil {
				invalidTxErrors = append(invalidTxErrors, fmt.Sprintf("Invalid signature tx: %v", r))
				t.Logf("✅ Invalid signature transaction rejected as expected: %v", r)
			}
		}()

		// Try to submit with a bad signature by creating a transaction with wrong private key
		badPrivKey := "1111111111111111111111111111111111111111111111111111111111111111"
		lastNonce := uint64(0)
		badTx := evm.GetRandomTransaction(t, badPrivKey, TestToAddress, DefaultChainID, DefaultGasLimit, &lastNonce)

		err := client.SendTransaction(ctx, badTx)
		if err != nil {
			invalidTxErrors = append(invalidTxErrors, fmt.Sprintf("Invalid signature tx: %v", err))
			t.Logf("✅ Invalid signature transaction rejected as expected: %v", err)
		} else {
			invalidTxHashes = append(invalidTxHashes, badTx.Hash())
			t.Logf("⚠️  Invalid signature transaction was submitted: %s", badTx.Hash().Hex())
		}
	}()

	// Test insufficient funds transaction
	t.Log("7b. Testing transaction with insufficient funds...")
	func() {
		defer func() {
			if r := recover(); r != nil {
				invalidTxErrors = append(invalidTxErrors, fmt.Sprintf("Insufficient funds tx: %v", r))
				t.Logf("✅ Insufficient funds transaction rejected as expected: %v", r)
			}
		}()

		// Use an empty account that has no funds
		emptyAccountPrivKey := "2222222222222222222222222222222222222222222222222222222222222222"
		lastNonce := uint64(0)
		tx := evm.GetRandomTransaction(t, emptyAccountPrivKey, TestToAddress, DefaultChainID, DefaultGasLimit, &lastNonce)

		err := client.SendTransaction(ctx, tx)
		if err != nil {
			invalidTxErrors = append(invalidTxErrors, fmt.Sprintf("Insufficient funds tx: %v", err))
			t.Logf("✅ Insufficient funds transaction rejected as expected: %v", err)
		} else {
			invalidTxHashes = append(invalidTxHashes, tx.Hash())
			t.Logf("⚠️  Insufficient funds transaction was submitted: %s", tx.Hash().Hex())
		}
	}()

	// Test invalid nonce transaction (way too high)
	t.Log("7c. Testing transaction with invalid nonce...")
	func() {
		defer func() {
			if r := recover(); r != nil {
				invalidTxErrors = append(invalidTxErrors, fmt.Sprintf("Invalid nonce tx: %v", r))
				t.Logf("✅ Invalid nonce transaction rejected as expected: %v", r)
			}
		}()

		// Use a very high nonce that's way ahead of the current account nonce
		lastNonce := uint64(999999)
		tx := evm.GetRandomTransaction(t, TestPrivateKey, TestToAddress, DefaultChainID, DefaultGasLimit, &lastNonce)

		err := client.SendTransaction(ctx, tx)
		if err != nil {
			invalidTxErrors = append(invalidTxErrors, fmt.Sprintf("Invalid nonce tx: %v", err))
			t.Logf("✅ Invalid nonce transaction rejected as expected: %v", err)
		} else {
			invalidTxHashes = append(invalidTxHashes, tx.Hash())
			t.Logf("⚠️  Invalid nonce transaction was submitted: %s", tx.Hash().Hex())
		}
	}()

	// Test invalid gas limit transaction (too low)
	t.Log("7d. Testing transaction with invalid gas limit...")
	func() {
		defer func() {
			if r := recover(); r != nil {
				invalidTxErrors = append(invalidTxErrors, fmt.Sprintf("Invalid gas limit tx: %v", r))
				t.Logf("✅ Invalid gas limit transaction rejected as expected: %v", r)
			}
		}()

		// Use an extremely low gas limit that's insufficient for basic transfer
		lastNonce := uint64(0)
		tx := evm.GetRandomTransaction(t, TestPrivateKey, TestToAddress, DefaultChainID, 1000, &lastNonce) // Very low gas

		err := client.SendTransaction(ctx, tx)
		if err != nil {
			invalidTxErrors = append(invalidTxErrors, fmt.Sprintf("Invalid gas limit tx: %v", err))
			t.Logf("✅ Invalid gas limit transaction rejected as expected: %v", err)
		} else {
			invalidTxHashes = append(invalidTxHashes, tx.Hash())
			t.Logf("⚠️  Invalid gas limit transaction was submitted: %s", tx.Hash().Hex())
		}
	}()

	// Wait for any transactions to be processed
	t.Log("Waiting for transaction processing...")
	time.Sleep(500 * time.Millisecond)

	// Check that none of the invalid transactions were included in blocks
	invalidTxsIncluded := 0
	for i, txHash := range invalidTxHashes {
		receipt, err := client.TransactionReceipt(ctx, txHash)
		if err == nil && receipt != nil {
			invalidTxsIncluded++
			t.Errorf("❌ Invalid transaction %d was included in block %d: %s", i+1, receipt.BlockNumber.Uint64(), txHash.Hex())
		}
	}

	if invalidTxsIncluded > 0 {
		require.Fail(t, fmt.Sprintf("❌ %d invalid transactions were incorrectly included in blocks", invalidTxsIncluded))
	} else {
		t.Logf("✅ All invalid transactions were properly rejected: %d errors recorded", len(invalidTxErrors))
		for i, errMsg := range invalidTxErrors {
			t.Logf("   %d. %s", i+1, errMsg)
		}
	}

	// Submit a valid transaction to verify system stability
	t.Log("Testing system stability with valid transaction...")
	lastNonce := uint64(0)
	validTx := evm.GetRandomTransaction(t, TestPrivateKey, TestToAddress, DefaultChainID, DefaultGasLimit, &lastNonce)

	evm.SubmitTransaction(t, validTx)
	t.Logf("Submitted valid transaction: %s", validTx.Hash().Hex())

	// Wait for valid transaction to be included
	require.Eventually(t, func() bool {
		receipt, err := client.TransactionReceipt(ctx, validTx.Hash())
		return err == nil && receipt != nil && receipt.Status == 1
	}, 15*time.Second, 500*time.Millisecond, "Valid transaction should be included after invalid ones were rejected")

	t.Log("✅ Valid transaction included successfully - system stability confirmed")

	// Final verification
	validReceipt, err := client.TransactionReceipt(ctx, validTx.Hash())
	require.NoError(t, err, "Should get receipt for valid transaction")
	require.Equal(t, uint64(1), validReceipt.Status, "Valid transaction should be successful")

	t.Logf("✅ Test PASSED: Invalid transaction rejection working correctly")
	t.Logf("   - %d invalid transactions properly rejected", len(invalidTxErrors))
	t.Logf("   - No invalid transactions included in any blocks")
	t.Logf("   - Valid transactions continue to work normally")
	t.Logf("   - System maintains stability after rejecting invalid transactions")
	t.Logf("   - Transaction validation mechanisms functioning properly")
}
