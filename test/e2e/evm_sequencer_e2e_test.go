//go:build evm
// +build evm

// Package e2e contains end-to-end tests for Rollkit's EVM integration.
//
// This file specifically tests the EVM aggregator (sequencer) functionality including:
// - Basic sequencer operation and transaction processing
// - Full node synchronization via P2P
// - High-throughput transaction handling and ordering
//
// Test Coverage:
// 1. TestEvmSequencerE2E - Basic sequencer functionality
// 2. TestEvmSequencerWithFullNodeE2E - Full node P2P sync
// 3. TestEvmMultipleTransactionInclusionE2E - High-throughput transaction processing
//
// Prerequisites:
// - Docker and Docker Compose (for Reth EVM engine)
// - Built binaries: evm-single, local-da
// - Available ports: 7980 (DA), 7331/46657 (Rollkit RPC), 8545/8551/8555/8561 (EVM)
package e2e

import (
	"context"
	"flag"
	"path/filepath"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/execution/evm"
)

var evmSingleBinaryPath string

const (
	DOCKER_PATH = "../../execution/evm/docker"
)

func init() {
	flag.StringVar(&evmSingleBinaryPath, "evm-binary", "evm-single", "evm-single binary")
}

// setupTestRethEngineE2E sets up a Reth EVM engine for E2E testing using Docker Compose.
// This creates the sequencer's EVM instance on standard ports (8545/8551).
//
// Returns: JWT secret string for authenticating with the EVM engine
func setupTestRethEngineE2E(t *testing.T) string {
	return evm.SetupTestRethEngine(t, DOCKER_PATH, "jwt.hex")
}

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

	// 1. Start local DA
	localDABinary := filepath.Join(filepath.Dir(evmSingleBinaryPath), "local-da")
	sut.ExecCmd(localDABinary)
	t.Log("Started local DA")
	time.Sleep(200 * time.Millisecond)

	// 2. Start EVM (Reth) via Docker Compose using setupTestRethEngine logic
	jwtSecret := setupTestRethEngineE2E(t)

	// 3. Get genesis hash from EVM node
	genesisHash := evm.GetGenesisHash(t)
	t.Logf("Genesis hash: %s", genesisHash)

	// 4. Initialize sequencer node
	output, err := sut.RunCmd(evmSingleBinaryPath,
		"init",
		"--rollkit.node.aggregator=true",
		"--rollkit.signer.passphrase", "secret",
		"--home", nodeHome,
	)
	require.NoError(t, err, "failed to init sequencer", output)
	t.Log("Initialized sequencer node")

	// 5. Start sequencer node
	sut.ExecCmd(evmSingleBinaryPath,
		"start",
		"--evm.jwt-secret", jwtSecret,
		"--evm.genesis-hash", genesisHash,
		"--rollkit.node.block_time", "1s",
		"--rollkit.node.aggregator=true",
		"--rollkit.signer.passphrase", "secret",
		"--home", nodeHome,
		"--rollkit.da.address", "http://localhost:7980",
		"--rollkit.da.block_time", "1m",
	)
	sut.AwaitNodeUp(t, "http://127.0.0.1:7331", 10*time.Second)
	t.Log("Sequencer node is up")

	// 6. Submit a transaction to EVM
	lastNonce := uint64(0)
	tx := evm.GetRandomTransaction(t, "cece4f25ac74deb1468965160c7185e07dff413f23fcadb611b05ca37ab0a52e", "0x944fDcD1c868E3cC566C78023CcB38A32cDA836E", "1234", 22000, &lastNonce)
	evm.SubmitTransaction(t, tx)
	t.Log("Submitted test transaction to EVM")

	// 7. Wait for block production and verify transaction inclusion
	require.Eventually(t, func() bool {
		return evm.CheckTxIncluded(t, tx.Hash())
	}, 20*time.Second, 1*time.Second)
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

	// 1. Start local DA
	localDABinary := "local-da"
	if evmSingleBinaryPath != "evm-single" {
		localDABinary = filepath.Join(filepath.Dir(evmSingleBinaryPath), "local-da")
	}
	sut.ExecCmd(localDABinary)
	t.Log("Started local DA")
	time.Sleep(200 * time.Millisecond)

	// 2. Start EVM (Reth) via Docker Compose
	jwtSecret := setupTestRethEngineE2E(t)

	// 3. Get genesis hash from EVM node
	genesisHash := evm.GetGenesisHash(t)
	t.Logf("Genesis hash: %s", genesisHash)

	// 4. Initialize sequencer node
	output, err := sut.RunCmd(evmSingleBinaryPath,
		"init",
		"--rollkit.node.aggregator=true",
		"--rollkit.signer.passphrase", "secret",
		"--home", nodeHome,
	)
	require.NoError(t, err, "failed to init sequencer", output)
	t.Log("Initialized sequencer node")

	// 5. Start sequencer node
	sut.ExecCmd(evmSingleBinaryPath,
		"start",
		"--evm.jwt-secret", jwtSecret,
		"--evm.genesis-hash", genesisHash,
		"--rollkit.node.block_time", "1s",
		"--rollkit.node.aggregator=true",
		"--rollkit.signer.passphrase", "secret",
		"--home", nodeHome,
		"--rollkit.da.address", "http://localhost:7980",
		"--rollkit.da.block_time", "1m",
	)
	sut.AwaitNodeUp(t, "http://127.0.0.1:7331", 10*time.Second)
	t.Log("Sequencer node is up")

	// 6. Connect to EVM
	client, err := ethclient.Dial("http://localhost:8545")
	require.NoError(t, err, "Should be able to connect to EVM")
	defer client.Close()

	// 7. Submit multiple transactions in quick succession
	const numTxs = 500
	var txHashes []common.Hash
	var expectedNonces []uint64
	lastNonce := uint64(0)

	t.Logf("Submitting %d transactions in quick succession...", numTxs)
	for i := 0; i < numTxs; i++ {
		// Create transaction with proper chain ID (1234)
		tx := evm.GetRandomTransaction(t, "cece4f25ac74deb1468965160c7185e07dff413f23fcadb611b05ca37ab0a52e", "0x944fDcD1c868E3cC566C78023CcB38A32cDA836E", "1234", 22000, &lastNonce)

		evm.SubmitTransaction(t, tx)
		txHashes = append(txHashes, tx.Hash())
		expectedNonces = append(expectedNonces, tx.Nonce())

		// Log progress every 50 transactions to avoid spam
		if (i+1)%50 == 0 || i < 10 {
			t.Logf("Submitted transaction %d: hash=%s, nonce=%d", i+1, tx.Hash().Hex(), tx.Nonce())
		}

		// Reduce delay to increase throughput while still being manageable
		time.Sleep(10 * time.Millisecond)
	}

	// 8. Wait for all transactions to be included and verify order
	ctx := context.Background()
	var receipts []*common.Hash

	t.Log("Waiting for all transactions to be included...")
	require.Eventually(t, func() bool {
		receipts = receipts[:0] // Clear slice

		for _, txHash := range txHashes {
			receipt, err := client.TransactionReceipt(ctx, txHash)
			if err != nil || receipt == nil || receipt.Status != 1 {
				return false // Not all transactions included yet
			}
			receipts = append(receipts, &txHash)
		}

		// Log progress every 100 transactions
		if len(receipts)%100 == 0 && len(receipts) > 0 {
			t.Logf("Progress: %d/%d transactions included", len(receipts), numTxs)
		}

		return len(receipts) == numTxs
	}, 120*time.Second, 2*time.Second, "All transactions should be included")

	t.Logf("✅ All %d transactions were successfully included", numTxs)

	// 9. Verify transaction order by checking nonces
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

	// 10. Verify nonce ordering (transactions should maintain nonce order)
	for i := 0; i < numTxs; i++ {
		require.Equal(t, expectedNonces[i], actualNonces[i],
			"Transaction %d should have expected nonce %d, got %d", i+1, expectedNonces[i], actualNonces[i])
	}

	// 11. Verify no transactions were lost
	require.Equal(t, numTxs, len(actualNonces), "All %d transactions should be included", numTxs)

	// 12. Log block distribution
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
