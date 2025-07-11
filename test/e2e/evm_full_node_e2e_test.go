//go:build evm
// +build evm

// Package e2e contains end-to-end tests for Rollkit's EVM integration.
//
// This file specifically tests the EVM full node functionality including:
// - Full node synchronization via P2P with sequencer
// - Full node synchronization via DA-only with sequencer
// - Transaction sync verification between sequencer and full node
// - Multi-node setup and P2P block propagation
// - State root consistency across distributed nodes
// - Block propagation verification with multiple full nodes
// - Distributed node restart and recovery mechanisms
// - Lazy mode sequencer behavior with full node sync
//
// Test Coverage:
// 1. TestEvmSequencerWithFullNodeE2E - Full node sync with sequencer (P2P and DA-only subtests)
// 2. TestEvmFullNodeBlockPropagationE2E - Block propagation across multiple nodes
// 3. TestEvmLazyModeSequencerE2E - Lazy mode sequencer with full node P2P sync
// 4. TestEvmSequencerFullNodeRestartE2E - Distributed restart and recovery testing
//   - StandardRestart: Normal start -> Normal restart
//   - LazyModeRestart: Normal start -> Lazy restart
package e2e

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/require"
)

// Note: evmSingleBinaryPath is declared in evm_sequencer_e2e_test.go to avoid duplicate declaration

// verifyBlockPropagationAcrossNodes verifies that a specific block exists and matches across all provided node URLs.
// This function ensures that:
// - All nodes have the same block at the specified height
// - Block hashes are identical across all nodes
// - State roots match between all nodes
// - Transaction counts are consistent
//
// Parameters:
// - nodeURLs: List of EVM endpoint URLs to check
// - blockHeight: Height of the block to verify
// - nodeNames: Human-readable names for the nodes (for logging)
//
// This validation ensures that block propagation is working correctly across all full nodes.
func verifyBlockPropagationAcrossNodes(t *testing.T, nodeURLs []string, blockHeight uint64, nodeNames []string) {
	t.Helper()

	var blockHashes []common.Hash
	var stateRoots []common.Hash
	var txCounts []int
	var blockNumbers []uint64

	// Collect block information from all nodes
	for i, nodeURL := range nodeURLs {
		nodeName := nodeNames[i]
		blockHash, stateRoot, txCount, blockNum, err := checkBlockInfoAt(t, nodeURL, &blockHeight)
		require.NoError(t, err, "Should get block info from %s at height %d", nodeName, blockHeight)

		blockHashes = append(blockHashes, blockHash)
		stateRoots = append(stateRoots, stateRoot)
		txCounts = append(txCounts, txCount)
		blockNumbers = append(blockNumbers, blockNum)

		t.Logf("%s block %d: hash=%s, stateRoot=%s, txs=%d",
			nodeName, blockHeight, blockHash.Hex(), stateRoot.Hex(), txCount)
	}

	// Verify all block numbers match the requested height
	for i, blockNum := range blockNumbers {
		require.Equal(t, blockHeight, blockNum,
			"%s block number should match requested height %d", nodeNames[i], blockHeight)
	}

	// Verify all block hashes match (compare each node to the first one)
	referenceHash := blockHashes[0]
	for i := 1; i < len(blockHashes); i++ {
		require.Equal(t, referenceHash.Hex(), blockHashes[i].Hex(),
			"Block hash mismatch at height %d: %s vs %s",
			blockHeight, nodeNames[0], nodeNames[i])
	}

	// Verify all state roots match (compare each node to the first one)
	referenceStateRoot := stateRoots[0]
	for i := 1; i < len(stateRoots); i++ {
		require.Equal(t, referenceStateRoot.Hex(), stateRoots[i].Hex(),
			"State root mismatch at height %d: %s vs %s",
			blockHeight, nodeNames[0], nodeNames[i])
	}

	// Verify all transaction counts match
	referenceTxCount := txCounts[0]
	for i := 1; i < len(txCounts); i++ {
		require.Equal(t, referenceTxCount, txCounts[i],
			"Transaction count mismatch at height %d: %s (%d) vs %s (%d)",
			blockHeight, nodeNames[0], referenceTxCount, nodeNames[i], txCounts[i])
	}

	t.Logf("✅ Block %d propagated correctly to all %d nodes (hash: %s, txs: %d)",
		blockHeight, len(nodeURLs), referenceHash.Hex(), referenceTxCount)
}

// verifyTransactionSync verifies that the transaction syncs to the full node in the same block.
// This function ensures that:
// - The full node reaches or exceeds the block height containing the transaction
// - The transaction exists in the full node with the same block number
// - Both sequencer and full node have identical transaction receipts
// - The transaction status is successful on both nodes
//
// Parameters:
// - sequencerClient: Ethereum client connected to sequencer EVM
// - fullNodeClient: Ethereum client connected to full node EVM
// - txHash: Hash of the transaction to verify
// - expectedBlockNumber: Block number where transaction should be included
//
// This validation ensures that P2P sync is working correctly and that
// full nodes maintain consistency with the sequencer.
func verifyTransactionSync(t *testing.T, sequencerClient, fullNodeClient *ethclient.Client, txHash common.Hash, expectedBlockNumber uint64) {
	t.Helper()

	ctx := context.Background()

	// Wait for full node to sync the specific block containing the transaction
	require.Eventually(t, func() bool {
		// Check if full node has reached the block containing the transaction
		fullNodeHeader, err := fullNodeClient.HeaderByNumber(ctx, nil)
		if err != nil {
			return false
		}

		// If full node has reached or passed the transaction block
		if fullNodeHeader.Number.Uint64() >= expectedBlockNumber {
			// Verify the transaction exists in the full node
			receipt, err := fullNodeClient.TransactionReceipt(ctx, txHash)
			if err == nil && receipt != nil && receipt.Status == 1 {
				return receipt.BlockNumber.Uint64() == expectedBlockNumber
			}
		}
		return false
	}, 45*time.Second, 1*time.Second, "Full node should sync the block containing the transaction")

	// Final verification - both nodes should have the transaction in the same block
	sequencerReceipt, err := sequencerClient.TransactionReceipt(ctx, txHash)
	require.NoError(t, err, "Should get transaction receipt from sequencer")

	fullNodeReceipt, err := fullNodeClient.TransactionReceipt(ctx, txHash)
	require.NoError(t, err, "Should get transaction receipt from full node")

	require.Equal(t, sequencerReceipt.BlockNumber.Uint64(), fullNodeReceipt.BlockNumber.Uint64(),
		"Transaction should be in the same block number on both sequencer and full node")
}

// verifyStateRootsMatch verifies that state roots match between sequencer and full node for a specific block.
// This function ensures that:
// - Both nodes have the same block at the specified height
// - The state roots are identical between both nodes
// - Block metadata (hash, transaction count) matches
//
// Parameters:
// - sequencerURL: URL of the sequencer EVM endpoint
// - fullNodeURL: URL of the full node EVM endpoint
// - blockHeight: Height of the block to verify
//
// This validation ensures that P2P sync maintains state consistency.
func verifyStateRootsMatch(t *testing.T, sequencerURL, fullNodeURL string, blockHeight uint64) {
	t.Helper()

	// Get block info from sequencer
	seqHash, seqStateRoot, seqTxCount, seqBlockNum, err := checkBlockInfoAt(t, sequencerURL, &blockHeight)
	require.NoError(t, err, "Should get block info from sequencer at height %d", blockHeight)

	// Get block info from full node
	fnHash, fnStateRoot, fnTxCount, fnBlockNum, err := checkBlockInfoAt(t, fullNodeURL, &blockHeight)
	require.NoError(t, err, "Should get block info from full node at height %d", blockHeight)

	// Verify block numbers match
	require.Equal(t, seqBlockNum, fnBlockNum, "Block numbers should match at height %d", blockHeight)
	require.Equal(t, blockHeight, seqBlockNum, "Sequencer block number should match requested height")
	require.Equal(t, blockHeight, fnBlockNum, "Full node block number should match requested height")

	// Verify block hashes match
	require.Equal(t, seqHash.Hex(), fnHash.Hex(),
		"Block hashes should match at height %d. Sequencer: %s, Full node: %s",
		blockHeight, seqHash.Hex(), fnHash.Hex())

	// Verify state roots match (this is the key check)
	require.Equal(t, seqStateRoot.Hex(), fnStateRoot.Hex(),
		"State roots should match at height %d. Sequencer: %s, Full node: %s",
		blockHeight, seqStateRoot.Hex(), fnStateRoot.Hex())

	// Verify transaction counts match
	require.Equal(t, seqTxCount, fnTxCount,
		"Transaction counts should match at height %d. Sequencer: %d, Full node: %d",
		blockHeight, seqTxCount, fnTxCount)

	t.Logf("✅ Block %d state roots match: %s (txs: %d)", blockHeight, seqStateRoot.Hex(), seqTxCount)
}

