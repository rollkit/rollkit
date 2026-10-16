//go:build docker_e2e

package docker_e2e

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cosmossdk.io/math"
	keyring99 "github.com/99designs/keyring"
	"github.com/celestiaorg/tastora/framework/docker/consts"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module/testutil"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	rpcclient "github.com/evstack/ev-node/pkg/rpc/client"
)

const (
	// The tx signer links celestia-app v10.
	signedSubmitAppVersion  = "v10.4.0"
	signedSubmitNodeVersion = "v0.34.3"
	daSignerKeyName         = "evnode-da"
	daSignerFunds           = 10_000_000_000
	daIncludedHeightKey     = "d" // store.DAIncludedHeightKey; importing pkg/store pulls in KMS deps
)

// signedSubmitDA is a celestia-app v10 chain and bridge node with a funded
// DA signer key. The bridge wallet is never funded, so any blob that lands
// was paid for by the signer key.
type signedSubmitDA struct {
	daAddress   string // bridge JSON-RPC, container-internal
	authToken   string
	grpcAddress string // celestia-app gRPC, container-internal
	// keyFiles maps paths relative to the node home to keyring file contents.
	keyFiles map[string][]byte
	signer   sdk.AccAddress
	balance  func() (math.Int, error)
}

// setupSignedSubmitDA starts the DA side and creates a funded signer key in a
// keyring of the given backend. passphrase is only used by the file backend.
func (s *DockerTestSuite) setupSignedSubmitDA(ctx context.Context, backend, passphrase string) signedSubmitDA {
	s.setupDockerEnvironment()
	s.celestia = s.createChain(signedSubmitAppVersion)
	s.Require().NoError(s.celestia.Start(ctx))
	s.daNetwork = s.createDANetwork(signedSubmitNodeVersion)

	celInfo, err := s.celestia.GetNodes()[0].GetNetworkInfo(ctx)
	s.Require().NoError(err)
	bridgeNode := s.daNetwork.GetBridgeNodes()[0]
	s.StartBridgeNode(ctx, bridgeNode, celestiaChainID, s.getGenesisHash(ctx), celInfo.Internal.Hostname)

	keyDir := s.T().TempDir()
	cdc := testutil.MakeTestEncodingConfig().Codec
	var kr keyring.Keyring
	if backend == keyring.BackendFile {
		// Same way da/celestia opens it, without cosmos' terminal prompt.
		db, err := keyring99.Open(keyring99.Config{
			AllowedBackends:  []keyring99.BackendType{keyring99.FileBackend},
			FileDir:          filepath.Join(keyDir, "keyring-file"),
			FilePasswordFunc: func(string) (string, error) { return passphrase, nil },
		})
		s.Require().NoError(err)
		kr = keyring.NewInMemoryWithKeyring(db, cdc)
	} else {
		kr, err = keyring.New("evnode", backend, keyDir, nil, cdc)
		s.Require().NoError(err)
	}
	rec, _, err := kr.NewMnemonic(daSignerKeyName, keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, hd.Secp256k1)
	s.Require().NoError(err)
	signerAddr, err := rec.GetAddress()
	s.Require().NoError(err)

	keyFiles := map[string][]byte{}
	krDir := filepath.Join(keyDir, "keyring-"+backend)
	entries, err := os.ReadDir(krDir)
	s.Require().NoError(err)
	for _, f := range entries {
		bz, err := os.ReadFile(filepath.Join(krDir, f.Name()))
		s.Require().NoError(err)
		keyFiles[filepath.Join("keys", "keyring-"+backend, f.Name())] = bz
	}

	// Fund via the validator's CLI: tastora's RPC client cannot decode
	// celestia-app v10 tx results ("unknown field signers"), which breaks
	// BroadcastMessages.
	celNode := s.celestia.GetNode()
	_, stderr, err := celNode.Exec(ctx, []string{
		"celestia-appd", "tx", "bank", "send", consts.FaucetAccountKeyName, signerAddr.String(),
		fmt.Sprintf("%dutia", daSignerFunds),
		"--home", celNode.HomeDir(), "--keyring-backend", "test", "--chain-id", celestiaChainID,
		"--node", "tcp://" + celInfo.Internal.RPCAddress(), "--fees", "400000utia", "--yes",
	}, nil)
	s.Require().NoError(err, string(stderr))

	conn, err := grpc.NewClient(celInfo.External.GRPCAddress(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = conn.Close() })
	bank := banktypes.NewQueryClient(conn)
	balance := func() (math.Int, error) {
		res, err := bank.Balance(ctx, &banktypes.QueryBalanceRequest{Address: signerAddr.String(), Denom: "utia"})
		if err != nil {
			return math.Int{}, err
		}
		return res.Balance.Amount, nil
	}
	s.Require().Eventually(func() bool {
		bal, err := balance()
		return err == nil && bal.Equal(math.NewInt(daSignerFunds))
	}, 30*time.Second, time.Second, "DA signer was not funded")

	bridgeInfo, err := bridgeNode.GetNetworkInfo(ctx)
	s.Require().NoError(err)
	authToken, err := bridgeNode.GetAuthToken()
	s.Require().NoError(err)

	return signedSubmitDA{
		daAddress:   "http://" + bridgeInfo.Internal.RPCAddress(),
		authToken:   authToken,
		grpcAddress: celInfo.Internal.GRPCAddress(),
		keyFiles:    keyFiles,
		signer:      signerAddr,
		balance:     balance,
	}
}

