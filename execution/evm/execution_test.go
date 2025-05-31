//go:build evm
// +build evm

package evm

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	ethTypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/require"
	tc "github.com/testcontainers/testcontainers-go/modules/compose"
)

const (
	TEST_ETH_URL    = "http://localhost:8545"
	TEST_ENGINE_URL = "http://localhost:8551"

	CHAIN_ID          = "1234"
	GENESIS_HASH      = "0x0a962a0d163416829894c89cb604ae422323bcdf02d7ea08b94d68d3e026a380"
	GENESIS_STATEROOT = "0x362b7d8a31e7671b0f357756221ac385790c25a27ab222dc8cbdd08944f5aea4"
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
		jwtSecret := setupTestRethEngine(tt)

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
			nTxs := int(blockHeight) + 10
			// randomly use no transactions
			if blockHeight == 4 {
				nTxs = 0
			}
			txs := make([]*ethTypes.Transaction, nTxs)
			for i := range txs {
				txs[i] = getRandomTransaction(t, 22000)
			}
			for i := range txs {
				submitTransaction(t, txs[i])
			}
			time.Sleep(1000 * time.Millisecond)

			payload, err := executionClient.GetTxs(ctx)
			require.NoError(tt, err)
			require.Lenf(tt, payload, nTxs, "expected %d transactions, got %d", nTxs, len(payload))

			allPayloads = append(allPayloads, payload)

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
			require.GreaterOrEqual(tt, lastTxs, 0, "Number of transactions should be non-negative")

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
		jwtSecret := setupTestRethEngine(t)

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
			fmt.Println("all good blockheight", blockHeight)
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

// generateJWTSecret generates a random JWT secret
func generateJWTSecret() (string, error) {
	jwtSecret := make([]byte, 32)
	_, err := rand.Read(jwtSecret)
	if err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	return hex.EncodeToString(jwtSecret), nil
}

// setupTestRethEngine starts a reth container using docker-compose and returns the JWT secret
func setupTestRethEngine(t *testing.T) string {
	t.Helper()

	// Get absolute path to docker directory
	dockerPath, err := filepath.Abs(DOCKER_PATH)
	require.NoError(t, err)

	// Create JWT directory if it doesn't exist
	jwtPath := filepath.Join(dockerPath, "jwttoken")
	err = os.MkdirAll(jwtPath, 0750) // More permissive directory permissions
	require.NoError(t, err)

	// Generate JWT secret
	jwtSecret, err := generateJWTSecret()
	require.NoError(t, err)

	// Write JWT secret to file with more permissive file permissions
	jwtFile := filepath.Join(jwtPath, JWT_FILENAME)
	err = os.WriteFile(jwtFile, []byte(jwtSecret), 0600) // More permissive file permissions
	require.NoError(t, err)

	// Clean up JWT file after test
	t.Cleanup(func() {
		// Don't fail the test if we can't remove the file
		// The file might be locked by the container
		_ = os.Remove(jwtFile)
	})

	// Create compose file path
	composeFilePath := filepath.Join(dockerPath, "docker-compose.yml")

	// Create a unique identifier for this test run
	identifier := tc.StackIdentifier(strings.ToLower(t.Name()))
	identifier = tc.StackIdentifier(strings.ReplaceAll(string(identifier), "/", "_"))
	identifier = tc.StackIdentifier(strings.ReplaceAll(string(identifier), " ", "_"))

	// Create compose stack
	compose, err := tc.NewDockerComposeWith(tc.WithStackFiles(composeFilePath), identifier)
	require.NoError(t, err, "Failed to create docker compose")

	// Set up cleanup
	t.Cleanup(func() {
		ctx := context.Background()
		err := compose.Down(ctx, tc.RemoveOrphans(true), tc.RemoveVolumes(true))
		if err != nil {
			t.Logf("Warning: Failed to tear down docker-compose environment: %v", err)
		}
	})

	// Start the services
	ctx := context.Background()
	err = compose.Up(ctx, tc.Wait(true))
	require.NoError(t, err, "Failed to start docker compose")

	// Wait for services to be ready
	err = waitForRethContainer(t, jwtSecret)
	require.NoError(t, err)

	return jwtSecret
}

// waitForRethContainer polls the reth endpoints until they're ready or timeout occurs
func waitForRethContainer(t *testing.T, jwtSecret string) error {
	t.Helper()

	client := &http.Client{
		Timeout: 100 * time.Millisecond,
	}

	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			return fmt.Errorf("timeout waiting for reth container to be ready")
		default:
			// check :8545 is ready
			rpcReq := strings.NewReader(`{"jsonrpc":"2.0","method":"net_version","params":[],"id":1}`)
			resp, err := client.Post(TEST_ETH_URL, "application/json", rpcReq)
			if err == nil {
				if err := resp.Body.Close(); err != nil {
					return fmt.Errorf("failed to close response body: %w", err)
				}
				if resp.StatusCode == http.StatusOK {
					// check :8551 is ready with a stateless call
					req, err := http.NewRequest("POST", TEST_ENGINE_URL, strings.NewReader(`{"jsonrpc":"2.0","method":"engine_getClientVersionV1","params":[],"id":1}`))
					if err != nil {
						return err
					}
					req.Header.Set("Content-Type", "application/json")

					secret, err := decodeSecret(jwtSecret)
					if err != nil {
						return err
					}

					authToken, err := getAuthToken(secret)
					if err != nil {
						return err
					}

					req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", authToken))

					resp, err := client.Do(req)
					if err == nil {
						if err := resp.Body.Close(); err != nil {
							return fmt.Errorf("failed to close response body: %w", err)
						}
						if resp.StatusCode == http.StatusOK {
							return nil
						}
					}
				}
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func TestSubmitTransaction(t *testing.T) {
	t.Skip("Use this test to submit a transaction manually to the Ethereum client")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	rpcClient, err := ethclient.Dial(TEST_ETH_URL)
	require.NoError(t, err)
	height, err := rpcClient.BlockNumber(ctx)
	require.NoError(t, err)

	privateKey, err := crypto.HexToECDSA(TEST_PRIVATE_KEY)
	require.NoError(t, err)

	address := crypto.PubkeyToAddress(privateKey.PublicKey)
	lastNonce, err = rpcClient.NonceAt(ctx, address, new(big.Int).SetUint64(height))
	require.NoError(t, err)

	for s := 0; s < 30; s++ {
		startTime := time.Now()
		for i := 0; i < 5000; i++ {
			tx := getRandomTransaction(t, 22000)
			submitTransaction(t, tx)
		}
		elapsed := time.Since(startTime)
		if elapsed < time.Second {
			time.Sleep(time.Second - elapsed)
		}
	}
}

// submitTransaction submits a signed transaction to the Ethereum client
func submitTransaction(t *testing.T, signedTx *ethTypes.Transaction) {
	rpcClient, err := ethclient.Dial(TEST_ETH_URL)
	require.NoError(t, err)
	err = rpcClient.SendTransaction(context.Background(), signedTx)
	require.NoError(t, err)
}

var lastNonce uint64

// getRandomTransaction generates a randomized valid ETH transaction
func getRandomTransaction(t *testing.T, gasLimit uint64) *ethTypes.Transaction {
	privateKey, err := crypto.HexToECDSA(TEST_PRIVATE_KEY)
	require.NoError(t, err)

	chainId, _ := new(big.Int).SetString(CHAIN_ID, 10)
	txValue := big.NewInt(1000000000000000000)
	gasPrice := big.NewInt(30000000000)
	toAddress := common.HexToAddress(TEST_TO_ADDRESS)
	data := make([]byte, 16)
	_, err = rand.Read(data)
	require.NoError(t, err)

	tx := ethTypes.NewTx(&ethTypes.LegacyTx{
		Nonce:    lastNonce,
		To:       &toAddress,
		Value:    txValue,
		Gas:      gasLimit,
		GasPrice: gasPrice,
		Data:     data,
	})
	lastNonce++

	signedTx, err := ethTypes.SignTx(tx, ethTypes.NewEIP155Signer(chainId), privateKey)
	require.NoError(t, err)
	return signedTx
}