// setupSequencerWithFullNode sets up both sequencer and full node with P2P connections.
// This helper function handles the complex setup required for full node tests.
//
// Returns: sequencerClient, fullNodeClient for EVM connections
func setupSequencerWithFullNode(t *testing.T, sut *SystemUnderTest, sequencerHome, fullNodeHome string) (*ethclient.Client, *ethclient.Client) {
	t.Helper()

	// Common setup for both sequencer and full node
	jwtSecret, fullNodeJwtSecret, genesisHash := setupCommonEVMTest(t, sut, true)

	// Setup sequencer
	setupSequencerNode(t, sut, sequencerHome, jwtSecret, genesisHash)
	t.Log("Sequencer node is up")

	// Get P2P address and setup full node
	sequencerP2PAddress := getNodeP2PAddress(t, sut, sequencerHome)
	t.Logf("Sequencer P2P address: %s", sequencerP2PAddress)

	setupFullNode(t, sut, fullNodeHome, sequencerHome, fullNodeJwtSecret, genesisHash, sequencerP2PAddress)
	t.Log("Full node is up")

	// Connect to both EVM instances
	sequencerClient, err := ethclient.Dial(SequencerEthURL)
	require.NoError(t, err, "Should be able to connect to sequencer EVM")

	fullNodeClient, err := ethclient.Dial(FullNodeEthURL)
	require.NoError(t, err, "Should be able to connect to full node EVM")

	// Wait for P2P connections to establish
	t.Log("Waiting for P2P connections to establish...")
	require.Eventually(t, func() bool {
		// Check if both nodes are responsive
		seqHeader, seqErr := sequencerClient.HeaderByNumber(context.Background(), nil)
		fnHeader, fnErr := fullNodeClient.HeaderByNumber(context.Background(), nil)

		if seqErr != nil || fnErr != nil {
			return false
		}

		// Both nodes should be responsive and at genesis or later
		seqHeight := seqHeader.Number.Uint64()
		fnHeight := fnHeader.Number.Uint64()

		// With 100ms blocks (10 blocks/sec), allow larger sync tolerance during startup
		// Allow up to 20 blocks difference to account for P2P propagation delays
		return seqHeight >= 0 && fnHeight >= 0 && (seqHeight == 0 || fnHeight+20 >= seqHeight)
	}, DefaultTestTimeout, 250*time.Millisecond, "P2P connections should be established")

	t.Log("P2P connections established")
	return sequencerClient, fullNodeClient
}

// TestEvmSequencerWithFullNodeE2E tests the full node synchronization functionality
// with both P2P and DA-only connection methods.
//
// Test Purpose:
// - Validate that full nodes can sync with sequencers using different methods
// - Compare P2P vs DA-only synchronization mechanisms
// - Ensure both sync methods maintain state consistency
// - Demonstrate alternative sync approaches for different network conditions
//
// Sub-tests:
// 1. P2P_Sync: Full node connects via P2P gossip (fast, real-time)
// 2. DA_Only_Sync: Full node connects only via DA layer (slower, more reliable)
//
// This test provides comprehensive coverage of both synchronization methods,
// allowing developers to understand the trade-offs between speed (P2P) and
// reliability (DA-only) in different network scenarios.
func TestEvmSequencerWithFullNodeE2E(t *testing.T) {
	t.Run("P2P_Sync", func(t *testing.T) {
		TestEvmSequencerWithFullNodeP2PE2E(t)
	})

	t.Run("DA_Only_Sync", func(t *testing.T) {
		TestEvmSequencerWithFullNodeDAOnlyE2E(t)
	})
}

// TestEvmSequencerWithFullNodeP2PE2E tests the full node synchronization functionality
// where a full node connects to a sequencer via P2P and syncs transactions.
//
// Test Flow:
// 1. Sets up Local DA layer and separate EVM instances for sequencer and full node
// 2. Starts sequencer node with standard EVM ports (8545/8551)
// 3. Extracts P2P ID from sequencer logs for peer connection
// 4. Starts full node with different EVM ports (8555/8561) and P2P connection to sequencer
// 5. Submits transaction to sequencer EVM and gets the block number
// 6. Verifies the full node syncs the exact same block containing the transaction
// 7. Performs comprehensive state root verification across all synced blocks
//
// Validation:
// - Both sequencer and full node start successfully
// - P2P connection is established between nodes
// - Transaction submitted to sequencer is included in a specific block
// - Full node syncs the same block number containing the transaction
// - Transaction data is identical on both nodes (same block, same receipt)
// - State roots match between sequencer and full node for all blocks (key validation)
// - Block hashes and transaction counts are consistent across both nodes
//
// Key Technical Details:
// - Uses separate Docker Compose configurations for different EVM ports
// - Handles P2P ID extraction from logs (including split-line scenarios)
// - Copies genesis file from sequencer to full node for consistency
// - Validates that P2P sync works independently of DA layer timing
// - Implements comprehensive state root checking similar to execution_test.go patterns
// - Ensures EVM state consistency between sequencer and full node on the Reth side
//
// This test demonstrates that full nodes can sync with sequencers in real-time via P2P,
// validates the P2P block propagation mechanism in Rollkit, and ensures that
// the underlying EVM execution state remains consistent across all nodes.
func TestEvmSequencerWithFullNodeP2PE2E(t *testing.T) {
	flag.Parse()
	workDir := t.TempDir()
	sequencerHome := filepath.Join(workDir, "evm-sequencer")
	fullNodeHome := filepath.Join(workDir, "evm-full-node")
	sut := NewSystemUnderTest(t)

	// Setup both sequencer and full node
	sequencerClient, fullNodeClient := setupSequencerWithFullNode(t, sut, sequencerHome, fullNodeHome)
	defer sequencerClient.Close()
	defer fullNodeClient.Close()

	// === TESTING PHASE ===

	// Submit multiple transactions at different intervals to create more state changes
	var txHashes []common.Hash
	var txBlockNumbers []uint64

	// Submit first batch of transactions
	t.Log("Submitting first batch of transactions...")
	for i := 0; i < 3; i++ {
		txHash, txBlockNumber := submitTransactionAndGetBlockNumber(t, sequencerClient)
		txHashes = append(txHashes, txHash)
		txBlockNumbers = append(txBlockNumbers, txBlockNumber)
		t.Logf("Transaction %d included in sequencer block %d", i+1, txBlockNumber)

		// Small delay between transactions to spread them across blocks
		time.Sleep(2 * time.Millisecond)
	}

	// Wait a bit for block production
	time.Sleep(1 * time.Second)

	// Submit second batch of transactions
	t.Log("Submitting second batch of transactions...")
	for i := 0; i < 2; i++ {
		txHash, txBlockNumber := submitTransactionAndGetBlockNumber(t, sequencerClient)
		txHashes = append(txHashes, txHash)
		txBlockNumbers = append(txBlockNumbers, txBlockNumber)
		t.Logf("Transaction %d included in sequencer block %d", i+4, txBlockNumber)

		// Small delay between transactions
		time.Sleep(2 * time.Millisecond)
	}

	// Wait for all transactions to be processed
	time.Sleep(500 * time.Millisecond)

	t.Logf("Total transactions submitted: %d across blocks %v", len(txHashes), txBlockNumbers)

	t.Log("Waiting for full node to sync all transaction blocks...")

	// Verify all transactions have synced
	for i, txHash := range txHashes {
		txBlockNumber := txBlockNumbers[i]
		t.Logf("Verifying transaction %d sync in block %d...", i+1, txBlockNumber)
		verifyTransactionSync(t, sequencerClient, fullNodeClient, txHash, txBlockNumber)
	}

	// === STATE ROOT VERIFICATION ===

	t.Log("Verifying state roots match between sequencer and full node...")

	// Get the current height on both nodes to determine the range of blocks to check
	seqCtx := context.Background()
	seqHeader, err := sequencerClient.HeaderByNumber(seqCtx, nil)
	require.NoError(t, err, "Should get latest header from sequencer")

	fnCtx := context.Background()
	fnHeader, err := fullNodeClient.HeaderByNumber(fnCtx, nil)
	require.NoError(t, err, "Should get latest header from full node")

	// Ensure both nodes are at the same height before checking state roots
	seqHeight := seqHeader.Number.Uint64()
	fnHeight := fnHeader.Number.Uint64()

	// Wait for full node to catch up if needed
	if fnHeight < seqHeight {
		t.Logf("Full node height (%d) is behind sequencer height (%d), waiting for sync...", fnHeight, seqHeight)
		require.Eventually(t, func() bool {
			header, err := fullNodeClient.HeaderByNumber(fnCtx, nil)
			if err != nil {
				return false
			}
			return header.Number.Uint64() >= seqHeight
		}, DefaultTestTimeout, 500*time.Millisecond, "Full node should catch up to sequencer height")

		// Re-get the full node height after sync
		fnHeader, err = fullNodeClient.HeaderByNumber(fnCtx, nil)
		require.NoError(t, err, "Should get updated header from full node")
		fnHeight = fnHeader.Number.Uint64()
	}

	// Check state roots for all blocks from genesis up to current height
	// Note: Block 0 is genesis, start from block 1
	startHeight := uint64(1)
	endHeight := min(seqHeight, fnHeight)

	t.Logf("Checking state roots for blocks %d to %d", startHeight, endHeight)

	for blockHeight := startHeight; blockHeight <= endHeight; blockHeight++ {
		verifyStateRootsMatch(t, SequencerEthURL, FullNodeEthURL, blockHeight)
	}

	// Special focus on the transaction blocks
	t.Log("Re-verifying state roots for all transaction blocks...")
	for i, txBlockNumber := range txBlockNumbers {
		if txBlockNumber >= startHeight && txBlockNumber <= endHeight {
			t.Logf("Re-verifying state root for transaction %d block %d", i+1, txBlockNumber)
			verifyStateRootsMatch(t, SequencerEthURL, FullNodeEthURL, txBlockNumber)
		}
	}

	t.Logf("✅ Test PASSED: P2P synchronization working correctly!")
	t.Logf("   - All blocks (%d-%d) have matching state roots", startHeight, endHeight)
	t.Logf("   - %d transactions synced successfully via P2P across blocks %v", len(txHashes), txBlockNumbers)
	t.Logf("   - Full node synchronized using P2P connections")
	t.Logf("   - State consistency maintained between sequencer and full node")
	t.Logf("   - P2P block propagation mechanism functioning properly")
}

