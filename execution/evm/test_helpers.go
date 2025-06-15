package evm

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/require"
	tc "github.com/testcontainers/testcontainers-go/modules/compose"
)

// generateJWTSecret generates a random 32-byte JWT secret and returns it as a hex string.
func generateJWTSecret() (string, error) {
	jwtSecret := make([]byte, 32)
	_, err := rand.Read(jwtSecret)
	if err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return hex.EncodeToString(jwtSecret), nil
}

// SetupTestRethEngine sets up a Reth engine test environment using Docker Compose, writes a JWT secret file, and returns the secret. It also registers cleanup for resources.
func SetupTestRethEngine(t *testing.T, dockerPath, jwtFilename string) string {
	t.Helper()
	dockerAbsPath, err := filepath.Abs(dockerPath)
	require.NoError(t, err)
	jwtPath := filepath.Join(dockerAbsPath, "jwttoken")
	err = os.MkdirAll(jwtPath, 0750)
	require.NoError(t, err)
	jwtSecret, err := generateJWTSecret()
	require.NoError(t, err)
	jwtFile := filepath.Join(jwtPath, jwtFilename)
	err = os.WriteFile(jwtFile, []byte(jwtSecret), 0600)
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(jwtFile) })
	composeFilePath := filepath.Join(dockerAbsPath, "docker-compose.yml")
	identifier := tc.StackIdentifier(strings.ToLower(t.Name()))
	identifier = tc.StackIdentifier(strings.ReplaceAll(string(identifier), "/", "_"))
	identifier = tc.StackIdentifier(strings.ReplaceAll(string(identifier), " ", "_"))
	compose, err := tc.NewDockerComposeWith(tc.WithStackFiles(composeFilePath), identifier)
	require.NoError(t, err, "Failed to create docker compose")
	t.Cleanup(func() {
		ctx := context.Background()
		err := compose.Down(ctx, tc.RemoveOrphans(true), tc.RemoveVolumes(true))
		if err != nil {
			t.Logf("Warning: Failed to tear down docker-compose environment: %v", err)
		}
	})
	ctx := context.Background()
	err = compose.Up(ctx, tc.Wait(true))
	require.NoError(t, err, "Failed to start docker compose")
	err = waitForRethContainer(t, jwtSecret, "http://localhost:8545", "http://localhost:8551")
	require.NoError(t, err)
	return jwtSecret
}

// waitForRethContainer waits for the Reth container to be ready by polling the provided endpoints with JWT authentication.
func waitForRethContainer(t *testing.T, jwtSecret, ethURL, engineURL string) error {
	t.Helper()
	client := &http.Client{Timeout: 100 * time.Millisecond}
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			return fmt.Errorf("timeout waiting for reth container to be ready")
		default:
			rpcReq := strings.NewReader(`{"jsonrpc":"2.0","method":"net_version","params":[],"id":1}`)
			resp, err := client.Post(ethURL, "application/json", rpcReq)
			if err == nil {
				if err := resp.Body.Close(); err != nil {
					return fmt.Errorf("failed to close response body: %w", err)
				}
				if resp.StatusCode == http.StatusOK {
					req, err := http.NewRequest("POST", engineURL, strings.NewReader(`{"jsonrpc":"2.0","method":"engine_getClientVersionV1","params":[],"id":1}`))
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

// Transaction Helpers

// GetRandomTransaction creates and signs a random Ethereum legacy transaction using the provided private key, recipient, chain ID, gas limit, and nonce.
func GetRandomTransaction(t *testing.T, privateKeyHex, toAddressHex, chainID string, gasLimit uint64, lastNonce *uint64) *types.Transaction {
	t.Helper()
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	require.NoError(t, err)
	chainId, ok := new(big.Int).SetString(chainID, 10)
	require.True(t, ok)
	txValue := big.NewInt(1000000000000000000)
	gasPrice := big.NewInt(100000000000) // Increased to 100 Gwei to handle replacement scenarios and ensure acceptance
	toAddress := common.HexToAddress(toAddressHex)
	data := make([]byte, 16)
	_, err = rand.Read(data)
	require.NoError(t, err)
	tx := types.NewTx(&types.LegacyTx{
		Nonce:    *lastNonce,
		To:       &toAddress,
		Value:    txValue,
		Gas:      gasLimit,
		GasPrice: gasPrice,
		Data:     data,
	})
	*lastNonce++
	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainId), privateKey)
	require.NoError(t, err)
	return signedTx
}

// SubmitTransaction submits a signed Ethereum transaction to the local node at http://localhost:8545.
func SubmitTransaction(t *testing.T, tx *types.Transaction) {
	t.Helper()
	rpcClient, err := ethclient.Dial("http://localhost:8545")
	require.NoError(t, err)
	defer rpcClient.Close()

	err = rpcClient.SendTransaction(context.Background(), tx)
	require.NoError(t, err)
}

// ClearTransactionPool attempts to clear the transaction pool by getting current nonce from blockchain
func ClearTransactionPool(t *testing.T, privateKeyHex string) uint64 {
	t.Helper()
	rpcClient, err := ethclient.Dial("http://localhost:8545")
	require.NoError(t, err)
	defer rpcClient.Close()

	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	require.NoError(t, err)
	address := crypto.PubkeyToAddress(privateKey.PublicKey)

	// Get the current nonce from the latest block (not pending)
	nonce, err := rpcClient.NonceAt(context.Background(), address, nil)
	require.NoError(t, err)

	t.Logf("Current nonce for address %s: %d", address.Hex(), nonce)
	return nonce
}

// CheckTxIncluded checks if a transaction with the given hash was included in a block and succeeded.
func CheckTxIncluded(t *testing.T, txHash common.Hash) bool {
	t.Helper()
	rpcClient, err := ethclient.Dial("http://localhost:8545")
	if err != nil {
		return false
	}
	defer rpcClient.Close()
	receipt, err := rpcClient.TransactionReceipt(context.Background(), txHash)
	return err == nil && receipt != nil && receipt.Status == 1
}

// GetGenesisHash retrieves the hash of the genesis block from the local Ethereum node.
func GetGenesisHash(t *testing.T) string {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	data := []byte(`{"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["0x0", false],"id":1}`)
	resp, err := client.Post("http://localhost:8545", "application/json", bytes.NewReader(data))
	require.NoError(t, err)
	defer resp.Body.Close()
	var result struct {
		Result struct {
			Hash string `json:"hash"`
		} `json:"result"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	require.NotEmpty(t, result.Result.Hash)
	return result.Result.Hash
}
