package docker_e2e

import (
	"context"
	"cosmossdk.io/math"
	"encoding/hex"
	"fmt"
	"github.com/celestiaorg/go-square/v2/share"
	tastoradocker "github.com/celestiaorg/tastora/framework/docker"
	"github.com/celestiaorg/tastora/framework/testutil/sdkacc"
	"github.com/celestiaorg/tastora/framework/testutil/toml"
	tastoratypes "github.com/celestiaorg/tastora/framework/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module/testutil"
	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/cosmos/cosmos-sdk/x/bank"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap/zaptest"
)

const (
	// testChainID is the chain ID used for testing.
	// it must be the string "test" as it is handled explicitly in app/node.
	testChainID = "test"
)

type DockerTestSuite struct {
	suite.Suite
	provider     tastoratypes.Provider
	celestia     tastoratypes.Chain
	daNetwork    tastoratypes.DataAvailabilityNetwork
	rollkitChain tastoratypes.RollkitChain
}

func (s *DockerTestSuite) SetupSuite() {
	sdkConf := sdk.GetConfig()
	sdkConf.SetBech32PrefixForAccount("celestia", "celestiapub")
	sdkConf.Seal()
}

// ConfigOption is a function type for modifying tastoradocker.Config
type ConfigOption func(*tastoradocker.Config)

// CreateProvider creates a new tastoratypes.Provider with optional configuration modifications
func (s *DockerTestSuite) CreateProvider(opts ...ConfigOption) tastoratypes.Provider {
	t := s.T()
	encConfig := testutil.MakeTestEncodingConfig(auth.AppModuleBasic{}, bank.AppModuleBasic{})
	numValidators := 1
	numFullNodes := 0
	client, network := tastoradocker.DockerSetup(t)

	cfg := tastoradocker.Config{
		Logger:          zaptest.NewLogger(t),
		DockerClient:    client,
		DockerNetworkID: network,
		ChainConfig: &tastoradocker.ChainConfig{
			ConfigFileOverrides: map[string]any{
				"config/app.toml":    appOverrides(),
				"config/config.toml": configOverrides(),
			},
			Type:          "celestia",
			Name:          "celestia",
			Version:       "v4.0.0-rc6",
			NumValidators: &numValidators,
			NumFullNodes:  &numFullNodes,
			ChainID:       testChainID,
			Images: []tastoradocker.DockerImage{
				{
					Repository: "ghcr.io/celestiaorg/celestia-app",
					Version:    "v4.0.0-rc6",
					UIDGID:     "10001:10001",
				},
			},
			Bin:            "celestia-appd",
			Bech32Prefix:   "celestia",
			Denom:          "utia",
			CoinType:       "118",
			GasPrices:      "0.025utia",
			GasAdjustment:  1.3,
			EncodingConfig: &encConfig,
			AdditionalStartArgs: []string{
				"--force-no-bbr",
				"--grpc.enable",
				"--grpc.address",
				"0.0.0.0:9090",
				"--rpc.grpc_laddr=tcp://0.0.0.0:9098",
				"--timeout-commit", "1s",
			},
		},
		DataAvailabilityNetworkConfig: &tastoradocker.DataAvailabilityNetworkConfig{
			BridgeNodeCount: 1,
			Image: tastoradocker.DockerImage{
				Repository: "ghcr.io/celestiaorg/celestia-node",
				Version:    "pr-4283",
				UIDGID:     "10001:10001",
			},
		},
		RollkitChainConfig: &tastoradocker.RollkitChainConfig{
			ChainID:              "rollkit-test",
			Bin:                  "testapp",
			AggregatorPassphrase: "12345678",
			NumNodes:             1,
			Image: tastoradocker.DockerImage{
				//Repository: "ghcr.io/rollkit/rollkit",
				Repository: "rollkit",
				Version:    "latest",
				UIDGID:     "10001:10001",
			},
		},
	}

	for _, opt := range opts {
		opt(&cfg)
	}

	return tastoradocker.NewProvider(cfg, t)
}

// getGenesisHash returns the genesis hash of the given chain node.
func (s *DockerTestSuite) getGenesisHash(ctx context.Context) string {
	node := s.celestia.GetNodes()[0]
	c, err := node.GetRPCClient()
	s.Require().NoError(err, "failed to get node client")

	first := int64(1)
	block, err := c.Block(ctx, &first)
	s.Require().NoError(err, "failed to get block")

	genesisHash := block.Block.Header.Hash().String()
	s.Require().NotEmpty(genesisHash, "genesis hash is empty")
	return genesisHash
}

