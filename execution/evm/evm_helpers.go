//go:build evm
// +build evm

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
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	tc "github.com/testcontainers/testcontainers-go/modules/compose"
)

// JWT Helpers
func GenerateJWTSecret() (string, error) {
	jwtSecret := make([]byte, 32)
	_, err := rand.Read(jwtSecret)
	if err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return hex.EncodeToString(jwtSecret), nil
}

func DecodeSecret(jwtSecret string) ([]byte, error) {
	secret, err := hex.DecodeString(strings.TrimPrefix(jwtSecret, "0x"))
	if err != nil {
		return nil, fmt.Errorf("failed to decode JWT secret: %w", err)
	}
	return secret, nil
}

func GetAuthToken(jwtSecret []byte) (string, error) {
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

// Docker Compose Setup for Reth Engine
func SetupTestRethEngine(t *testing.T, dockerPath, jwtFilename string) string {
	dockerAbsPath, err := filepath.Abs(dockerPath)
	require.NoError(t, err)
	jwtPath := filepath.Join(dockerAbsPath, "jwttoken")
	err = os.MkdirAll(jwtPath, 0750)
	require.NoError(t, err)
	jwtSecret, err := GenerateJWTSecret()
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
	err = WaitForRethContainer(t, jwtSecret, "http://localhost:8545", "http://localhost:8551")
	require.NoError(t, err)
	return jwtSecret
}

// Wait for Reth container to be ready
func WaitForRethContainer(t *testing.T, jwtSecret, ethURL, engineURL string) error {
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
					secret, err := DecodeSecret(jwtSecret)
					if err != nil {
						return err
					}
					authToken, err := GetAuthToken(secret)
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
func GetRandomTransaction(t *testing.T, privateKeyHex, toAddressHex, chainID string, gasLimit uint64, lastNonce *uint64) *types.Transaction {
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	require.NoError(t, err)
	chainId, _ := new(big.Int).SetString(chainID, 10)
	txValue := big.NewInt(1000000000000000000)
	gasPrice := big.NewInt(30000000000)
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

func SubmitTransaction(t *testing.T, tx *types.Transaction) {
	rpcClient, err := ethclient.Dial("http://localhost:8545")
	require.NoError(t, err)
	err = rpcClient.SendTransaction(context.Background(), tx)
	require.NoError(t, err)
}

func CheckTxIncluded(t *testing.T, txHash common.Hash) bool {
	rpcClient, err := ethclient.Dial("http://localhost:8545")
	if err != nil {
		return false
	}
	receipt, err := rpcClient.TransactionReceipt(context.Background(), txHash)
	return err == nil && receipt != nil && receipt.Status == 1
}

// Genesis Hash Helper
func GetGenesisHash(t *testing.T) string {
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