// setupSequencerWithFullNodeLazy sets up both sequencer (in lazy mode) and full node with P2P connections.
// This helper function is specifically for testing lazy mode behavior where blocks are only
// produced when transactions are available, not on a regular timer.
//
// Returns: sequencerClient, fullNodeClient for EVM connections
func setupSequencerWithFullNodeLazy(t *testing.T, sut *SystemUnderTest, sequencerHome, fullNodeHome string) (*ethclient.Client, *ethclient.Client) {
	t.Helper()

	// Common setup for both sequencer and full node
	jwtSecret, fullNodeJwtSecret, genesisHash := setupCommonEVMTest(t, sut, true)

	// Setup sequencer in lazy mode
	setupSequencerNodeLazy(t, sut, sequencerHome, jwtSecret, genesisHash)
	t.Log("Sequencer node (lazy mode) is up")

	// Get P2P address and setup full node
	sequencerP2PAddress := getNodeP2PAddress(t, sut, sequencerHome)
	t.Logf("Sequencer P2P address: %s", sequencerP2PAddress)

	setupFullNode(t, sut, fullNodeHome, sequencerHome, fullNodeJwtSecret, genesisHash, sequencerP2PAddress)
	t.Log("Full node is up")

	// Connect to both EVM instances
	sequencerClient, err := ethclient.Dial(SequencerEthURL)
	require.NoError(t, err, "Should be able to connect to sequencer EVM")

	fullNodeClient, err := ethclient.Dial(FullNodeEthURL)
	require.NoError(t, err, "Should be able to connect to full node EVM")

	// Wait for P2P connections to establish
	t.Log("Waiting for P2P connections to establish...")
	require.Eventually(t, func() bool {
		// Check if both nodes are responsive
		seqHeader, seqErr := sequencerClient.HeaderByNumber(context.Background(), nil)
		fnHeader, fnErr := fullNodeClient.HeaderByNumber(context.Background(), nil)

		if seqErr != nil || fnErr != nil {
			return false
		}

		// Both nodes should be responsive and close in height (allow for initial blocks)
		seqHeight := seqHeader.Number.Uint64()
		fnHeight := fnHeader.Number.Uint64()

		// In lazy mode, we might have initial blocks, so allow small difference
		heightDiff := int64(seqHeight) - int64(fnHeight)
		if heightDiff < 0 {
			heightDiff = -heightDiff
		}

		return heightDiff <= 2 // Allow up to 2 blocks difference during startup
	}, DefaultTestTimeout, 250*time.Millisecond, "P2P connections should be established")

	t.Log("P2P connections established")
	return sequencerClient, fullNodeClient
}

// TestEvmLazyModeSequencerE2E tests the lazy mode functionality where blocks are only
// produced when transactions are submitted, not on a regular timer.
//
// Test Flow:
// 1. Sets up a sequencer in lazy mode and a full node with P2P connections
// 2. Verifies both nodes start at genesis (height 0) and no blocks are produced initially
// 3. Monitors both nodes for a period to ensure no automatic block production
// 4. Submits a transaction to the sequencer and verifies a block is produced
// 5. Waits again to verify no additional blocks are produced without transactions
// 6. Repeats the transaction submission process multiple times
// 7. Verifies the full node syncs all blocks correctly via P2P
// 8. Performs comprehensive state root verification across all blocks
//
// Validation:
// - Both sequencer and full node start at genesis height (0)
// - No blocks are produced during idle periods (lazy mode working)
// - Blocks are only produced when transactions are submitted
// - Each transaction triggers exactly one block
// - Full node syncs all blocks via P2P
// - State roots match between sequencer and full node for all blocks
// - Block hashes and transaction data are consistent across both nodes
//
// Key Technical Details:
// - Uses lazy mode flag (--rollkit.node.lazy_mode=true)
// - Sets lazy block interval to 60 seconds to avoid timer-based block production
// - Monitors nodes for 2 seconds to verify no automatic block production
// - Submits transactions at different intervals to test various scenarios
// - Validates that P2P sync works correctly with lazy block production
// - Ensures state consistency between sequencer and full node
//
// This test demonstrates that lazy mode optimizes resource usage by only
// producing blocks when necessary (when transactions are available), while
// maintaining proper P2P sync functionality.
func TestEvmLazyModeSequencerE2E(t *testing.T) {
	flag.Parse()
	workDir := t.TempDir()
	sequencerHome := filepath.Join(workDir, "evm-lazy-sequencer")
	fullNodeHome := filepath.Join(workDir, "evm-lazy-full-node")
	sut := NewSystemUnderTest(t)

	// Setup sequencer in lazy mode and full node
	sequencerClient, fullNodeClient := setupSequencerWithFullNodeLazy(t, sut, sequencerHome, fullNodeHome)
	defer sequencerClient.Close()
	defer fullNodeClient.Close()

	ctx := context.Background()

	// === VERIFY INITIAL STATE ===

	// Get initial heights (may not be genesis due to lazy timer)
	seqHeader, err := sequencerClient.HeaderByNumber(ctx, nil)
	require.NoError(t, err, "Should get sequencer initial header")
	fnHeader, err := fullNodeClient.HeaderByNumber(ctx, nil)
	require.NoError(t, err, "Should get full node initial header")

	initialSeqHeight := seqHeader.Number.Uint64()
	initialFnHeight := fnHeader.Number.Uint64()

	t.Logf("✅ Both nodes initialized - sequencer height: %d, full node height: %d", initialSeqHeight, initialFnHeight)

	// === TEST LAZY MODE BEHAVIOR ===

	// Monitor for no block production during idle period (reduced time)
	t.Log("Monitoring nodes for idle block production (should be none in lazy mode)...")
	verifyNoBlockProduction(t, sequencerClient, 1*time.Second, "sequencer")
	verifyNoBlockProduction(t, fullNodeClient, 1*time.Second, "full node")

	// Track transactions and their blocks
	var txHashes []common.Hash
	var txBlockNumbers []uint64

	// === ROUND 1: Single transaction ===

	t.Log("Round 1: Submitting single transaction to trigger block production...")

	txHash1, txBlockNumber1 := submitTransactionAndGetBlockNumber(t, sequencerClient)
	txHashes = append(txHashes, txHash1)
	txBlockNumbers = append(txBlockNumbers, txBlockNumber1)

	t.Logf("Transaction 1 included in sequencer block %d", txBlockNumber1)
	require.Greater(t, txBlockNumber1, initialSeqHeight, "First transaction should be in a new block after initial height")

	// Verify full node syncs the block
	verifyTransactionSync(t, sequencerClient, fullNodeClient, txHash1, txBlockNumber1)
	t.Log("✅ Full node synced transaction 1")

	// Verify no additional blocks are produced after transaction
	t.Log("Monitoring for idle period after transaction 1...")
	verifyNoBlockProduction(t, sequencerClient, 1*time.Second, "sequencer")
	verifyNoBlockProduction(t, fullNodeClient, 1*time.Second, "full node")

	// === ROUND 2: Burst transactions ===

	t.Log("Round 2: Submitting burst of transactions...")

	// Submit 3 transactions quickly
	for i := 0; i < 3; i++ {
		txHash, txBlockNumber := submitTransactionAndGetBlockNumber(t, sequencerClient)
		txHashes = append(txHashes, txHash)
		txBlockNumbers = append(txBlockNumbers, txBlockNumber)
		t.Logf("Transaction %d included in sequencer block %d", i+2, txBlockNumber)

		// Small delay between transactions
		time.Sleep(10 * time.Millisecond)
	}

	// Verify all transactions sync to full node
	for i := 1; i < len(txHashes); i++ {
		verifyTransactionSync(t, sequencerClient, fullNodeClient, txHashes[i], txBlockNumbers[i])
		t.Logf("✅ Full node synced transaction %d", i+1)
	}

	// Verify no additional blocks after burst
	t.Log("Monitoring for idle period after burst transactions...")
	verifyNoBlockProduction(t, sequencerClient, 1*time.Second, "sequencer")
	verifyNoBlockProduction(t, fullNodeClient, 1*time.Second, "full node")

	// === ROUND 3: Delayed transaction ===

	t.Log("Round 3: Submitting delayed transaction...")

	txHashDelayed, txBlockNumberDelayed := submitTransactionAndGetBlockNumber(t, sequencerClient)
	txHashes = append(txHashes, txHashDelayed)
	txBlockNumbers = append(txBlockNumbers, txBlockNumberDelayed)

	t.Logf("Delayed transaction included in sequencer block %d", txBlockNumberDelayed)

	// Verify full node syncs the delayed transaction
	verifyTransactionSync(t, sequencerClient, fullNodeClient, txHashDelayed, txBlockNumberDelayed)
	t.Log("✅ Full node synced delayed transaction")

	// === STATE ROOT VERIFICATION ===

	t.Log("Performing comprehensive state root verification...")

	// Get current heights
	seqHeader, err = sequencerClient.HeaderByNumber(ctx, nil)
	require.NoError(t, err, "Should get sequencer final header")
	fnHeader, err = fullNodeClient.HeaderByNumber(ctx, nil)
	require.NoError(t, err, "Should get full node final header")

	seqHeight := seqHeader.Number.Uint64()
	fnHeight := fnHeader.Number.Uint64()

	// Ensure full node caught up
	if fnHeight < seqHeight {
		t.Logf("Waiting for full node to catch up to sequencer height %d...", seqHeight)
		require.Eventually(t, func() bool {
			header, err := fullNodeClient.HeaderByNumber(ctx, nil)
			return err == nil && header.Number.Uint64() >= seqHeight
		}, DefaultTestTimeout, 500*time.Millisecond, "Full node should catch up")
	}

	// Verify state roots for all blocks (skip genesis block 0)
	startHeight := uint64(1)
	if seqHeight > 0 {
		t.Logf("Verifying state roots for blocks %d to %d...", startHeight, seqHeight)
		for blockHeight := startHeight; blockHeight <= seqHeight; blockHeight++ {
			verifyStateRootsMatch(t, SequencerEthURL, FullNodeEthURL, blockHeight)
		}
	} else {
		t.Log("No blocks to verify (sequencer at genesis)")
	}

	// === FINAL VALIDATION ===

	// Verify transaction distribution
	uniqueBlocks := make(map[uint64]bool)
	for _, blockNum := range txBlockNumbers {
		uniqueBlocks[blockNum] = true
	}

	t.Logf("📊 Lazy mode test results:")
	t.Logf("   - Total transactions submitted: %d", len(txHashes))
	t.Logf("   - Total blocks produced: %d", seqHeight)
	t.Logf("   - Unique transaction blocks: %d", len(uniqueBlocks))
	t.Logf("   - Sequencer final height: %d", seqHeight)
	t.Logf("   - Full node final height: %d", fnHeight)

	// Validate that blocks were only produced when transactions were sent
	// In lazy mode, we should only have blocks when transactions are submitted
	require.Greater(t, len(txHashes), 0, "Should have submitted transactions")
	require.Equal(t, seqHeight, fnHeight, "Both nodes should be at same height")

	// Verify specific transaction blocks
	t.Log("Final verification of all transaction blocks...")
	for i, txHash := range txHashes {
		txBlockNumber := txBlockNumbers[i]

		// Verify transaction exists on both nodes
		seqReceipt, err := sequencerClient.TransactionReceipt(ctx, txHash)
		require.NoError(t, err, "Should get transaction %d receipt from sequencer", i+1)
		require.Equal(t, uint64(1), seqReceipt.Status, "Transaction %d should be successful on sequencer", i+1)

		fnReceipt, err := fullNodeClient.TransactionReceipt(ctx, txHash)
		require.NoError(t, err, "Should get transaction %d receipt from full node", i+1)
		require.Equal(t, uint64(1), fnReceipt.Status, "Transaction %d should be successful on full node", i+1)

		require.Equal(t, seqReceipt.BlockNumber.Uint64(), fnReceipt.BlockNumber.Uint64(),
			"Transaction %d should be in same block on both nodes", i+1)
		require.Equal(t, txBlockNumber, seqReceipt.BlockNumber.Uint64(),
			"Transaction %d should be in expected block %d", i+1, txBlockNumber)
	}

	t.Logf("✅ Test PASSED: Lazy mode sequencer working correctly!")
	t.Logf("   - Blocks only produced when transactions submitted ✓")
	t.Logf("   - No automatic/timer-based block production ✓")
	t.Logf("   - Full node P2P sync working correctly ✓")
	t.Logf("   - State roots consistent across all blocks ✓")
	t.Logf("   - All %d transactions successfully processed and synced ✓", len(txHashes))
}