// requireSignerPaid fails unless the signer's balance dropped below its funding.
func (s *DockerTestSuite) requireSignerPaid(da signedSubmitDA) {
	bal, err := da.balance()
	s.Require().NoError(err)
	s.T().Logf("signer %s balance %sutia", da.signer, bal)
	s.Require().True(bal.LT(math.NewInt(daSignerFunds)), "ev-node's key paid no fees")
}

// waitForDAIncluded polls ev-node's RPC until the DA included height reaches want.
func (s *DockerTestSuite) waitForDAIncluded(ctx context.Context, rpcURL string, want uint64) {
	rpc := rpcclient.NewClient(rpcURL)
	var daIncluded uint64
	s.Require().Eventually(func() bool {
		bz, err := rpc.GetMetadata(ctx, daIncludedHeightKey)
		if err != nil || len(bz) != 8 {
			return false
		}
		daIncluded = binary.LittleEndian.Uint64(bz)
		return daIncluded >= want
	}, 3*time.Minute, 2*time.Second, "DA included height did not reach %d", want)
	s.T().Logf("DA included height %d", daIncluded)
}

// TestSignedBlobSubmission runs testapp with da.tx_signer: ev-node signs
// PayForBlobs with its own key and broadcasts them to celestia-app gRPC.
func (s *DockerTestSuite) TestSignedBlobSubmission() {
	ctx := context.Background()
	da := s.setupSignedSubmitDA(ctx, keyring.BackendTest, "")
	s.evNodeChain = s.CreateEvolveChain()

	evNode := s.evNodeChain.GetNodes()[0]
	s.Require().NoError(evNode.Init(ctx))
	for path, bz := range da.keyFiles {
		s.Require().NoError(evNode.WriteFile(ctx, path, bz))
	}
	s.Require().NoError(evNode.Start(ctx,
		"--evnode.da.address", da.daAddress,
		"--evnode.da.auth_token", da.authToken,
		"--evnode.rpc.address", "0.0.0.0:7331",
		"--evnode.da.namespace", "ev-header",
		"--evnode.da.data_namespace", "ev-data",
		"--kv-endpoint", "0.0.0.0:8080",
		"--evnode.da.tx_signer.enabled",
		"--evnode.da.tx_signer.grpc_address", da.grpcAddress,
		"--evnode.da.tx_signer.grpc_tls=false",
		"--evnode.da.tx_signer.keyring_backend", "test",
		"--evnode.da.tx_signer.key_name", daSignerKeyName,
	))

	evInfo, err := evNode.GetNetworkInfo(ctx)
	s.Require().NoError(err)
	s.waitForDAIncluded(ctx, "http://"+strings.Replace(evInfo.External.RPCAddress(), "0.0.0.0", "localhost", 1), 5)
	s.requireSignerPaid(da)
}
