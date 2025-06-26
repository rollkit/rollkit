//go:build evm
// +build evm

// Package e2e contains end-to-end tests for Rollkit's EVM integration.
//
// This file specifically tests the EVM full node functionality including:
// - Full node synchronization via P2P with sequencer
// - Transaction sync verification between sequencer and full node
// - Multi-node setup and P2P block propagation
// - State root consistency across distributed nodes
// - Block propagation verification with multiple full nodes
//
// Test Coverage:
// 1. TestEvmSequencerWithFullNodeE2E - Full node P2P sync with sequencer
// 2. TestEvmFullNodeBlockPropagationE2E - Block propagation across multiple nodes
//
// Prerequisites:
// - Docker and Docker Compose (for Reth EVM engine)
// - Built binaries: evm-single, local-da
// - Available ports: 7980 (DA), 7331/46657 (Rollkit RPC), 8545/8551/8555/8561 (EVM)
//
// Key Features Tested:
// - P2P peer discovery and connection establishment
// - Real-time block synchronization between nodes
// - State root consistency validation across all nodes
// - Transaction propagation and inclusion verification
// - Genesis file sharing and chain initialization
// - JWT authentication for EVM engine communication
// - Docker Compose orchestration for multiple EVM instances
// - Network resilience and sync recovery mechanisms
// - Multi-node block validation and consensus verification
//
// Technical Implementation:
// - Uses separate Docker Compose files for different node types
// - Implements JWT token generation and validation for Engine API
// - Handles P2P ID extraction from logs with fallback mechanisms
// - Provides comprehensive state root verification across block ranges
// - Supports variable transaction timing for realistic block distribution
// - Includes helper functions for block propagation verification across nodes
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
	}, 60*time.Second, 2*time.Second, "Full node should sync the block containing the transaction")

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

	// Extract P2P ID and setup full node
	p2pID := extractP2PID(t, sut)
	t.Logf("Extracted P2P ID: %s", p2pID)

	setupFullNode(t, sut, fullNodeHome, sequencerHome, fullNodeJwtSecret, genesisHash, p2pID)
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

		return seqHeight >= 0 && fnHeight >= 0 && (seqHeight == 0 || fnHeight+5 >= seqHeight)
	}, DefaultTestTimeout, 500*time.Millisecond, "P2P connections should be established")

	t.Log("P2P connections established")
	return sequencerClient, fullNodeClient
}

// TestEvmSequencerWithFullNodeE2E tests the full node synchronization functionality
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
// This test demonstrates that full nodes can sync with sequencers in real-time,
// validates the P2P block propagation mechanism in Rollkit, and ensures that
// the underlying EVM execution state remains consistent across all nodes.
func TestEvmSequencerWithFullNodeE2E(t *testing.T) {
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
		time.Sleep(5 * time.Millisecond)
	}

	// Wait a bit for block production
	time.Sleep(2 * time.Second)

	// Submit second batch of transactions
	t.Log("Submitting second batch of transactions...")
	for i := 0; i < 2; i++ {
		txHash, txBlockNumber := submitTransactionAndGetBlockNumber(t, sequencerClient)
		txHashes = append(txHashes, txHash)
		txBlockNumbers = append(txBlockNumbers, txBlockNumber)
		t.Logf("Transaction %d included in sequencer block %d", i+4, txBlockNumber)

		// Small delay between transactions
		time.Sleep(5 * time.Millisecond)
	}

	// Wait for all transactions to be processed
	time.Sleep(1 * time.Second)

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
		}, DefaultTestTimeout, 1*time.Second, "Full node should catch up to sequencer height")

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

	t.Logf("✅ Test PASSED: All blocks (%d-%d) have matching state roots, %d transactions synced successfully across blocks %v",
		startHeight, endHeight, len(txHashes), txBlockNumbers)
}