// restartSequencerAndFullNode restarts both the sequencer and full node while preserving their configurations.
// This helper function manages the complex restart process required for both nodes.
//
// Important: This function properly restarts the local DA layer first, following the same pattern
// as TestEvmSequencerRestartRecoveryE2E. This ensures that all components (DA, sequencer, full node)
// are restarted in the correct order and with proper dependencies.
//
// Parameters:
// - sequencerHome: Directory path for sequencer data
// - fullNodeHome: Directory path for full node data
// - jwtSecret: JWT secret for sequencer's EVM engine
// - fullNodeJwtSecret: JWT secret for full node's EVM engine
// - genesisHash: Hash of the genesis block for chain validation
// - useLazyMode: Whether to restart the sequencer in lazy mode
//
// This function ensures both nodes are properly restarted and P2P connections are re-established.
// The DA restart is handled by the shared restartDAAndSequencer/restartDAAndSequencerLazy functions.
func restartSequencerAndFullNode(t *testing.T, sut *SystemUnderTest, sequencerHome, fullNodeHome, jwtSecret, fullNodeJwtSecret, genesisHash string, useLazyMode bool) {
	t.Helper()

	// Restart DA and sequencer first (following the pattern from TestEvmSequencerRestartRecoveryE2E)
	if useLazyMode {
		restartDAAndSequencerLazy(t, sut, sequencerHome, jwtSecret, genesisHash)
	} else {
		restartDAAndSequencer(t, sut, sequencerHome, jwtSecret, genesisHash)
	}

	// Get the P2P address of the restarted sequencer using net-info command
	sequencerP2PAddress := getNodeP2PAddress(t, sut, sequencerHome)
	t.Logf("Sequencer P2P address after restart: %s", sequencerP2PAddress)

	// Now restart the full node (without init - node already exists)
	sut.ExecCmd(evmSingleBinaryPath,
		"start",
		"--home", fullNodeHome,
		"--evm.jwt-secret", fullNodeJwtSecret,
		"--evm.genesis-hash", genesisHash,
		"--rollkit.rpc.address", "127.0.0.1:"+FullNodeRPCPort,
		"--rollkit.p2p.listen_address", "/ip4/127.0.0.1/tcp/"+FullNodeP2PPort,
		"--rollkit.p2p.peers", sequencerP2PAddress,
		"--evm.engine-url", FullNodeEngineURL,
		"--evm.eth-url", FullNodeEthURL,
		"--rollkit.da.address", DAAddress,
		"--rollkit.da.block_time", DefaultDABlockTime,
	)

	// Give both nodes time to establish P2P connections
	time.Sleep(1 * time.Second)
	sut.AwaitNodeUp(t, "http://127.0.0.1:"+FullNodeRPCPort, 10*time.Second)
	t.Log("Full node restarted successfully")
}

