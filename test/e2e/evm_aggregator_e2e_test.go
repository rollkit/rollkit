//go:build evm
// +build evm

package e2e

import (
	"flag"
	"path/filepath"
	"testing"
	"time"

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

func setupTestRethEngineE2E(t *testing.T) string {
	return evm.SetupTestRethEngine(t, DOCKER_PATH, "jwt.hex")
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

	// 2. Start EVM (Reth) via Docker Compose using setupTestRethEngine logic
	jwtSecret := setupTestRethEngineE2E(t)

	// 3. Get genesis hash from EVM node
	genesisHash := evm.GetGenesisHash(t)
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
	lastNonce := uint64(0)
	tx := evm.GetRandomTransaction(t, "cece4f25ac74deb1468965160c7185e07dff413f23fcadb611b05ca37ab0a52e", "0x944fDcD1c868E3cC566C78023CcB38A32cDA836E", "1234", 22000, &lastNonce)
	evm.SubmitTransaction(t, tx)
	t.Log("Submitted test transaction to EVM")

	// 7. Wait for block production and verify transaction inclusion
	found := false
	for i := 0; i < 20; i++ {
		time.Sleep(1 * time.Second)
		if evm.CheckTxIncluded(t, tx.Hash()) {
			found = true
			break
		}
	}
	require.True(t, found, "Transaction not included in EVM block after waiting")
	t.Log("Transaction included in EVM block")
}
