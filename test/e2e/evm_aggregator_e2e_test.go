package e2e

// import (
// 	"bytes"
// 	"context"
// 	"crypto/rand"
// 	"encoding/json"
// 	"flag"
// 	"io/ioutil"
// 	"math/big"
// 	"net/http"
// 	"os"
// 	"os/exec"
// 	"path/filepath"
// 	"testing"
// 	"time"

// 	"github.com/ethereum/go-ethereum/common"
// 	"github.com/ethereum/go-ethereum/core/types"
// 	"github.com/ethereum/go-ethereum/crypto"
// 	"github.com/ethereum/go-ethereum/ethclient"
// 	"github.com/stretchr/testify/require"
// )

// var evmSingleBinaryPath string

// func init() {
// 	flag.StringVar(&evmSingleBinaryPath, "evm-binary", "evm-single", "evm-single binary")
// }

// func TestEvmAggregatorE2E(t *testing.T) {
// 	flag.Parse()
// 	workDir := t.TempDir()
// 	nodeHome := filepath.Join(workDir, "evm-agg")
// 	sut := NewSystemUnderTest(t)

// 	// 1. Start local DA
// 	localDABinary := filepath.Join(filepath.Dir(evmSingleBinaryPath), "local-da")
// 	sut.ExecCmd(localDABinary)
// 	t.Log("Started local DA")
// 	time.Sleep(1 * time.Second)

// 	// 2. Start EVM (Reth) via Docker Compose
// 	dockerDir := filepath.Join("execution", "evm", "docker")
// 	dockerComposePath := filepath.Join(dockerDir, "docker-compose.yml")
// 	dockerUp := exec.Command("docker", "compose", "-f", dockerComposePath, "up", "-d")
// 	dockerUp.Stdout = os.Stdout
// 	dockerUp.Stderr = os.Stderr
// 	require.NoError(t, dockerUp.Run())
// 	t.Cleanup(func() {
// 		dockerDown := exec.Command("docker", "compose", "-f", dockerComposePath, "down", "-v")
// 		dockerDown.Stdout = os.Stdout
// 		dockerDown.Stderr = os.Stderr
// 		_ = dockerDown.Run()
// 	})

// 	// 3. Wait for EVM endpoints to be ready
// 	jwtSecretPath := filepath.Join(dockerDir, "jwttoken", "jwt.hex")
// 	jwtSecret, err := ioutil.ReadFile(jwtSecretPath)
// 	require.NoError(t, err)
// 	waitForRethReady(t)

// 	// 4. Get genesis hash from EVM node
// 	genesisHash := getGenesisHash(t)

// 	// 5. Initialize aggregator node
// 	output, err := sut.RunCmd(evmSingleBinaryPath,
// 		"init",
// 		"--rollkit.node.aggregator=true",
// 		"--rollkit.signer.passphrase", "secret",
// 		"--home", nodeHome,
// 	)
// 	require.NoError(t, err, "failed to init aggregator", output)
// 	t.Log("Initialized aggregator node")

// 	// 6. Start aggregator node
// 	sut.ExecCmd(evmSingleBinaryPath,
// 		"start",
// 		"--evm.jwt-secret", string(jwtSecret),
// 		"--evm.genesis-hash", genesisHash,
// 		"--rollkit.node.block_time", "1s",
// 		"--rollkit.node.aggregator=true",
// 		"--rollkit.signer.passphrase", "secret",
// 		"--home", nodeHome,
// 	)
// 	sut.AwaitNodeUp(t, "http://127.0.0.1:7331", 10*time.Second)
// 	t.Log("Aggregator node is up")

// 	// 7. Submit a transaction to EVM
// 	tx := getRandomTransactionE2E(t, 22000)
// 	submitTransactionE2E(t, tx)
// 	t.Log("Submitted test transaction to EVM")

// 	// 8. Wait for block production and verify transaction inclusion
// 	found := false
// 	for i := 0; i < 20; i++ {
// 		time.Sleep(1 * time.Second)
// 		if checkTxIncludedE2E(t, tx.Hash()) {
// 			found = true
// 			break
// 		}
// 	}
// 	require.True(t, found, "Transaction not included in EVM block after waiting")
// 	t.Log("Transaction included in EVM block")
// }