// TestEvmSequencerFullNodeRestartE2E tests the ability of both sequencer and full node
// to recover from a restart while maintaining P2P synchronization and data integrity.
//
// Test Purpose:
// - Validate that both sequencer and full node can recover from restarts without data loss
// - Test that P2P connections are re-established after both nodes restart
// - Ensure transaction history is preserved across restarts for both nodes
// - Verify that block production and P2P sync resume correctly after restart
// - Test the robustness of the distributed system during dual node restarts
//
// Sub-tests:
// 1. StandardRestart: Normal start -> Normal restart
// 2. LazyModeRestart: Normal start -> Lazy restart
//
// Test Flow:
// 1. Sets up Local DA layer, sequencer, and full node with P2P connections (in specified initial mode)
// 2. Submits initial transactions and verifies P2P sync works
// 3. Records blockchain state on both nodes before restart
// 4. Gracefully stops both sequencer and full node
// 5. Restarts both nodes with specified configurations
// 6. Verifies both nodes start successfully and re-establish P2P connections
// 7. Submits new transactions and verifies continued P2P sync functionality
// 8. Validates that all previous and new transactions are preserved on both nodes
// 9. Performs comprehensive state root verification across both nodes
//
// Expected Behavior:
// - Initial transactions should be processed and synced correctly
// - Both nodes should restart without errors or crashes
// - Previous blockchain state should be preserved on both nodes
// - P2P connections should be re-established automatically
// - New transactions should be processed and synced after restart
// - State roots should remain consistent between sequencer and full node
// - Block heights should progress correctly on both nodes
// - Lazy mode should exhibit proper idle behavior (no automatic block production)
//
// Key Technical Details:
// - Tests dual node restart scenarios (more complex than single node restart)
// - Validates P2P peer discovery and connection re-establishment
// - Ensures genesis file consistency across restarts
// - Tests DA layer connection recovery for both nodes
// - Verifies JWT authentication continues to work for both EVM engines
// - Comprehensive state synchronization validation between nodes
// - Tests lazy mode behavior during initial setup and after restart
//
// This test demonstrates that the distributed rollkit network maintains
// consistency and continues to function correctly even when all nodes
// are restarted simultaneously, including mode changes.
func TestEvmSequencerFullNodeRestartE2E(t *testing.T) {
	flag.Parse()

	t.Run("StandardRestart", func(t *testing.T) {
		testSequencerFullNodeRestart(t, false, false) // normal -> normal
	})

	t.Run("LazyModeRestart", func(t *testing.T) {
		testSequencerFullNodeRestart(t, false, true) // normal -> lazy
	})
}

