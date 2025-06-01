//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
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
	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/require"
	tc "github.com/testcontainers/testcontainers-go/modules/compose"
)

var evmSingleBinaryPath string

const (
	DOCKER_PATH = "../../execution/evm/docker"
)

func init() {
	flag.StringVar(&evmSingleBinaryPath, "evm-binary", "evm-single", "evm-single binary")
}

func generateJWTSecret() (string, error) {
	jwtSecret := make([]byte, 32)
	_, err := rand.Read(jwtSecret)
	if err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return hex.EncodeToString(jwtSecret), nil
}

func decodeSecret(jwtSecret string) ([]byte, error) {
	secret, err := hex.DecodeString(strings.TrimPrefix(jwtSecret, "0x"))
	if err != nil {
		return nil, fmt.Errorf("failed to decode JWT secret: %w", err)
	}
	return secret, nil
}

func getAuthToken(jwtSecret []byte) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"exp": time.Now().Add(time.Hour * 1).Unix(),
		"iat": time.Now().Unix(),
	})
	authToken, err := token.SignedString(jwtSecret)
	if err != nil {
		return "", fmt.Errorf("failed to sign JWT token: %w", err)
	}
	return authToken, nil
}

func waitForRethContainer(t *testing.T, jwtSecret string) error {
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
			resp, err := client.Post("http://localhost:8545", "application/json", rpcReq)
			if err == nil {
				if err := resp.Body.Close(); err != nil {
					return fmt.Errorf("failed to close response body: %w", err)
				}
				if resp.StatusCode == http.StatusOK {
					req, err := http.NewRequest("POST", "http://localhost:8551", strings.NewReader(`{"jsonrpc":"2.0","method":"engine_getClientVersionV1","params":[],"id":1}`))
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

func setupTestRethEngineE2E(t *testing.T) string {
	dockerPath, err := filepath.Abs(DOCKER_PATH)
	t.Logf("Docker path: %s", dockerPath)
	require.NoError(t, err)
	jwtPath := filepath.Join(dockerPath, "jwttoken")
	err = os.MkdirAll(jwtPath, 0750)
	require.NoError(t, err)
	jwtSecret, err := generateJWTSecret()
	require.NoError(t, err)
	jwtFile := filepath.Join(jwtPath, "jwt.hex")
	err = os.WriteFile(jwtFile, []byte(jwtSecret), 0600)
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(jwtFile) })
	composeFilePath := filepath.Join(dockerPath, "docker-compose.yml")
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
	err = waitForRethContainer(t, jwtSecret)
	require.NoError(t, err)
	return jwtSecret
}

func TestEvmAggregatorE2E(t *testing.T) {
	flag.Parse()
	workDir := t.TempDir()
	nodeHome := filepath.Join(workDir, "evm-agg")
	sut := NewSystemUnderTest(t)

	// 1. Start local DA
	localDABinary := filepath.Join(filepath.Dir(evmSingleBinaryPath), "local-da")
	sut.ExecCmd(localDABinary)
	t.Log("Started local DA")
	time.Sleep(500 * time.Millisecond)

	// // 2. Start EVM (Reth) via Docker Compose using setupTestRethEngine logic
	jwtSecret := setupTestRethEngineE2E(t)

	// // 3. Get genesis hash from EVM node
	genesisHash := getGenesisHash(t)
	t.Logf("Genesis hash: %s", genesisHash)

	// 4. Initialize aggregator node
	output, err := sut.RunCmd(evmSingleBinaryPath,
		"init",
		"--rollkit.node.aggregator=true",
		"--rollkit.signer.passphrase", "secret",
		"--home", nodeHome,
	)
	require.NoError(t, err, "failed to init aggregator", output)
	t.Log("Initialized aggregator node")

	// 5. Start aggregator node
	sut.ExecCmd(evmSingleBinaryPath,
		"start",
		"--evm.jwt-secret", jwtSecret,
		"--evm.genesis-hash", genesisHash,
		"--rollkit.node.block_time", "1s",
		"--rollkit.node.aggregator=true",
		"--rollkit.signer.passphrase", "secret",
		"--home", nodeHome,
	)
	sut.AwaitNodeUp(t, "http://127.0.0.1:7331", 10*time.Second)
	t.Log("Aggregator node is up")

	// 6. Submit a transaction to EVM
	tx := getRandomTransactionE2E(t, 22000)
	submitTransactionE2E(t, tx)
	t.Log("Submitted test transaction to EVM")

	// 7. Wait for block production and verify transaction inclusion
	found := false
	for i := 0; i < 20; i++ {
		time.Sleep(1 * time.Second)
		if checkTxIncludedE2E(t, tx.Hash()) {
			found = true
			break
		}
	}
	require.True(t, found, "Transaction not included in EVM block after waiting")
	t.Log("Transaction included in EVM block")
}

// Helper: Get genesis hash from EVM node
func getGenesisHash(t *testing.T) string {
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

// Helper: Submit transaction to EVM node
func submitTransactionE2E(t *testing.T, tx *types.Transaction) {
	rpcClient, err := ethclient.Dial("http://localhost:8545")
	require.NoError(t, err)
	err = rpcClient.SendTransaction(context.Background(), tx)
	require.NoError(t, err)
}

// Helper: Check if transaction is included in EVM block
func checkTxIncludedE2E(t *testing.T, txHash common.Hash) bool {
	rpcClient, err := ethclient.Dial("http://localhost:8545")
	if err != nil {
		return false
	}
	receipt, err := rpcClient.TransactionReceipt(context.Background(), txHash)
	return err == nil && receipt != nil && receipt.Status == 1
}

// Helper: Create a test transaction (copied from execution_test.go)
func getRandomTransactionE2E(t *testing.T, gasLimit uint64) *types.Transaction {
	const (
		TEST_PRIVATE_KEY = "cece4f25ac74deb1468965160c7185e07dff413f23fcadb611b05ca37ab0a52e"
		CHAIN_ID         = "1234"
		TEST_TO_ADDRESS  = "0x944fDcD1c868E3cC566C78023CcB38A32cDA836E"
	)
	privateKey, err := crypto.HexToECDSA(TEST_PRIVATE_KEY)
	require.NoError(t, err)

	rpcClient, err := ethclient.Dial("http://localhost:8545")
	require.NoError(t, err)
	fromAddress := crypto.PubkeyToAddress(privateKey.PublicKey)
	nonce, err := rpcClient.PendingNonceAt(context.Background(), fromAddress)
	require.NoError(t, err)

	chainId, _ := new(big.Int).SetString(CHAIN_ID, 10)
	txValue := big.NewInt(1000000000000000000)
	gasPrice := big.NewInt(30000000000)
	toAddress := common.HexToAddress(TEST_TO_ADDRESS)
	data := make([]byte, 16)
	_, err = rand.Read(data)
	require.NoError(t, err)

	tx := types.NewTx(&types.LegacyTx{
		Nonce:    nonce,
		To:       &toAddress,
		Value:    txValue,
		Gas:      gasLimit,
		GasPrice: gasPrice,
		Data:     data,
	})

	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainId), privateKey)
	require.NoError(t, err)
	return signedTx
}