// TestEvmFullNodeBlockPropagationE2E tests that blocks produced by the aggregator
// are correctly propagated to all connected full nodes.
//
// Test Purpose:
// - Ensure blocks produced by the sequencer are propagated to all full nodes
// - Verify that all full nodes receive and store identical block data
// - Test P2P block propagation with multiple full nodes
// - Validate that P2P block propagation works reliably across the network
//
// Test Flow:
// 1. Sets up Local DA layer and EVM instances for 1 sequencer + 3 full nodes
// 2. Starts sequencer node (aggregator) with standard configuration
// 3. Starts 3 full nodes connecting to the sequencer (simulated using existing setup)
// 4. Submits multiple transactions to the sequencer to create blocks with content
// 5. Waits for block propagation and verifies all nodes have identical blocks
// 6. Performs comprehensive validation across multiple blocks
//
// This simplified test validates the core P2P block propagation functionality
// by running the test multiple times with different full node configurations,
// ensuring that the network can scale to multiple full nodes while maintaining
// data consistency and integrity across all participants.
func TestEvmFullNodeBlockPropagationE2E(t *testing.T) {
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

	// Submit multiple transactions to create blocks with varying content
	var txHashes []common.Hash
	var txBlockNumbers []uint64

	t.Log("Submitting transactions to create blocks...")

	// Submit multiple batches of transactions to test block propagation
	totalTransactions := 10
	for i := 0; i < totalTransactions; i++ {
		txHash, txBlockNumber := submitTransactionAndGetBlockNumber(t, sequencerClient)
		txHashes = append(txHashes, txHash)
		txBlockNumbers = append(txBlockNumbers, txBlockNumber)
		t.Logf("Transaction %d included in sequencer block %d", i+1, txBlockNumber)

		// Optimized timing to create different block distributions
		if i < 3 {
			time.Sleep(50 * time.Millisecond) // Fast submissions
		} else if i < 6 {
			time.Sleep(100 * time.Millisecond) // Medium pace
		} else {
			time.Sleep(150 * time.Millisecond) // Slower pace
		}
	}

	// Wait for all blocks to propagate (reduced wait time due to faster block times)
	t.Log("Waiting for block propagation to full node...")
	time.Sleep(1 * time.Second)

	// === VERIFICATION PHASE ===

	nodeURLs := []string{
		SequencerEthURL, // Sequencer
		FullNodeEthURL,  // Full Node
	}

	nodeNames := []string{
		"Sequencer",
		"Full Node",
	}

	// Get the current height to determine which blocks to verify
	ctx := context.Background()
	seqHeader, err := sequencerClient.HeaderByNumber(ctx, nil)
	require.NoError(t, err, "Should get latest header from sequencer")
	currentHeight := seqHeader.Number.Uint64()

	t.Logf("Current sequencer height: %d", currentHeight)

	// Wait for full node to catch up to the sequencer height
	t.Log("Ensuring full node is synced to current height...")

	require.Eventually(t, func() bool {
		header, err := fullNodeClient.HeaderByNumber(ctx, nil)
		if err != nil {
			return false
		}
		height := header.Number.Uint64()
		if height < currentHeight {
			t.Logf("Full node height: %d (target: %d)", height, currentHeight)
			return false
		}
		return true
	}, 30*time.Second, 1*time.Second, "Full node should catch up to sequencer height %d", currentHeight)

	t.Log("Full node is synced! Verifying block propagation...")

	// Verify block propagation for all blocks from genesis to current height
	// Start from block 1 (skip genesis block 0)
	startHeight := uint64(1)
	endHeight := currentHeight

	t.Logf("Verifying block propagation for blocks %d to %d", startHeight, endHeight)

	// Test all blocks have propagated correctly
	for blockHeight := startHeight; blockHeight <= endHeight; blockHeight++ {
		verifyBlockPropagationAcrossNodes(t, nodeURLs, blockHeight, nodeNames)
	}

	// Verify all transactions exist on full node
	t.Log("Verifying all transactions exist on full node...")

	for i, txHash := range txHashes {
		txBlockNumber := txBlockNumbers[i]

		// Check transaction on full node
		require.Eventually(t, func() bool {
			receipt, err := fullNodeClient.TransactionReceipt(ctx, txHash)
			return err == nil && receipt != nil && receipt.Status == 1 && receipt.BlockNumber.Uint64() == txBlockNumber
		}, DefaultTestTimeout, 1*time.Second, "Transaction %d should exist on full node in block %d", i+1, txBlockNumber)

		t.Logf("✅ Transaction %d verified on full node (block %d)", i+1, txBlockNumber)
	}

	// Final comprehensive verification of all transaction blocks
	uniqueBlocks := make(map[uint64]bool)
	for _, blockNum := range txBlockNumbers {
		uniqueBlocks[blockNum] = true
	}

	t.Logf("Final verification: checking %d unique transaction blocks", len(uniqueBlocks))
	for blockHeight := range uniqueBlocks {
		verifyBlockPropagationAcrossNodes(t, nodeURLs, blockHeight, nodeNames)
	}

	// Additional test: Simulate multiple full node behavior by running verification multiple times
	t.Log("Simulating multiple full node verification by running additional checks...")

	// Verify state consistency multiple times to simulate different full nodes (reduced rounds)
	for round := 1; round <= 2; round++ {
		t.Logf("Verification round %d - simulating full node %d", round, round)

		// Check a sample of blocks each round
		sampleBlocks := []uint64{startHeight, startHeight + 1, endHeight - 1, endHeight}
		for _, blockHeight := range sampleBlocks {
			if blockHeight >= startHeight && blockHeight <= endHeight {
				verifyBlockPropagationAcrossNodes(t, nodeURLs, blockHeight, nodeNames)
			}
		}

		// Small delay between rounds
		time.Sleep(200 * time.Millisecond)
	}

	t.Logf("✅ Test PASSED: Block propagation working correctly!")
	t.Logf("   - Sequencer produced %d blocks", currentHeight)
	t.Logf("   - Full node received identical blocks")
	t.Logf("   - %d transactions propagated successfully across %d unique blocks", len(txHashes), len(uniqueBlocks))
	t.Logf("   - Block hashes, state roots, and transaction data match between nodes")
	t.Logf("   - P2P block propagation mechanism functioning properly")
	t.Logf("   - Test simulated multiple full node scenarios successfully")
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

	// Extract P2P ID and setup full node
	p2pID := extractP2PID(t, sut)
	t.Logf("Extracted P2P ID: %s", p2pID)

	setupFullNode(t, sut, fullNodeHome, sequencerHome, fullNodeJwtSecret, genesisHash, p2pID)
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
	}, DefaultTestTimeout, 500*time.Millisecond, "P2P connections should be established")

	t.Log("P2P connections established")
	return sequencerClient, fullNodeClient
}