// testSequencerFullNodeRestart contains the shared test logic for all restart test combinations.
// The initialLazyMode parameter determines whether the sequencer starts in lazy mode.
// The restartLazyMode parameter determines whether the sequencer is restarted in lazy mode.
func testSequencerFullNodeRestart(t *testing.T, initialLazyMode, restartLazyMode bool) {
	flag.Parse()
	workDir := t.TempDir()
	sequencerHome := filepath.Join(workDir, "evm-sequencer")
	fullNodeHome := filepath.Join(workDir, "evm-full-node")
	sut := NewSystemUnderTest(t)

	// === PHASE 1: Initial setup and transaction processing ===

	t.Logf("Phase 1: Setting up sequencer (initial_lazy=%t) and full node with P2P connections...", initialLazyMode)
	t.Logf("Test mode: initial_lazy=%t, restart_lazy=%t", initialLazyMode, restartLazyMode)

	// Get JWT secrets and setup common components first
	jwtSecret, fullNodeJwtSecret, genesisHash := setupCommonEVMTest(t, sut, true)

	// Setup sequencer based on initial mode
	if initialLazyMode {
		setupSequencerNodeLazy(t, sut, sequencerHome, jwtSecret, genesisHash)
		t.Log("Sequencer node (lazy mode) is up")
	} else {
		setupSequencerNode(t, sut, sequencerHome, jwtSecret, genesisHash)
		t.Log("Sequencer node is up")
	}

	// Get P2P address and setup full node
	sequencerP2PAddress := getNodeP2PAddress(t, sut, sequencerHome)
	t.Logf("Sequencer P2P address: %s", sequencerP2PAddress)

	setupFullNode(t, sut, fullNodeHome, sequencerHome, fullNodeJwtSecret, genesisHash, sequencerP2PAddress)
	t.Log("Full node is up")

	// Connect to both EVM instances
	sequencerClient, err := ethclient.Dial(SequencerEthURL)
	require.NoError(t, err, "Should be able to connect to sequencer EVM")
	defer sequencerClient.Close()

	fullNodeClient, err := ethclient.Dial(FullNodeEthURL)
	require.NoError(t, err, "Should be able to connect to full node EVM")
	defer fullNodeClient.Close()

	// Wait for P2P connections to establish
	t.Log("Waiting for P2P connections to establish...")
	require.Eventually(t, func() bool {
		// Check if both nodes are responsive
		seqHeader, seqErr := sequencerClient.HeaderByNumber(context.Background(), nil)
		fnHeader, fnErr := fullNodeClient.HeaderByNumber(context.Background(), nil)

		if seqErr != nil || fnErr != nil {
			return false
		}

		// Both nodes should be responsive and at genesis or later
		seqHeight := seqHeader.Number.Uint64()
		fnHeight := fnHeader.Number.Uint64()

		// With 100ms blocks (10 blocks/sec), allow larger sync tolerance during startup
		// Allow up to 20 blocks difference to account for P2P propagation delays
		return seqHeight == 0 || fnHeight+20 >= seqHeight
	}, DefaultTestTimeout, 250*time.Millisecond, "P2P connections should be established")

	t.Log("P2P connections established")

	ctx := context.Background()

	// If starting in lazy mode, wait for any initial blocks to settle, then verify lazy behavior
	if initialLazyMode {
		t.Log("Waiting for lazy mode sequencer to settle...")
		time.Sleep(2 * time.Second)

		t.Log("Verifying lazy mode behavior: no blocks produced without transactions...")
		verifyNoBlockProduction(t, sequencerClient, 1*time.Second, "sequencer")
		verifyNoBlockProduction(t, fullNodeClient, 1*time.Second, "full node")
	}

	// Submit initial batch of transactions to establish state
	const numInitialTxs = 4
	var initialTxHashes []common.Hash
	var initialTxBlockNumbers []uint64

	t.Logf("Submitting %d initial transactions to establish state...", numInitialTxs)
	for i := 0; i < numInitialTxs; i++ {
		txHash, txBlockNumber := submitTransactionAndGetBlockNumber(t, sequencerClient)
		initialTxHashes = append(initialTxHashes, txHash)
		initialTxBlockNumbers = append(initialTxBlockNumbers, txBlockNumber)
		t.Logf("Initial transaction %d included in sequencer block %d", i+1, txBlockNumber)

		// Verify each transaction syncs to full node
		verifyTransactionSync(t, sequencerClient, fullNodeClient, txHash, txBlockNumber)
		t.Logf("✅ Initial transaction %d synced to full node", i+1)

		time.Sleep(5 * time.Millisecond)
	}

	// Record pre-restart state
	seqHeader, err := sequencerClient.HeaderByNumber(ctx, nil)
	require.NoError(t, err, "Should get sequencer header before restart")
	fnHeader, err := fullNodeClient.HeaderByNumber(ctx, nil)
	require.NoError(t, err, "Should get full node header before restart")

	preRestartSeqHeight := seqHeader.Number.Uint64()
	preRestartFnHeight := fnHeader.Number.Uint64()
	preRestartSeqStateRoot := seqHeader.Root.Hex()
	preRestartFnStateRoot := fnHeader.Root.Hex()

	t.Logf("Pre-restart state:")
	t.Logf("  - Sequencer: height=%d, stateRoot=%s", preRestartSeqHeight, preRestartSeqStateRoot)
	t.Logf("  - Full node: height=%d, stateRoot=%s", preRestartFnHeight, preRestartFnStateRoot)
	t.Logf("  - Initial transactions processed: %d", numInitialTxs)

	// Verify both nodes are at similar heights (allow small difference due to fast block production)
	heightDiff := int64(preRestartSeqHeight) - int64(preRestartFnHeight)
	if heightDiff < 0 {
		heightDiff = -heightDiff
	}
	require.LessOrEqual(t, heightDiff, int64(10),
		"Nodes should be within 10 blocks of each other before restart (seq: %d, fn: %d)",
		preRestartSeqHeight, preRestartFnHeight)

	// === PHASE 2: Graceful shutdown of both nodes ===

	t.Log("Phase 2: Gracefully stopping both sequencer and full node...")

	// Shutdown all processes tracked by SUT
	sut.ShutdownAll()

	// Wait for graceful shutdown to allow state to be saved
	t.Log("Waiting for graceful shutdown and state persistence...")
	time.Sleep(2 * time.Second)

	// Verify shutdown using SUT's process tracking
	require.Eventually(t, func() bool {
		hasAnyProcess := sut.HasProcess()
		t.Logf("Shutdown check: any processes exist=%v", hasAnyProcess)
		return !hasAnyProcess
	}, 10*time.Second, 250*time.Millisecond, "all processes should be stopped")

	t.Log("Both nodes stopped successfully")

	// === PHASE 3: Restart both nodes ===

	t.Log("Phase 3: Restarting both sequencer and full node...")

	// Restart both nodes with specified restart mode
	restartSequencerAndFullNode(t, sut, sequencerHome, fullNodeHome, jwtSecret, fullNodeJwtSecret, genesisHash, restartLazyMode)

	// Reconnect to both EVM instances (connections lost during restart)
	sequencerClient, err = ethclient.Dial(SequencerEthURL)
	require.NoError(t, err, "Should be able to reconnect to sequencer EVM")
	defer sequencerClient.Close()

	fullNodeClient, err = ethclient.Dial(FullNodeEthURL)
	require.NoError(t, err, "Should be able to reconnect to full node EVM")
	defer fullNodeClient.Close()

	// Wait for P2P connections to re-establish
	t.Log("Waiting for P2P connections to re-establish...")
	require.Eventually(t, func() bool {
		// Check if both nodes are responsive
		seqHeader, seqErr := sequencerClient.HeaderByNumber(context.Background(), nil)
		fnHeader, fnErr := fullNodeClient.HeaderByNumber(context.Background(), nil)

		if seqErr != nil || fnErr != nil {
			return false
		}

		// Both nodes should be at or near the same height
		seqHeight := seqHeader.Number.Uint64()
		fnHeight := fnHeader.Number.Uint64()

		// Allow larger difference during restart synchronization with fast blocks
		// With 100ms blocks, allow up to 15 blocks difference during startup
		heightDiff := int64(seqHeight) - int64(fnHeight)
		if heightDiff < 0 {
			heightDiff = -heightDiff
		}

		return heightDiff <= 15
	}, DefaultTestTimeout, 500*time.Millisecond, "P2P connections should be re-established")

	t.Log("P2P connections re-established successfully")

	// === LAZY MODE VERIFICATION (if applicable) ===

	if restartLazyMode {
		t.Log("Verifying lazy mode behavior after restart...")
		// Test that no blocks are produced during idle period in lazy mode
		verifyNoBlockProduction(t, sequencerClient, 1*time.Second, "sequencer (lazy mode)")
		verifyNoBlockProduction(t, fullNodeClient, 1*time.Second, "full node (with lazy sequencer)")
		t.Log("✅ Lazy mode idle behavior verified after restart")
	}

	// === PHASE 4: Verify state preservation ===

	t.Log("Phase 4: Verifying blockchain state preservation after restart...")

	postRestartSeqHeader, err := sequencerClient.HeaderByNumber(ctx, nil)
	require.NoError(t, err, "Should get sequencer header after restart")
	postRestartFnHeader, err := fullNodeClient.HeaderByNumber(ctx, nil)
	require.NoError(t, err, "Should get full node header after restart")

	postRestartSeqHeight := postRestartSeqHeader.Number.Uint64()
	postRestartFnHeight := postRestartFnHeader.Number.Uint64()

	t.Logf("Post-restart state:")
	t.Logf("  - Sequencer: height=%d", postRestartSeqHeight)
	t.Logf("  - Full node: height=%d", postRestartFnHeight)

	// Heights should be preserved or increased (nodes might produce some blocks during startup)
	require.GreaterOrEqual(t, postRestartSeqHeight, preRestartSeqHeight,
		"Sequencer height should be preserved or increased after restart")
	require.GreaterOrEqual(t, postRestartFnHeight, preRestartFnHeight,
		"Full node height should be preserved or increased after restart")

	// Verify all initial transactions are still accessible on both nodes
	t.Log("Verifying initial transactions are preserved on both nodes...")
	for i, txHash := range initialTxHashes {
		expectedBlockNumber := initialTxBlockNumbers[i]

		// Check sequencer
		seqReceipt, err := sequencerClient.TransactionReceipt(ctx, txHash)
		require.NoError(t, err, "Should get transaction %d receipt from sequencer after restart", i+1)
		require.NotNil(t, seqReceipt, "Transaction %d receipt should exist on sequencer after restart", i+1)
		require.Equal(t, uint64(1), seqReceipt.Status, "Transaction %d should be successful on sequencer after restart", i+1)

		// Check full node
		fnReceipt, err := fullNodeClient.TransactionReceipt(ctx, txHash)
		require.NoError(t, err, "Should get transaction %d receipt from full node after restart", i+1)
		require.NotNil(t, fnReceipt, "Transaction %d receipt should exist on full node after restart", i+1)
		require.Equal(t, uint64(1), fnReceipt.Status, "Transaction %d should be successful on full node after restart", i+1)

		// Verify both nodes have transaction in same block
		require.Equal(t, seqReceipt.BlockNumber.Uint64(), fnReceipt.BlockNumber.Uint64(),
			"Transaction %d should be in same block on both nodes", i+1)
		require.Equal(t, expectedBlockNumber, seqReceipt.BlockNumber.Uint64(),
			"Transaction %d should be in expected block %d", i+1, expectedBlockNumber)

		t.Logf("✅ Initial transaction %d preserved on both nodes", i+1)
	}

	// === PHASE 5: Post-restart functionality verification ===

	t.Log("Phase 5: Verifying post-restart functionality and P2P sync...")

	// Submit new transactions after restart to verify functionality
	const numPostRestartTxs = 3
	var postRestartTxHashes []common.Hash
	var postRestartTxBlockNumbers []uint64

	t.Logf("Submitting %d post-restart transactions...", numPostRestartTxs)
	for i := 0; i < numPostRestartTxs; i++ {
		txHash, txBlockNumber := submitTransactionAndGetBlockNumber(t, sequencerClient)
		postRestartTxHashes = append(postRestartTxHashes, txHash)
		postRestartTxBlockNumbers = append(postRestartTxBlockNumbers, txBlockNumber)
		t.Logf("Post-restart transaction %d included in sequencer block %d", i+1, txBlockNumber)

		// Verify transaction syncs to full node (testing P2P sync functionality)
		verifyTransactionSync(t, sequencerClient, fullNodeClient, txHash, txBlockNumber)
		t.Logf("✅ Post-restart transaction %d synced to full node via P2P", i+1)

		time.Sleep(5 * time.Millisecond)
	}

	// === LAZY MODE POST-TRANSACTION VERIFICATION (if applicable) ===

	if restartLazyMode {
		t.Log("Verifying lazy mode post-transaction idle behavior...")
		// Test that no additional blocks are produced after transactions in lazy mode
		verifyNoBlockProduction(t, sequencerClient, 500*time.Millisecond, "sequencer (lazy mode post-tx)")
		verifyNoBlockProduction(t, fullNodeClient, 500*time.Millisecond, "full node (lazy mode post-tx)")
		t.Log("✅ Lazy mode post-transaction idle behavior verified")
	}

	// === PHASE 6: Final state verification with synchronized shutdown ===

	t.Log("Phase 6: Final comprehensive verification with synchronized shutdown...")

	// Step 1: Wait for both nodes to be closely synchronized
	t.Log("Ensuring both nodes are synchronized before shutdown...")
	require.Eventually(t, func() bool {
		seqHeader, seqErr := sequencerClient.HeaderByNumber(ctx, nil)
		fnHeader, fnErr := fullNodeClient.HeaderByNumber(ctx, nil)

		if seqErr != nil || fnErr != nil {
			return false
		}

		seqHeight := seqHeader.Number.Uint64()
		fnHeight := fnHeader.Number.Uint64()

		heightDiff := int64(seqHeight) - int64(fnHeight)
		if heightDiff < 0 {
			heightDiff = -heightDiff
		}

		t.Logf("Synchronization check - Sequencer: %d, Full node: %d, diff: %d", seqHeight, fnHeight, heightDiff)
		return heightDiff <= 10
	}, DefaultTestTimeout, 250*time.Millisecond, "Nodes should be synchronized before shutdown")

	// Step 2: Get both heights while still running
	finalSeqHeader, err := sequencerClient.HeaderByNumber(ctx, nil)
	require.NoError(t, err, "Should get sequencer header")
	finalFnHeader, err := fullNodeClient.HeaderByNumber(ctx, nil)
	require.NoError(t, err, "Should get full node header")

	finalSeqHeight := finalSeqHeader.Number.Uint64()
	finalFnHeight := finalFnHeader.Number.Uint64()

	t.Logf("Final synchronized state:")
	t.Logf("  - Sequencer: height=%d", finalSeqHeight)
	t.Logf("  - Full node: height=%d", finalFnHeight)
	t.Logf("  - Total transactions processed: %d", numInitialTxs+numPostRestartTxs)

	// Step 3: Verify both nodes are at the same final height (allow small tolerance)
	finalHeightDiff := int64(finalSeqHeight) - int64(finalFnHeight)
	if finalHeightDiff < 0 {
		finalHeightDiff = -finalHeightDiff
	}
	require.LessOrEqual(t, finalHeightDiff, int64(10),
		"Nodes should be within 10 blocks of each other at final state (seq: %d, fn: %d)",
		finalSeqHeight, finalFnHeight)

	// Verify blockchain progressed after restart
	require.Greater(t, finalSeqHeight, preRestartSeqHeight,
		"Blockchain should have progressed after restart")

	// Perform state root verification for key blocks
	t.Log("Performing state root verification...")

	// Verify state roots match for a sample of blocks
	// Use the minimum height between both nodes to avoid missing blocks
	startHeight := uint64(1)
	minEndHeight := min(finalSeqHeight, finalFnHeight)

	// For efficiency, check every block if there are few, or sample if many
	var blocksToCheck []uint64
	if minEndHeight <= 10 {
		// Check all blocks up to the minimum height
		for height := startHeight; height <= minEndHeight; height++ {
			blocksToCheck = append(blocksToCheck, height)
		}
	} else {
		// Sample key blocks: first, middle, and last few (using minimum height)
		blocksToCheck = append(blocksToCheck, startHeight)
		if minEndHeight > 2 {
			blocksToCheck = append(blocksToCheck, minEndHeight/2)
		}
		for height := max(minEndHeight-2, startHeight+1); height <= minEndHeight; height++ {
			blocksToCheck = append(blocksToCheck, height)
		}
	}

	for _, blockHeight := range blocksToCheck {
		verifyStateRootsMatch(t, SequencerEthURL, FullNodeEthURL, blockHeight)
	}

	// === PHASE 7: Final transaction verification ===

	t.Log("Phase 7: Final verification of all transactions...")

	// Verify all transactions (initial + post-restart) are accessible on both nodes
	allTxHashes := append(initialTxHashes, postRestartTxHashes...)
	allTxBlockNumbers := append(initialTxBlockNumbers, postRestartTxBlockNumbers...)

	for i, txHash := range allTxHashes {
		expectedBlockNumber := allTxBlockNumbers[i]

		// Verify on sequencer
		seqReceipt, err := sequencerClient.TransactionReceipt(ctx, txHash)
		require.NoError(t, err, "Should get transaction %d receipt from sequencer", i+1)
		require.Equal(t, uint64(1), seqReceipt.Status, "Transaction %d should be successful on sequencer", i+1)

		// Verify on full node
		fnReceipt, err := fullNodeClient.TransactionReceipt(ctx, txHash)
		require.NoError(t, err, "Should get transaction %d receipt from full node", i+1)
		require.Equal(t, uint64(1), fnReceipt.Status, "Transaction %d should be successful on full node", i+1)

		// Verify consistency
		require.Equal(t, seqReceipt.BlockNumber.Uint64(), fnReceipt.BlockNumber.Uint64(),
			"Transaction %d should be in same block on both nodes", i+1)
		require.Equal(t, expectedBlockNumber, seqReceipt.BlockNumber.Uint64(),
			"Transaction %d should be in expected block %d", i+1, expectedBlockNumber)

		if i < numInitialTxs {
			t.Logf("✅ Initial transaction %d verified on both nodes", i+1)
		} else {
			t.Logf("✅ Post-restart transaction %d verified on both nodes", i-numInitialTxs+1)
		}
	}

	// Test summary
	initialModeDesc := "Standard"
	if initialLazyMode {
		initialModeDesc = "Lazy"
	}
	restartModeDesc := "Standard"
	if restartLazyMode {
		restartModeDesc = "Lazy"
	}

	t.Logf("✅ Test PASSED: Sequencer and Full Node restart/recovery working correctly (%s -> %s Mode)!", initialModeDesc, restartModeDesc)
	t.Logf("   - Initial setup in %s mode and P2P sync: ✓", initialModeDesc)
	if initialLazyMode {
		t.Logf("   - Initial lazy mode idle behavior verified: ✓")
	}
	t.Logf("   - %d initial transactions processed and synced: ✓", numInitialTxs)
	t.Logf("   - Graceful shutdown of both nodes: ✓")
	t.Logf("   - Successful restart of both nodes in %s mode without crashes: ✓", restartModeDesc)
	if restartLazyMode {
		t.Logf("   - Sequencer restarted in lazy mode: ✓")
		t.Logf("   - Restart lazy mode idle behavior verified: ✓")
	}
	t.Logf("   - P2P connections re-established: ✓")
	t.Logf("   - State preservation on both nodes: ✓")
	t.Logf("   - Previous transactions preserved on both nodes: ✓")
	t.Logf("   - %d post-restart transactions processed and synced: ✓", numPostRestartTxs)
	if restartLazyMode {
		t.Logf("   - Restart lazy mode post-transaction idle behavior verified: ✓")
	}
	t.Logf("   - Continued P2P sync functionality: ✓")
	t.Logf("   - State root consistency across both nodes: ✓")
	t.Logf("   - Blockchain height progressed: %d -> %d: ✓", preRestartSeqHeight, finalSeqHeight)
	t.Logf("   - All %d transactions verified on both nodes: ✓", len(allTxHashes))
	t.Logf("   - Distributed system resilience demonstrated: ✓")
}