// SetupDockerResources creates a new provider and chain using the given configuration options.
// none of the resources are started.
func (s *DockerTestSuite) SetupDockerResources(opts ...ConfigOption) {
	s.provider = s.CreateProvider(opts...)
	s.celestia = s.CreateChain()
	s.daNetwork = s.CreateDANetwork()
	s.rollkitChain = s.CreateRollkitChain()
}

// CreateChain creates a chain using the provider.
func (s *DockerTestSuite) CreateChain() tastoratypes.Chain {
	ctx := context.Background()

	chain, err := s.provider.GetChain(ctx)
	s.Require().NoError(err)

	return chain
}

// CreateDANetwork creates a DA network using the provider
func (s *DockerTestSuite) CreateDANetwork() tastoratypes.DataAvailabilityNetwork {
	ctx := context.Background()

	daNetwork, err := s.provider.GetDataAvailabilityNetwork(ctx)
	s.Require().NoError(err)

	return daNetwork
}

// CreateRollkitChain creates a Rollkit chain using the provider
func (s *DockerTestSuite) CreateRollkitChain() tastoratypes.RollkitChain {
	ctx := context.Background()

	rollkitChain, err := s.provider.GetRollkitChain(ctx)
	s.Require().NoError(err)

	return rollkitChain
}

// StartBridgeNode initializes and starts a bridge node within the data availability network using the given parameters.
func (s *DockerTestSuite) StartBridgeNode(ctx context.Context, bridgeNode tastoratypes.DANode, chainID string, genesisHash string, celestiaNodeHostname string) {
	s.Require().Equal(tastoratypes.BridgeNode, bridgeNode.GetType())
	err := bridgeNode.Start(ctx,
		tastoratypes.WithChainID(chainID),
		tastoratypes.WithAdditionalStartArguments("--p2p.network", chainID, "--core.ip", celestiaNodeHostname, "--rpc.addr", "0.0.0.0"),
		tastoratypes.WithEnvironmentVariables(
			map[string]string{
				"CELESTIA_CUSTOM": tastoratypes.BuildCelestiaCustomEnvVar(chainID, genesisHash, ""),
				"P2P_NETWORK":     chainID,
			},
		),
	)
	s.Require().NoError(err)
}

// FundWallet transfers the specified amount of utia from the faucet wallet to the target wallet.
func (s *DockerTestSuite) FundWallet(ctx context.Context, wallet tastoratypes.Wallet, amount int64) {
	fromAddress, err := sdkacc.AddressFromWallet(s.celestia.GetFaucetWallet())
	s.Require().NoError(err)

	toAddress, err := sdk.AccAddressFromBech32(wallet.GetFormattedAddress())
	s.Require().NoError(err)

	bankSend := banktypes.NewMsgSend(fromAddress, toAddress, sdk.NewCoins(sdk.NewCoin("utia", math.NewInt(amount))))
	_, err = s.celestia.BroadcastMessages(ctx, s.celestia.GetFaucetWallet(), bankSend)
	s.Require().NoError(err)
}

// StartRollkitNode initializes and starts a Rollkit node.
func (s *DockerTestSuite) StartRollkitNode(ctx context.Context, bridgeNode tastoratypes.DANode, rollkitNode tastoratypes.RollkitNode) {
	err := rollkitNode.Init(ctx)
	s.Require().NoError(err)

	bridgeNodeHostName, err := bridgeNode.GetInternalHostName()
	s.Require().NoError(err)

	authToken, err := bridgeNode.GetAuthToken()
	s.Require().NoError(err)

	daAddress := fmt.Sprintf("http://%s:26658", bridgeNodeHostName)
	err = rollkitNode.Start(ctx,
		"--rollkit.da.address", daAddress,
		"--rollkit.da.gas_price", "0.025",
		"--rollkit.da.auth_token", authToken,
		"--rollkit.rpc.address", "0.0.0.0:7331", // bind to 0.0.0.0 so rpc is reachable from test host.
		"--rollkit.da.namespace", generateValidNamespaceHex(),
	)
	s.Require().NoError(err)
}

func generateValidNamespaceHex() string {
	return hex.EncodeToString(share.RandomBlobNamespaceID())
}

// appOverrides enables indexing of transactions so Broadcasting of transactions works
func appOverrides() toml.Toml {
	tomlCfg := make(toml.Toml)
	txIndex := make(toml.Toml)
	txIndex["indexer"] = "kv"
	tomlCfg["tx-index"] = txIndex
	return tomlCfg
}

func configOverrides() toml.Toml {
	tomlCfg := make(toml.Toml)
	txIndex := make(toml.Toml)
	txIndex["indexer"] = "kv"
	tomlCfg["tx_index"] = txIndex
	return tomlCfg
}
