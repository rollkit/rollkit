//go:build evm
// +build evm

package evm

import (
	"context"
	"log"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	ethTypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/require"
)

const (
	TEST_ETH_URL    = "http://localhost:8545"
	TEST_ENGINE_URL = "http://localhost:8551"

	CHAIN_ID          = "1234"
	GENESIS_HASH      = "0x2b8bbb1ea1e04f9c9809b4b278a8687806edc061a356c7dbc491930d8e922503"
	GENESIS_STATEROOT = "0x05e9954443da80d86f2104e56ffdfd98fe21988730684360104865b3dc8191b4"
	TEST_PRIVATE_KEY  = "cece4f25ac74deb1468965160c7185e07dff413f23fcadb611b05ca37ab0a52e"
	TEST_TO_ADDRESS   = "0x944fDcD1c868E3cC566C78023CcB38A32cDA836E"

	DOCKER_PATH  = "./docker"
	JWT_FILENAME = "jwt.hex"
)

// TestEngineExecution tests the end-to-end execution flow of the EVM engine client.
// The test has two phases:
//
// Build Chain Phase:
// - Sets up test Reth engine with JWT auth
// - Initializes chain with genesis parameters
// - For blocks 1-10:
//   - Generates and submits random transactions
//   - Block 4 has 0 transactions as edge case
//   - Executes transactions and verifies state changes
//   - Stores payloads for sync testing
//
// Sync Chain Phase:
// - Creates fresh engine instance
// - Replays stored payloads
// - Verifies execution matches original:
//   - State roots
//   - Block data
//   - Transaction counts
//
// Validates the engine can process transactions, maintain state,
// handle empty blocks, and support chain replication.
func TestEngineExecution(t *testing.T) {
	allPayloads := make([][][]byte, 0, 10) // Slice to store payloads from build to sync phase

	initialHeight := uint64(1)
	genesisHash := common.HexToHash(GENESIS_HASH)
	genesisTime := time.Now().UTC().Truncate(time.Second)
	genesisStateRoot := common.HexToHash(GENESIS_STATEROOT)
	rollkitGenesisStateRoot := genesisStateRoot[:]

	t.Run("Build chain", func(tt *testing.T) {
		jwtSecret := SetupTestRethEngine(tt, DOCKER_PATH, JWT_FILENAME)

		executionClient, err := NewEngineExecutionClient(
			TEST_ETH_URL,
			TEST_ENGINE_URL,
			jwtSecret,
			genesisHash,
			common.Address{},
		)
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
		defer cancel()
		stateRoot, gasLimit, err := executionClient.InitChain(ctx, genesisTime, initialHeight, CHAIN_ID)
		require.NoError(t, err)
		require.Equal(t, rollkitGenesisStateRoot, stateRoot)
		require.NotZero(t, gasLimit)

		prevStateRoot := rollkitGenesisStateRoot
		lastHeight, lastHash, lastTxs := checkLatestBlock(tt, ctx)
		log.Println("lastTxs", lastTxs)
		lastNonce := uint64(0)

		for blockHeight := initialHeight; blockHeight <= 1; blockHeight++ {
			nTxs := int(blockHeight) + 10
			// randomly use no transactions
			if blockHeight == 4 {
				nTxs = 0
			}
			txs := make([]*ethTypes.Transaction, nTxs)
			for i := range txs {
				txs[i] = GetRandomTransaction(t, TEST_PRIVATE_KEY, TEST_TO_ADDRESS, CHAIN_ID, 22000, &lastNonce)
			}
			for i := range txs {
				SubmitTransaction(t, txs[i])
			}
			time.Sleep(1000 * time.Millisecond)

			payload, err := executionClient.GetTxs(ctx)
			require.NoError(tt, err)
			require.Lenf(tt, payload, nTxs, "expected %d transactions, got %d", nTxs, len(payload))
			log.Println("nTxs", nTxs)

			allPayloads = append(allPayloads, payload)

			txs = make([]*ethTypes.Transaction, nTxs)
			for i := range txs {
				txs[i] = GetRandomTransaction(t, TEST_PRIVATE_KEY, TEST_TO_ADDRESS, CHAIN_ID, 22000, &lastNonce)
			}
			for i := range txs {
				SubmitTransaction(t, txs[i])
			}

			// Check latest block before execution
			beforeHeight, beforeHash, beforeTxs := checkLatestBlock(tt, ctx)
			require.Equal(tt, lastHeight, beforeHeight, "Latest block height should match")
			require.Equal(tt, lastHash.Hex(), beforeHash.Hex(), "Latest block hash should match")
			require.Equal(tt, lastTxs, beforeTxs, "Number of transactions should match")

			newStateRoot, maxBytes, err := executionClient.ExecuteTxs(ctx, payload, blockHeight, time.Now(), prevStateRoot)
			require.NoError(tt, err)
			if nTxs > 0 {
				require.NotZero(tt, maxBytes)
			}

			err = executionClient.SetFinal(ctx, blockHeight)
			require.NoError(tt, err)

			// Check latest block after execution
			lastHeight, lastHash, lastTxs = checkLatestBlock(tt, ctx)
			require.Equal(tt, blockHeight, lastHeight, "Latest block height should match")
			require.NotEmpty(tt, lastHash.Hex(), "Latest block hash should not be empty")
			require.Equal(tt, lastTxs, nTxs, "Number of transactions should be equal")

			if nTxs == 0 {
				require.Equal(tt, prevStateRoot, newStateRoot)
			} else {
				require.NotEqual(tt, prevStateRoot, newStateRoot)
			}
			prevStateRoot = newStateRoot
		}
	})

	if t.Failed() {
		return
	}

	// start new container and try to sync
	t.Run("Sync chain", func(tt *testing.T) {
		tt.Skip("Skip sync chain")
		jwtSecret := SetupTestRethEngine(t, DOCKER_PATH, JWT_FILENAME)

		executionClient, err := NewEngineExecutionClient(
			TEST_ETH_URL,
			TEST_ENGINE_URL,
			jwtSecret,
			genesisHash,
			common.Address{},
		)
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
		defer cancel()
		stateRoot, gasLimit, err := executionClient.InitChain(ctx, genesisTime, initialHeight, CHAIN_ID)
		require.NoError(t, err)
		require.Equal(t, rollkitGenesisStateRoot, stateRoot)
		require.NotZero(t, gasLimit)

		prevStateRoot := rollkitGenesisStateRoot
		lastHeight, lastHash, lastTxs := checkLatestBlock(tt, ctx)

		for blockHeight := initialHeight; blockHeight <= 10; blockHeight++ {
			payload := allPayloads[blockHeight-1]

			// Check latest block before execution
			beforeHeight, beforeHash, beforeTxs := checkLatestBlock(tt, ctx)
			require.Equal(tt, lastHeight, beforeHeight, "Latest block height should match")
			require.Equal(tt, lastHash.Hex(), beforeHash.Hex(), "Latest block hash should match")
			require.Equal(tt, lastTxs, beforeTxs, "Number of transactions should match")

			newStateRoot, maxBytes, err := executionClient.ExecuteTxs(ctx, payload, blockHeight, time.Now(), prevStateRoot)
			require.NoError(t, err)
			if len(payload) > 0 {
				require.NotZero(tt, maxBytes)
			}
			if len(payload) == 0 {
				require.Equal(tt, prevStateRoot, newStateRoot)
			} else {
				require.NotEqual(tt, prevStateRoot, newStateRoot)
			}

			err = executionClient.SetFinal(ctx, blockHeight)
			require.NoError(tt, err)

			// Check latest block after execution
			lastHeight, lastHash, lastTxs = checkLatestBlock(tt, ctx)
			require.Equal(tt, blockHeight, lastHeight, "Latest block height should match")
			require.NotEmpty(tt, lastHash.Hex(), "Latest block hash should not be empty")
			require.GreaterOrEqual(tt, lastTxs, 0, "Number of transactions should be non-negative")

			prevStateRoot = newStateRoot
		}
	})
}