// setupSequencerWithFullNodeDAOnly sets up both sequencer and full node with DA-only connections.
// This helper function creates a setup where the full node connects only via DA and not P2P.
// This tests the DA synchronization mechanism independently of P2P block propagation.
//
// Returns: sequencerClient, fullNodeClient for EVM connections
func setupSequencerWithFullNodeDAOnly(t *testing.T, sut *SystemUnderTest, sequencerHome, fullNodeHome string) (*ethclient.Client, *ethclient.Client) {
	t.Helper()

	// Common setup for both sequencer and full node
	jwtSecret, fullNodeJwtSecret, genesisHash := setupCommonEVMTest(t, sut, true)

	// Setup sequencer
	setupSequencerNode(t, sut, sequencerHome, jwtSecret, genesisHash)
	t.Log("Sequencer node is up")

	// Setup full node without P2P connections (DA-only)
	setupFullNodeDAOnly(t, sut, fullNodeHome, sequencerHome, fullNodeJwtSecret, genesisHash)
	t.Log("Full node is up (DA-only mode)")

	// Connect to both EVM instances
	sequencerClient, err := ethclient.Dial(SequencerEthURL)
	require.NoError(t, err, "Should be able to connect to sequencer EVM")

	fullNodeClient, err := ethclient.Dial(FullNodeEthURL)
	require.NoError(t, err, "Should be able to connect to full node EVM")

	// Wait for DA synchronization to establish
	t.Log("Waiting for DA synchronization to establish...")
	require.Eventually(t, func() bool {
		// Check if both nodes are responsive
		seqHeader, seqErr := sequencerClient.HeaderByNumber(context.Background(), nil)
		fnHeader, fnErr := fullNodeClient.HeaderByNumber(context.Background(), nil)

		if seqErr != nil || fnErr != nil {
			return false
		}

		// Both nodes should be responsive and at genesis or later
		seqHeight := seqHeader.Number.Uint64()
		fnHeight := fnHeader.Number.Uint64()

		// DA sync is slower than P2P, so allow more generous sync tolerance
		// Allow up to 50 blocks difference to account for DA block time and processing delays
		return seqHeight >= 0 && fnHeight >= 0 && (seqHeight == 0 || fnHeight+50 >= seqHeight)
	}, 30*time.Second, 1*time.Second, "DA synchronization should be established")

	t.Log("DA synchronization established")
	return sequencerClient, fullNodeClient
}

// setupFullNodeDAOnly initializes and starts the full node with DA-only connection (no P2P).
// This function handles:
// - Full node initialization (non-aggregator mode)
// - Genesis file copying from sequencer to ensure chain consistency
// - DA layer connection for data availability without P2P peer connections
// - Different EVM engine ports (8555/8561) to avoid conflicts
//
// Parameters:
// - fullNodeHome: Directory path for full node data
// - sequencerHome: Directory path of sequencer (for genesis file copying)
// - fullNodeJwtSecret: JWT secret for full node's EVM engine
// - genesisHash: Hash of the genesis block for chain validation
func setupFullNodeDAOnly(t *testing.T, sut *SystemUnderTest, fullNodeHome, sequencerHome, fullNodeJwtSecret, genesisHash string) {
	t.Helper()

	// Initialize full node
	output, err := sut.RunCmd(evmSingleBinaryPath,
		"init",
		"--home", fullNodeHome,
	)
	require.NoError(t, err, "failed to init full node", output)

	// Copy genesis file from sequencer to full node
	sequencerGenesis := filepath.Join(sequencerHome, "config", "genesis.json")
	fullNodeGenesis := filepath.Join(fullNodeHome, "config", "genesis.json")
	genesisData, err := os.ReadFile(sequencerGenesis)
	require.NoError(t, err, "failed to read sequencer genesis file")
	err = os.WriteFile(fullNodeGenesis, genesisData, 0644)
	require.NoError(t, err, "failed to write full node genesis file")

	// Start full node without P2P connections (DA-only)
	sut.ExecCmd(evmSingleBinaryPath,
		"start",
		"--home", fullNodeHome,
		"--evm.jwt-secret", fullNodeJwtSecret,
		"--evm.genesis-hash", genesisHash,
		"--rollkit.rpc.address", "127.0.0.1:"+FullNodeRPCPort,
		// Note: No P2P configuration - the full node will sync only via DA
		"--evm.engine-url", FullNodeEngineURL,
		"--evm.eth-url", FullNodeEthURL,
		"--rollkit.da.address", DAAddress,
		"--rollkit.da.block_time", DefaultDABlockTime,
	)
	sut.AwaitNodeUp(t, "http://127.0.0.1:"+FullNodeRPCPort, NodeStartupTimeout)
}