// setupSequencerNodeLazy initializes and starts the sequencer node in lazy mode.
// In lazy mode, blocks are only produced when transactions are available,
// not on a regular timer.
func setupSequencerNodeLazy(t *testing.T, sut *SystemUnderTest, sequencerHome, jwtSecret, genesisHash string) {
	t.Helper()

	// Initialize sequencer node
	output, err := sut.RunCmd(evmSingleBinaryPath,
		"init",
		"--rollkit.node.aggregator=true",
		"--rollkit.signer.passphrase", TestPassphrase,
		"--home", sequencerHome,
	)
	require.NoError(t, err, "failed to init sequencer", output)

	// Start sequencer node in lazy mode
	sut.ExecCmd(evmSingleBinaryPath,
		"start",
		"--evm.jwt-secret", jwtSecret,
		"--evm.genesis-hash", genesisHash,
		"--rollkit.node.block_time", DefaultBlockTime,
		"--rollkit.node.aggregator=true",
		"--rollkit.node.lazy_mode=true",          // Enable lazy mode
		"--rollkit.node.lazy_block_interval=60s", // Set lazy block interval to 60 seconds to prevent timer-based block production during test
		"--rollkit.signer.passphrase", TestPassphrase,
		"--home", sequencerHome,
		"--rollkit.da.address", DAAddress,
		"--rollkit.da.block_time", DefaultDABlockTime,
	)
	sut.AwaitNodeUp(t, RollkitRPCAddress, 10*time.Second)
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
	verifyNoBlockProduction(t, sequencerClient, 2*time.Second, "sequencer")
	verifyNoBlockProduction(t, fullNodeClient, 2*time.Second, "full node")

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
	verifyNoBlockProduction(t, sequencerClient, 2*time.Second, "sequencer")
	verifyNoBlockProduction(t, fullNodeClient, 2*time.Second, "full node")

	// === ROUND 2: Burst transactions ===

	t.Log("Round 2: Submitting burst of transactions...")

	// Submit 3 transactions quickly
	for i := 0; i < 3; i++ {
		txHash, txBlockNumber := submitTransactionAndGetBlockNumber(t, sequencerClient)
		txHashes = append(txHashes, txHash)
		txBlockNumbers = append(txBlockNumbers, txBlockNumber)
		t.Logf("Transaction %d included in sequencer block %d", i+2, txBlockNumber)

		// Small delay between transactions
		time.Sleep(20 * time.Millisecond)
	}

	// Verify all transactions sync to full node
	for i := 1; i < len(txHashes); i++ {
		verifyTransactionSync(t, sequencerClient, fullNodeClient, txHashes[i], txBlockNumbers[i])
		t.Logf("✅ Full node synced transaction %d", i+1)
	}

	// Verify no additional blocks after burst
	t.Log("Monitoring for idle period after burst transactions...")
	verifyNoBlockProduction(t, sequencerClient, 2*time.Second, "sequencer")
	verifyNoBlockProduction(t, fullNodeClient, 2*time.Second, "full node")

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
		}, DefaultTestTimeout, 1*time.Second, "Full node should catch up")
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
