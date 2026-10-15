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

// TestSignedBlobSubmission runs testapp with da.tx_signer: ev-node signs
// PayForBlobs with its own key and broadcasts them to celestia-app gRPC. The
// bridge wallet is never funded, so any blob that lands was paid for by
// ev-node's key.
func (s *DockerTestSuite) TestSignedBlobSubmission() {
	ctx := context.Background()
	s.setupDockerEnvironment()
	s.celestia = s.createChain(signedSubmitAppVersion)
	s.Require().NoError(s.celestia.Start(ctx))
	s.daNetwork = s.createDANetwork(signedSubmitNodeVersion)
	s.evNodeChain = s.CreateEvolveChain()

	celInfo, err := s.celestia.GetNodes()[0].GetNetworkInfo(ctx)
	s.Require().NoError(err)
	bridgeNode := s.daNetwork.GetBridgeNodes()[0]
	s.StartBridgeNode(ctx, bridgeNode, celestiaChainID, s.getGenesisHash(ctx), celInfo.Internal.Hostname)

	keyDir := s.T().TempDir()
	encCfg := testutil.MakeTestEncodingConfig()
	kr, err := keyring.New("evnode", keyring.BackendTest, keyDir, nil, encCfg.Codec)
	s.Require().NoError(err)
	rec, _, err := kr.NewMnemonic(daSignerKeyName, keyring.English, sdk.FullFundraiserPath, keyring.DefaultBIP39Passphrase, hd.Secp256k1)
	s.Require().NoError(err)
	signerAddr, err := rec.GetAddress()
	s.Require().NoError(err)

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
	defer conn.Close()
	bank := banktypes.NewQueryClient(conn)
	signerBalance := func() math.Int {
		res, err := bank.Balance(ctx, &banktypes.QueryBalanceRequest{Address: signerAddr.String(), Denom: "utia"})
		if err != nil {
			return math.ZeroInt()
		}
		return res.Balance.Amount
	}
	s.Require().Eventually(func() bool { return signerBalance().Equal(math.NewInt(daSignerFunds)) },
		30*time.Second, time.Second, "DA signer was not funded")

	evNode := s.evNodeChain.GetNodes()[0]
	s.Require().NoError(evNode.Init(ctx))
	keyFiles, err := os.ReadDir(filepath.Join(keyDir, "keyring-test"))
	s.Require().NoError(err)
	for _, f := range keyFiles {
		bz, err := os.ReadFile(filepath.Join(keyDir, "keyring-test", f.Name()))
		s.Require().NoError(err)
		s.Require().NoError(evNode.WriteFile(ctx, filepath.Join("keys", "keyring-test", f.Name()), bz))
	}

	bridgeInfo, err := bridgeNode.GetNetworkInfo(ctx)
	s.Require().NoError(err)
	authToken, err := bridgeNode.GetAuthToken()
	s.Require().NoError(err)
	s.Require().NoError(evNode.Start(ctx,
		"--evnode.da.address", "http://"+bridgeInfo.Internal.RPCAddress(),
		"--evnode.da.auth_token", authToken,
		"--evnode.rpc.address", "0.0.0.0:7331",
		"--evnode.da.namespace", "ev-header",
		"--evnode.da.data_namespace", "ev-data",
		"--kv-endpoint", "0.0.0.0:8080",
		"--evnode.da.tx_signer.enabled",
		"--evnode.da.tx_signer.grpc_address", celInfo.Internal.GRPCAddress(),
		"--evnode.da.tx_signer.grpc_tls=false",
		"--evnode.da.tx_signer.keyring_backend", "test",
		"--evnode.da.tx_signer.key_name", daSignerKeyName,
	))

	evInfo, err := evNode.GetNetworkInfo(ctx)
	s.Require().NoError(err)
	rpc := rpcclient.NewClient("http://" + strings.Replace(evInfo.External.RPCAddress(), "0.0.0.0", "localhost", 1))
	var daIncluded uint64
	s.Require().Eventually(func() bool {
		bz, err := rpc.GetMetadata(ctx, daIncludedHeightKey)
		if err != nil || len(bz) != 8 {
			return false
		}
		daIncluded = binary.LittleEndian.Uint64(bz)
		return daIncluded >= 5
	}, 3*time.Minute, 2*time.Second, "DA included height did not reach 5")

	bal := signerBalance()
	s.T().Logf("DA included height %d; signer %s balance %sutia", daIncluded, signerAddr, bal)
	s.Require().True(bal.LT(math.NewInt(daSignerFunds)), "ev-node's key paid no fees")
}
