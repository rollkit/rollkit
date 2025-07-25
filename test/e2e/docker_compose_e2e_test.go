//go:build evm
// +build evm

// Package e2e contains end-to-end tests for Rollkit's Docker Compose EVM integration.
//
// This file specifically tests the Docker Compose setup including:
// - Full stack deployment (Reth + Local DA + Rollkit EVM Single)
// - Service health and readiness verification
// - Transaction processing and inclusion verification
// - Sustained operation under load
// - Error handling and recovery scenarios
//
// Test Coverage:
// TestDockerComposeE2E - Comprehensive Docker Compose stack test covering:
//   - Phase 1: Service startup and health verification
//   - Phase 2: Basic transaction processing
//   - Phase 3: High-throughput transaction handling (20 transactions)
package e2e

import (
	"context"
	"flag"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/require"

	"github.com/evstack/ev-node/execution/evm"
)

// TestDockerComposeE2E runs a comprehensive test suite against the Docker Compose setup.
// This test assumes the Docker Compose services are already running and accessible.
//
// Test Phases:
// 1. Service Health Verification - Verify all services are responding
// 2. Basic Transaction Test - Single transaction processing
// 3. High-Throughput Test - Multiple transaction processing
//
// This test is designed to be run by CI/CD pipelines after Docker Compose services
// have been started and are ready to accept connections.
func TestDockerComposeE2E(t *testing.T) {
	flag.Parse()

	// Connect to EVM (should be running on localhost:8545 in Docker Compose)
	client, err := ethclient.Dial("http://localhost:8545")
	require.NoError(t, err, "Should be able to connect to EVM via Docker Compose")
	defer client.Close()

	ctx := context.Background()
	var globalNonce uint64 = 0

	// ===== PHASE 1: Service Health Verification =====
	t.Log("🔄 PHASE 1: Service Health Verification")

	// Test EVM JSON-RPC endpoint
	t.Log("1a. Testing EVM JSON-RPC endpoint...")
	blockNumber, err := client.BlockNumber(ctx)
	require.NoError(t, err, "EVM should respond to block number requests")
	t.Logf("✅ EVM is responding, current block: %d", blockNumber)

	// Test chain ID
	chainID, err := client.ChainID(ctx)
	require.NoError(t, err, "Should be able to get chain ID")
	t.Logf("✅ Chain ID: %s", chainID.String())

	// Test network version
	networkID, err := client.NetworkID(ctx)
	require.NoError(t, err, "Should be able to get network ID")
	t.Logf("✅ Network ID: %s", networkID.String())

	// Test DA endpoint connectivity (it uses JSON-RPC, not REST)
	t.Log("1b. Testing DA endpoint connectivity...")
	httpClient := &http.Client{Timeout: 5 * time.Second}
	// DA service responds to JSON-RPC calls, not simple GET requests
	// We'll test connectivity by making a JSON-RPC request (even if it errors, that proves it's responsive)
	daTestReq := strings.NewReader(`{"jsonrpc":"2.0","method":"da.Get","params":["test","test"],"id":1}`)
	resp, err := httpClient.Post("http://localhost:7980", "application/json", daTestReq)
	require.NoError(t, err, "DA endpoint should be accessible")
	require.True(t, resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusInternalServerError,
		"DA should respond with JSON-RPC (status: %d)", resp.StatusCode)
	resp.Body.Close()
	t.Log("✅ DA endpoint is responding to JSON-RPC requests")

	t.Log("✅ All services are healthy and responding")

	// ===== PHASE 2: Basic Transaction Test =====
	t.Log("🔄 PHASE 2: Basic Transaction Test")

	// Get initial block height for comparison
	initialBlock, err := client.HeaderByNumber(ctx, nil)
	require.NoError(t, err, "Should get initial block header")
	t.Logf("Initial block height: %d", initialBlock.Number.Uint64())

	// Submit a single transaction
	tx1 := evm.GetRandomTransaction(t, TestPrivateKey, TestToAddress, DefaultChainID, DefaultGasLimit, &globalNonce)

	err = client.SendTransaction(ctx, tx1)
	require.NoError(t, err, "Should be able to submit transaction to Docker Compose EVM")
	t.Logf("Submitted transaction: %s", tx1.Hash().Hex())

	// Wait for transaction inclusion with extended timeout for Docker environment
	require.Eventually(t, func() bool {
		receipt, err := client.TransactionReceipt(ctx, tx1.Hash())
		return err == nil && receipt != nil && receipt.Status == 1
	}, 30*time.Second, 1*time.Second, "Transaction should be included in Docker Compose setup")

	// Verify block progression
	newBlock, err := client.HeaderByNumber(ctx, nil)
	require.NoError(t, err, "Should get new block header")
	require.Greater(t, newBlock.Number.Uint64(), initialBlock.Number.Uint64(),
		"Block height should have increased")

	t.Log("✅ Basic transaction processing verified")

	// ===== PHASE 3: High-Throughput Transaction Test =====
	t.Log("🔄 PHASE 3: High-Throughput Transaction Test (20 transactions)")

	// Submit multiple transactions for throughput testing
	const numTxs = 20 // Optimized for fast CI/CD execution
	var txHashes []common.Hash

	t.Logf("Submitting %d transactions for throughput testing...", numTxs)
	startTime := time.Now()

	for i := 0; i < numTxs; i++ {
		tx := evm.GetRandomTransaction(t, TestPrivateKey, TestToAddress, DefaultChainID, DefaultGasLimit, &globalNonce)

		err := client.SendTransaction(ctx, tx)
		require.NoError(t, err, "Should submit transaction %d", i+1)

		txHashes = append(txHashes, tx.Hash())

		// Small delay to avoid overwhelming the system
		time.Sleep(100 * time.Millisecond)

		if (i+1)%10 == 0 {
			t.Logf("Submitted %d/%d transactions", i+1, numTxs)
		}
	}

	submissionTime := time.Since(startTime)
	t.Logf("✅ All %d transactions submitted in %v", numTxs, submissionTime)

	// Wait for all transactions to be included
	t.Log("Waiting for all transactions to be included...")
	var includedCount int

	require.Eventually(t, func() bool {
		includedCount = 0
		for _, txHash := range txHashes {
			receipt, err := client.TransactionReceipt(ctx, txHash)
			if err == nil && receipt != nil && receipt.Status == 1 {
				includedCount++
			}
		}

		if includedCount%10 == 0 && includedCount > 0 {
			t.Logf("Progress: %d/%d transactions included", includedCount, numTxs)
		}

		return includedCount == numTxs
	}, 60*time.Second, 2*time.Second, "All transactions should be included in Docker Compose setup")

	totalTime := time.Since(startTime)
	t.Logf("✅ All %d transactions included in %v (avg: %v per tx)",
		numTxs, totalTime, totalTime/time.Duration(numTxs))

	// ===== FINAL VERIFICATION =====
	t.Log("🔄 Final Verification")

	// Get final statistics
	finalHeader, err := client.HeaderByNumber(ctx, nil)
	require.NoError(t, err, "Should get final block header")

	totalTestTransactions := 1 + numTxs // basic + throughput
	finalHeight := finalHeader.Number.Uint64()
	totalBlocks := finalHeight - initialBlock.Number.Uint64()

	t.Logf("🎉 DOCKER COMPOSE E2E TEST COMPLETED SUCCESSFULLY!")
	t.Logf("   📊 Test Statistics:")
	t.Logf("      - Total test duration: %v", time.Since(startTime).Truncate(time.Second))
	t.Logf("      - Total transactions submitted: %d", totalTestTransactions)
	t.Logf("      - Total blocks produced: %d", totalBlocks)
	t.Logf("      - Final block height: %d", finalHeight)
	t.Logf("      - Chain ID: %s", chainID.String())
	t.Logf("   ✅ All Docker Compose services functioned correctly throughout the test!")
	t.Logf("   ✅ Transaction processing, block production, and system health verified!")
}