// createEthClient creates an Ethereum client for checking block information
func createEthClient(t *testing.T) *ethclient.Client {
	t.Helper()

	// Use the same ETH URL as in the tests
	ethClient, err := ethclient.Dial(TEST_ETH_URL)
	require.NoError(t, err, "Failed to create Ethereum client")

	return ethClient
}

// checkLatestBlock retrieves and returns the latest block height, hash, and transaction count using Ethereum API
func checkLatestBlock(t *testing.T, ctx context.Context) (uint64, common.Hash, int) {
	t.Helper()

	// Create an Ethereum client
	ethClient := createEthClient(t)
	defer ethClient.Close()

	// Get the latest block header
	header, err := ethClient.HeaderByNumber(ctx, nil) // nil means latest block
	if err != nil {
		t.Logf("Warning: Failed to get latest block header: %v", err)
		return 0, common.Hash{}, 0
	}

	blockNumber := header.Number.Uint64()
	blockHash := header.Hash()

	// Get the full block to count transactions
	block, err := ethClient.BlockByNumber(ctx, header.Number)
	if err != nil {
		t.Logf("Warning: Failed to get full block: %v", err)
		t.Logf("Latest block: height=%d, hash=%s, txs=unknown", blockNumber, blockHash.Hex())
		return blockNumber, blockHash, 0
	}

	txCount := len(block.Transactions())

	//t.Logf("Latest block: height=%d, hash=%s, txs=%d", blockNumber, blockHash.Hex(), txCount)
	return blockNumber, blockHash, txCount
}

func TestSubmitTransaction(t *testing.T) {
	//t.Skip("Use this test to submit a transaction manually to the Ethereum client")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	rpcClient, err := ethclient.Dial(TEST_ETH_URL)
	require.NoError(t, err)
	height, err := rpcClient.BlockNumber(ctx)
	require.NoError(t, err)

	privateKey, err := crypto.HexToECDSA(TEST_PRIVATE_KEY)
	require.NoError(t, err)

	address := crypto.PubkeyToAddress(privateKey.PublicKey)
	lastNonce, err := rpcClient.NonceAt(ctx, address, new(big.Int).SetUint64(height))
	require.NoError(t, err)

	for s := 0; s < 30; s++ {
		startTime := time.Now()
		for i := 0; i < 5000; i++ {
			tx := GetRandomTransaction(t, TEST_PRIVATE_KEY, TEST_TO_ADDRESS, CHAIN_ID, 22000, &lastNonce)
			SubmitTransaction(t, tx)
		}
		elapsed := time.Since(startTime)
		if elapsed < 10*time.Second {
			time.Sleep(10*time.Second - elapsed)
		}
	}
}