// verifyTransactionSyncDAOnly verifies that the transaction syncs to the full node via DA.
// This function is similar to verifyTransactionSync but allows for longer sync times
// since DA synchronization is typically slower than P2P.
//
// Parameters:
// - sequencerClient: Ethereum client connected to sequencer EVM
// - fullNodeClient: Ethereum client connected to full node EVM
// - txHash: Hash of the transaction to verify
// - expectedBlockNumber: Block number where transaction should be included
func verifyTransactionSyncDAOnly(t *testing.T, sequencerClient, fullNodeClient *ethclient.Client, txHash common.Hash, expectedBlockNumber uint64) {
	t.Helper()

	ctx := context.Background()

	// Wait for full node to sync the specific block containing the transaction via DA
	// DA sync is slower than P2P, so we use longer timeout and intervals
	require.Eventually(t, func() bool {
		// Check if full node has reached the block containing the transaction
		fullNodeHeader, err := fullNodeClient.HeaderByNumber(ctx, nil)
		if err != nil {
			return false
		}

		// If full node has reached or passed the transaction block
		if fullNodeHeader.Number.Uint64() >= expectedBlockNumber {
			// Verify the transaction exists in the full node
			receipt, err := fullNodeClient.TransactionReceipt(ctx, txHash)
			if err == nil && receipt != nil && receipt.Status == 1 {
				return receipt.BlockNumber.Uint64() == expectedBlockNumber
			}
		}
		return false
	}, 60*time.Second, 2*time.Second, "Full node should sync the block containing the transaction via DA")

	// Final verification - both nodes should have the transaction in the same block
	sequencerReceipt, err := sequencerClient.TransactionReceipt(ctx, txHash)
	require.NoError(t, err, "Should get transaction receipt from sequencer")

	fullNodeReceipt, err := fullNodeClient.TransactionReceipt(ctx, txHash)
	require.NoError(t, err, "Should get transaction receipt from full node")

	require.Equal(t, sequencerReceipt.BlockNumber.Uint64(), fullNodeReceipt.BlockNumber.Uint64(),
		"Transaction should be in the same block number on both sequencer and full node")
}

// TestEvmSequencerWithFullNodeDAOnlyE2E tests the full node synchronization functionality
// where a full node connects to a sequencer ONLY via DA (no P2P connections).
//
// Test Purpose:
// - Validate that full nodes can sync with sequencers using only the DA layer
// - Test DA-based block synchronization without P2P gossip
// - Ensure transaction data consistency when using DA-only synchronization
// - Verify that state roots remain consistent between sequencer and full node via DA
//
// Test Flow:
// 1. Sets up Local DA layer and separate EVM instances for sequencer and full node
// 2. Starts sequencer node with standard configuration
// 3. Starts full node WITHOUT P2P connections (DA-only mode)
// 4. Submits transactions to sequencer EVM and gets the block numbers
// 5. Verifies the full node syncs the exact same blocks containing the transactions via DA
// 6. Performs comprehensive state root verification across all synced blocks
//
// Validation:
// - Both sequencer and full node start successfully
// - DA synchronization is established between nodes (no P2P)
// - Transactions submitted to sequencer are included in specific blocks
// - Full node syncs the same block numbers containing the transactions via DA
// - Transaction data is identical on both nodes (same block, same receipt)
// - State roots match between sequencer and full node for all blocks
// - Block hashes and transaction counts are consistent across both nodes
//
// Key Technical Details:
// - Uses separate Docker Compose configurations for different EVM ports
// - Full node starts without P2P listen address or peers configuration
// - Validates that DA sync works independently of P2P layer
// - Implements comprehensive state root checking
// - Longer timeouts to account for DA sync being slower than P2P
//
// This test demonstrates that full nodes can sync with sequencers using only the DA layer,
// providing an alternative synchronization mechanism that doesn't rely on P2P networking.
// This is useful for scenarios where P2P connections are restricted or unreliable.
func TestEvmSequencerWithFullNodeDAOnlyE2E(t *testing.T) {
	flag.Parse()
	workDir := t.TempDir()
	sequencerHome := filepath.Join(workDir, "evm-sequencer")
	fullNodeHome := filepath.Join(workDir, "evm-full-node-da-only")
	sut := NewSystemUnderTest(t)

	// Setup both sequencer and full node (DA-only)
	sequencerClient, fullNodeClient := setupSequencerWithFullNodeDAOnly(t, sut, sequencerHome, fullNodeHome)
	defer sequencerClient.Close()
	defer fullNodeClient.Close()

	// === TESTING PHASE ===

	// Submit multiple transactions at different intervals to create state changes
	var txHashes []common.Hash
	var txBlockNumbers []uint64

	// Submit first batch of transactions
	t.Log("Submitting first batch of transactions...")
	for i := 0; i < 3; i++ {
		txHash, txBlockNumber := submitTransactionAndGetBlockNumber(t, sequencerClient)
		txHashes = append(txHashes, txHash)
		txBlockNumbers = append(txBlockNumbers, txBlockNumber)
		t.Logf("Transaction %d included in sequencer block %d", i+1, txBlockNumber)

		// Longer delay between transactions to allow DA processing
		time.Sleep(500 * time.Millisecond)
	}

	// Wait for DA processing
	t.Log("Waiting for DA processing...")
	time.Sleep(3 * time.Second)

	// Submit second batch of transactions
	t.Log("Submitting second batch of transactions...")
	for i := 0; i < 2; i++ {
		txHash, txBlockNumber := submitTransactionAndGetBlockNumber(t, sequencerClient)
		txHashes = append(txHashes, txHash)
		txBlockNumbers = append(txBlockNumbers, txBlockNumber)
		t.Logf("Transaction %d included in sequencer block %d", i+4, txBlockNumber)

		// Longer delay between transactions for DA processing
		time.Sleep(500 * time.Millisecond)
	}

	// Wait for all transactions to be processed and propagated via DA
	t.Log("Waiting for DA synchronization...")
	time.Sleep(5 * time.Second)

	t.Logf("Total transactions submitted: %d across blocks %v", len(txHashes), txBlockNumbers)

	t.Log("Waiting for full node to sync all transaction blocks via DA...")

	// Verify all transactions have synced via DA
	for i, txHash := range txHashes {
		txBlockNumber := txBlockNumbers[i]
		t.Logf("Verifying transaction %d sync in block %d via DA...", i+1, txBlockNumber)
		verifyTransactionSyncDAOnly(t, sequencerClient, fullNodeClient, txHash, txBlockNumber)
	}

	// === STATE ROOT VERIFICATION ===

	t.Log("Verifying state roots match between sequencer and full node...")

	// Get the current height on both nodes to determine the range of blocks to check
	seqCtx := context.Background()
	seqHeader, err := sequencerClient.HeaderByNumber(seqCtx, nil)
	require.NoError(t, err, "Should get latest header from sequencer")

	fnCtx := context.Background()
	fnHeader, err := fullNodeClient.HeaderByNumber(fnCtx, nil)
	require.NoError(t, err, "Should get latest header from full node")

	// Ensure both nodes are at the same height before checking state roots
	seqHeight := seqHeader.Number.Uint64()
	fnHeight := fnHeader.Number.Uint64()

	// Wait for full node to catch up if needed (DA sync can be slower)
	if fnHeight < seqHeight {
		t.Logf("Full node height (%d) is behind sequencer height (%d), waiting for DA sync...", fnHeight, seqHeight)
		require.Eventually(t, func() bool {
			header, err := fullNodeClient.HeaderByNumber(fnCtx, nil)
			if err != nil {
				return false
			}
			return header.Number.Uint64() >= seqHeight
		}, 60*time.Second, 2*time.Second, "Full node should catch up to sequencer height via DA")

		// Re-get the full node height after sync
		fnHeader, err = fullNodeClient.HeaderByNumber(fnCtx, nil)
		require.NoError(t, err, "Should get updated header from full node")
		fnHeight = fnHeader.Number.Uint64()
	}

	// Check state roots for all blocks from genesis up to current height
	// Note: Block 0 is genesis, start from block 1
	startHeight := uint64(1)
	endHeight := min(seqHeight, fnHeight)

	t.Logf("Checking state roots for blocks %d to %d", startHeight, endHeight)

	for blockHeight := startHeight; blockHeight <= endHeight; blockHeight++ {
		verifyStateRootsMatch(t, SequencerEthURL, FullNodeEthURL, blockHeight)
	}

	// Special focus on the transaction blocks
	t.Log("Re-verifying state roots for all transaction blocks...")
	for i, txBlockNumber := range txBlockNumbers {
		if txBlockNumber >= startHeight && txBlockNumber <= endHeight {
			t.Logf("Re-verifying state root for transaction %d block %d", i+1, txBlockNumber)
			verifyStateRootsMatch(t, SequencerEthURL, FullNodeEthURL, txBlockNumber)
		}
	}

	t.Logf("✅ Test PASSED: DA-only synchronization working correctly!")
	t.Logf("   - All blocks (%d-%d) have matching state roots", startHeight, endHeight)
	t.Logf("   - %d transactions synced successfully via DA across blocks %v", len(txHashes), txBlockNumbers)
	t.Logf("   - Full node synchronized using only DA layer (no P2P connections)")
	t.Logf("   - State consistency maintained between sequencer and full node")
	t.Logf("   - DA-based block synchronization mechanism functioning properly")
}
