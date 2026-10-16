//go:build docker_e2e && evm

package docker_e2e

import (
	"context"
	"path"

	"github.com/celestiaorg/tastora/framework/docker/evstack/evmsingle"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"

	"github.com/evstack/ev-node/execution/evm"
)

// TestSignedBlobSubmissionEVM runs the evm binary on reth with da.tx_signer
// and a passphrase-protected file keyring, the production setup, and checks
// that blocks carrying user transactions reach DA paid by ev-node's key.
func (s *DockerTestSuite) TestSignedBlobSubmissionEVM() {
	ctx := context.Background()
	const (
		passphrase     = "da-signer-passphrase"     //nolint:gosec // test keyring
		passphraseFile = "config/da-passphrase.txt" //nolint:gosec // path, not a credential
	)
	da := s.setupSignedSubmitDA(ctx, keyring.BackendFile, passphrase)
	rethCfg := s.SetupRethNode(ctx, "reth")

	chain, err := evmsingle.NewChainBuilder(s.T()).
		WithDockerClient(s.dockerClient).
		WithDockerNetworkID(s.dockerNetworkID).
		WithImage(getEVMSingleImage()).
		WithBinary("evm").
		WithNode(evmsingle.NewNodeConfigBuilder().
			WithEVMEngineURL(rethCfg.EngineURL).
			WithEVMETHURL(rethCfg.EthURL).
			WithEVMJWTSecret(rethCfg.JWTSecret).
			WithEVMGenesisHash(rethCfg.GenesisHash).
			WithEVMBlockTime("1s").
			WithEVMSignerPassphrase("secret").
			WithDAAddress(da.daAddress).
			WithDAAuthToken(da.authToken).
			WithDANamespace("evm-header").
			WithAdditionalStartArgs(
				"--evnode.da.data_namespace", "evm-data",
				"--evnode.da.tx_signer.enabled",
				"--evnode.da.tx_signer.grpc_address", da.grpcAddress,
				"--evnode.da.tx_signer.grpc_tls=false",
				"--evnode.da.tx_signer.keyring_backend", "file",
				"--evnode.da.tx_signer.key_name", daSignerKeyName,
				"--evnode.da.tx_signer.passphrase_file", path.Join(evmsingle.DefaultHomeDir(), passphraseFile),
			).
			Build()).
		Build(ctx)
	s.Require().NoError(err)
	node := chain.Nodes()[0]
	for p, bz := range da.keyFiles {
		s.Require().NoError(node.WriteFile(ctx, p, bz))
	}
	s.Require().NoError(node.WriteFile(ctx, passphraseFile, []byte(passphrase)))
	s.Require().NoError(node.Start(ctx))
	s.WaitForEVMHealthy(ctx, node)

	eth := s.SetupEthClient(ctx, rethCfg.EthURLExternal, evmTestChainID)
	var nonce uint64
	var lastBlock uint64
	for range 3 {
		tx := evm.GetRandomTransaction(s.T(), evmTestPrivateKey, evmTestToAddress, evmTestChainID, evmTestGasLimit, &nonce)
		s.Require().NoError(eth.SendTransaction(ctx, tx))
		s.WaitForTxIncluded(ctx, eth, tx.Hash())
		receipt, err := eth.TransactionReceipt(ctx, tx.Hash())
		s.Require().NoError(err)
		lastBlock = receipt.BlockNumber.Uint64()
	}

	info, err := node.GetNetworkInfo(ctx)
	s.Require().NoError(err)
	s.waitForDAIncluded(ctx, "http://localhost:"+info.External.Ports.RPC, lastBlock)
	s.requireSignerPaid(da)
}