// // Helper: Wait for Reth endpoints to be ready
// func waitForRethReady(t *testing.T) {
// 	client := &http.Client{Timeout: 200 * time.Millisecond}
// 	for i := 0; i < 60; i++ {
// 		// Check :8545
// 		req := map[string]interface{}{
// 			"jsonrpc": "2.0",
// 			"method":  "net_version",
// 			"params":  []interface{}{},
// 			"id":      1,
// 		}
// 		b, _ := json.Marshal(req)
// 		resp, err := client.Post("http://localhost:8545", "application/json", bytes.NewReader(b))
// 		if err == nil && resp.StatusCode == http.StatusOK {
// 			resp.Body.Close()
// 			return
// 		}
// 		if resp != nil {
// 			resp.Body.Close()
// 		}
// 		time.Sleep(1 * time.Second)
// 	}
// 	t.Fatalf("Reth EVM endpoints not ready after timeout")
// }

// // Helper: Get genesis hash from EVM node
// func getGenesisHash(t *testing.T) string {
// 	client := &http.Client{Timeout: 2 * time.Second}
// 	data := []byte(`{"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["0x0", false],"id":1}`)
// 	resp, err := client.Post("http://localhost:8545", "application/json", bytes.NewReader(data))
// 	require.NoError(t, err)
// 	defer resp.Body.Close()
// 	var result struct {
// 		Result struct {
// 			Hash string `json:"hash"`
// 		} `json:"result"`
// 	}
// 	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
// 	require.NotEmpty(t, result.Result.Hash)
// 	return result.Result.Hash
// }

// // Helper: Submit transaction to EVM node
// func submitTransactionE2E(t *testing.T, tx *types.Transaction) {
// 	rpcClient, err := ethclient.Dial("http://localhost:8545")
// 	require.NoError(t, err)
// 	err = rpcClient.SendTransaction(context.Background(), tx)
// 	require.NoError(t, err)
// }

// // Helper: Check if transaction is included in EVM block
// func checkTxIncludedE2E(t *testing.T, txHash common.Hash) bool {
// 	rpcClient, err := ethclient.Dial("http://localhost:8545")
// 	if err != nil {
// 		return false
// 	}
// 	receipt, err := rpcClient.TransactionReceipt(context.Background(), txHash)
// 	return err == nil && receipt != nil && receipt.Status == 1
// }

// // Helper: Create a test transaction (copied from execution_test.go)
// func getRandomTransactionE2E(t *testing.T, gasLimit uint64) *types.Transaction {
// 	const (
// 		TEST_PRIVATE_KEY = "cece4f25ac74deb1468965160c7185e07dff413f23fcadb611b05ca37ab0a52e"
// 		CHAIN_ID         = "1234"
// 		TEST_TO_ADDRESS  = "0x944fDcD1c868E3cC566C78023CcB38A32cDA836E"
// 	)
// 	privateKey, err := crypto.HexToECDSA(TEST_PRIVATE_KEY)
// 	require.NoError(t, err)

// 	rpcClient, err := ethclient.Dial("http://localhost:8545")
// 	require.NoError(t, err)
// 	fromAddress := crypto.PubkeyToAddress(privateKey.PublicKey)
// 	nonce, err := rpcClient.PendingNonceAt(context.Background(), fromAddress)
// 	require.NoError(t, err)

// 	chainId, _ := new(big.Int).SetString(CHAIN_ID, 10)
// 	txValue := big.NewInt(1000000000000000000)
// 	gasPrice := big.NewInt(30000000000)
// 	toAddress := common.HexToAddress(TEST_TO_ADDRESS)
// 	data := make([]byte, 16)
// 	_, err = rand.Read(data)
// 	require.NoError(t, err)

// 	tx := types.NewTx(&types.LegacyTx{
// 		Nonce:    nonce,
// 		To:       &toAddress,
// 		Value:    txValue,
// 		Gas:      gasLimit,
// 		GasPrice: gasPrice,
// 		Data:     data,
// 	})

// 	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainId), privateKey)
// 	require.NoError(t, err)
// 	return signedTx
// }
