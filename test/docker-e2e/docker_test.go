//go:build docker_e2e

package docker_e2e

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"cosmossdk.io/math"
	tastoradocker "github.com/celestiaorg/tastora/framework/docker"
	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/celestiaorg/tastora/framework/docker/cosmos"
	da "github.com/celestiaorg/tastora/framework/docker/dataavailability"
	"github.com/celestiaorg/tastora/framework/docker/evstack"
	"github.com/celestiaorg/tastora/framework/docker/evstack/evmsingle"
	"github.com/celestiaorg/tastora/framework/docker/evstack/reth"
	"github.com/celestiaorg/tastora/framework/testutil/sdkacc"
	tastoratypes "github.com/celestiaorg/tastora/framework/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module/testutil"
	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/cosmos/cosmos-sdk/x/bank"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/suite"
)

const (
	// celestiaChainID is the chain ID used for testing.
	// it must be the string "test" as it is handled explicitly in app/node.
	celestiaChainID = "test"
	// celestiaAppVersion specifies the tag of the celestia-app image to deploy in tests.
	celestiaAppVersion = "v5.0.2"

	// EVM test constants shared across EVM-related tests.
	evmTestChainID    = "1234"
	evmTestPrivateKey = "cece4f25ac74deb1468965160c7185e07dff413f23fcadb611b05ca37ab0a52e"
	evmTestToAddress  = "0x944fDcD1c868E3cC566C78023CcB38A32cDA836E"
	evmTestGasLimit   = uint64(22000)
)

func init() {
	sdkConf := sdk.GetConfig()
	sdkConf.SetBech32PrefixForAccount("celestia", "celestiapub")
	sdkConf.Seal()
}

func TestDockerSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping due to short mode")
	}
	suite.Run(t, new(DockerTestSuite))
}

type DockerTestSuite struct {
	suite.Suite
	celestia        *cosmos.Chain
	daNetwork       *da.Network
	evNodeChain     *evstack.Chain
	dockerClient    tastoratypes.TastoraDockerClient
	dockerNetworkID string
}

// setupDockerEnvironment sets up the basic Docker environment
func (s *DockerTestSuite) setupDockerEnvironment() {
	t := s.T()
	client, network := tastoradocker.Setup(t)

	// Store client and network ID in the suite for later use
	s.dockerClient = client
	s.dockerNetworkID = network
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

// SetupDockerResources creates chains using the builder pattern.
// none of the resources are started.
func (s *DockerTestSuite) SetupDockerResources() {
	s.setupDockerEnvironment()
	s.celestia = s.CreateChain()
	s.daNetwork = s.CreateDANetwork()
	s.evNodeChain = s.CreateEvolveChain()
}

// CreateChain creates a chain using the ChainBuilder pattern.
func (s *DockerTestSuite) CreateChain() *cosmos.Chain {
	return s.createChain(celestiaAppVersion)
}

func (s *DockerTestSuite) createChain(appVersion string) *cosmos.Chain {
	ctx := context.Background()
	t := s.T()
	encConfig := testutil.MakeTestEncodingConfig(auth.AppModuleBasic{}, bank.AppModuleBasic{})

	// Create chain using ChainBuilder pattern
	chain, err := cosmos.NewChainBuilder(t).
		WithName("celestia").
		WithChainID(celestiaChainID).
		WithBinaryName("celestia-appd").
		WithBech32Prefix("celestia").
		WithDenom("utia").
		WithCoinType("118").
		WithGasPrices("0.025utia").
		WithGasAdjustment(1.3).
		WithEncodingConfig(&encConfig).
		WithImage(container.NewImage("ghcr.io/celestiaorg/celestia-app", appVersion, "10001:10001")).
		WithAdditionalStartArgs(
			"--force-no-bbr",
			"--grpc.enable",
			"--grpc.address",
			"0.0.0.0:9090",
			"--rpc.grpc_laddr=tcp://0.0.0.0:9098",
			"--timeout-commit", "1s",
		).
		WithDockerClient(s.dockerClient).
		WithDockerNetworkID(s.dockerNetworkID).
		WithNode(cosmos.NewChainNodeConfigBuilder().
			WithNodeType(tastoratypes.NodeTypeValidator).
			Build()).
		Build(ctx)

	s.Require().NoError(err)
	return chain
}

// CreateDANetwork creates a DA network using the builder pattern
func (s *DockerTestSuite) CreateDANetwork() *da.Network {
	return s.createDANetwork("v0.25.3")
}

func (s *DockerTestSuite) createDANetwork(nodeVersion string) *da.Network {
	ctx := context.Background()
	t := s.T()

	bridgeNodeConfig := da.NewNodeBuilder().
		WithNodeType(tastoratypes.BridgeNode).
		Build()

	daNetwork, err := da.NewNetworkBuilder(t).
		WithDockerClient(s.dockerClient).
		WithDockerNetworkID(s.dockerNetworkID).
		WithImage(container.NewImage("ghcr.io/celestiaorg/celestia-node", nodeVersion, "10001:10001")).
		WithNode(bridgeNodeConfig).
		Build(ctx)
	s.Require().NoError(err)

	return daNetwork
}

// CreateEvolveChain creates an evstack chain using the builder pattern
func (s *DockerTestSuite) CreateEvolveChain() *evstack.Chain {
	ctx := context.Background()
	t := s.T()

	aggregatorNodeConfig := evstack.NewNodeBuilder().
		WithAggregator(true).
		Build()

	evNodeChain, err := evstack.NewChainBuilder(t).
		WithChainID("evchain-test").
		WithBinaryName("testapp").
		WithAggregatorPassphrase("12345678").
		WithImage(getEvNodeImage()).
		WithDockerClient(s.dockerClient).
		WithDockerNetworkID(s.dockerNetworkID).
		WithNode(aggregatorNodeConfig).
		Build(ctx)
	s.Require().NoError(err)

	return evNodeChain
}

// StartBridgeNode initializes and starts a bridge node within the data availability network using the given parameters.
func (s *DockerTestSuite) StartBridgeNode(ctx context.Context, bridgeNode *da.Node, chainID string, genesisHash string, celestiaNodeHostname string) {
	s.Require().Equal(tastoratypes.BridgeNode, bridgeNode.GetType())
	err := bridgeNode.Start(ctx,
		da.WithChainID(chainID),
		da.WithAdditionalStartArguments("--p2p.network", chainID, "--core.ip", celestiaNodeHostname, "--rpc.addr", "0.0.0.0"),
		da.WithEnvironmentVariables(
			map[string]string{
				"CELESTIA_CUSTOM": tastoratypes.BuildCelestiaCustomEnvVar(chainID, genesisHash, ""),
				"P2P_NETWORK":     chainID,
			},
		),
	)
	s.Require().NoError(err)
}

// FundWallet transfers the specified amount of utia from the faucet wallet to the target wallet.
func (s *DockerTestSuite) FundWallet(ctx context.Context, wallet *tastoratypes.Wallet, amount int64) {
	fromAddress, err := sdkacc.AddressFromWallet(s.celestia.GetFaucetWallet())
	s.Require().NoError(err)

	toAddress, err := sdk.AccAddressFromBech32(wallet.GetFormattedAddress())
	s.Require().NoError(err)

	bankSend := banktypes.NewMsgSend(fromAddress, toAddress, sdk.NewCoins(sdk.NewCoin("utia", math.NewInt(amount))))
	_, err = s.celestia.BroadcastMessages(ctx, s.celestia.GetFaucetWallet(), bankSend)
	s.Require().NoError(err)
}

// StartEVNode initializes and starts an Ev node.
func (s *DockerTestSuite) StartEVNode(ctx context.Context, bridgeNode *da.Node, evNode *evstack.Node) {
	s.StartEVNodeWithNamespace(ctx, bridgeNode, evNode, "ev-header", "ev-data")
}

// StartEVNodeWithNamespace initializes and starts an EV node with a specific namespace.
func (s *DockerTestSuite) StartEVNodeWithNamespace(ctx context.Context, bridgeNode *da.Node, evNode *evstack.Node, headerNamespace, dataNamespace string) {
	err := evNode.Init(ctx)
	s.Require().NoError(err)

	bridgeNetworkInfo, err := bridgeNode.GetNetworkInfo(ctx)
	s.Require().NoError(err)

	authToken, err := bridgeNode.GetAuthToken()
	s.Require().NoError(err)

	bridgeRPCAddress := bridgeNetworkInfo.Internal.RPCAddress()
	daAddress := fmt.Sprintf("http://%s", bridgeRPCAddress)
	err = evNode.Start(ctx,
		"--evnode.da.address", daAddress,
		"--evnode.da.auth_token", authToken,
		"--evnode.rpc.address", "0.0.0.0:7331", // bind to 0.0.0.0 so rpc is reachable from test host.
		"--evnode.da.namespace", headerNamespace,
		"--evnode.da.data_namespace", dataNamespace,
		"--kv-endpoint", "0.0.0.0:8080",
	)
	s.Require().NoError(err)
}

// getEvNodeImage returns the Docker image configuration for EV Node
// Uses EV_NODE_IMAGE_REPO and EV_NODE_IMAGE_TAG environment variables if set
// Defaults to locally built image using a unique tag to avoid registry conflicts
func getEvNodeImage() container.Image {
	repo := strings.TrimSpace(os.Getenv("EV_NODE_IMAGE_REPO"))
	if repo == "" {
		repo = "evstack"
	}

	tag := strings.TrimSpace(os.Getenv("EV_NODE_IMAGE_TAG"))
	if tag == "" {
		tag = "local-dev"
	}

	return container.NewImage(repo, tag, "10001:10001")
}

// RethSetupConfig holds the configuration returned after setting up a Reth node.
type RethSetupConfig struct {
	Node           *reth.Node
	EngineURL      string // internal container-to-container URL
	EthURL         string // internal container-to-container URL
	EthURLExternal string // host-accessible URL
	JWTSecret      string
	GenesisHash    string
}

// SetupRethNode creates and starts a Reth node, waiting for it to be ready.
func (s *DockerTestSuite) SetupRethNode(ctx context.Context, name string) RethSetupConfig {
	rethNode, err := reth.NewNodeBuilder(s.T()).
		WithName(name).
		WithGenesis([]byte(reth.DefaultEvolveGenesisJSON())).
		WithDockerClient(s.dockerClient).
		WithDockerNetworkID(s.dockerNetworkID).
		Build(ctx)
	s.Require().NoError(err)
	s.Require().NoError(rethNode.Start(ctx))

	var setup RethSetupConfig
	s.Require().Eventually(func() bool {
		networkInfo, err := rethNode.GetNetworkInfo(ctx)
		if err != nil {
			return false
		}
		conn, err := net.DialTimeout("tcp", net.JoinHostPort("0.0.0.0", networkInfo.External.Ports.RPC), time.Second)
		if err != nil {
			return false
		}
		conn.Close()
		setup.EngineURL = fmt.Sprintf("http://%s:%s", networkInfo.Internal.Hostname, networkInfo.Internal.Ports.Engine)
		setup.EthURL = fmt.Sprintf("http://%s:%s", networkInfo.Internal.Hostname, networkInfo.Internal.Ports.RPC)
		setup.EthURLExternal = fmt.Sprintf("http://0.0.0.0:%s", networkInfo.External.Ports.RPC)
		return true
	}, 60*time.Second, 2*time.Second, "reth did not start in time")

	genesisHash, err := rethNode.GenesisHash(ctx)
	s.Require().NoError(err)

	setup.Node = rethNode
	setup.JWTSecret = rethNode.JWTSecretHex()
	setup.GenesisHash = genesisHash
	return setup
}

// SetupCelestiaAndDABridge starts Celestia chain and DA bridge, returns the DA address.
func (s *DockerTestSuite) SetupCelestiaAndDABridge(ctx context.Context) string {
	s.celestia = s.CreateChain()
	s.Require().NoError(s.celestia.Start(ctx))

	s.daNetwork = s.CreateDANetwork()
	bridgeNode := s.daNetwork.GetBridgeNodes()[0]

	chainID := s.celestia.GetChainID()
	genesisHash := s.getGenesisHash(ctx)
	networkInfo, err := s.celestia.GetNodes()[0].GetNetworkInfo(ctx)
	s.Require().NoError(err)

	s.StartBridgeNode(ctx, bridgeNode, chainID, genesisHash, networkInfo.Internal.Hostname)

	daWallet, err := bridgeNode.GetWallet()
	s.Require().NoError(err)
	s.FundWallet(ctx, daWallet, 100_000_000_00)

	bridgeNetworkInfo, err := bridgeNode.GetNetworkInfo(ctx)
	s.Require().NoError(err)
	return fmt.Sprintf("http://%s:%s", bridgeNetworkInfo.Internal.IP, bridgeNetworkInfo.Internal.Ports.RPC)
}

// WaitForEVMHealthy waits for an evmsingle node to respond to health checks.
func (s *DockerTestSuite) WaitForEVMHealthy(ctx context.Context, node *evmsingle.Node) {
	networkInfo, err := node.GetNetworkInfo(ctx)
	s.Require().NoError(err)

	healthURL := fmt.Sprintf("http://0.0.0.0:%s/health/live", networkInfo.External.Ports.RPC)
	s.Require().Eventually(func() bool {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 120*time.Second, 2*time.Second, "evm node did not become healthy")
}

// SetupEthClient creates an Ethereum client and verifies connectivity.
func (s *DockerTestSuite) SetupEthClient(ctx context.Context, url, expectedChainID string) *ethclient.Client {
	ethClient, err := ethclient.Dial(url)
	s.Require().NoError(err)

	chainID, err := ethClient.ChainID(ctx)
	s.Require().NoError(err)
	s.Require().Equal(expectedChainID, chainID.String())

	return ethClient
}

// WaitForTxIncluded waits for a transaction to be included in a block successfully.
func (s *DockerTestSuite) WaitForTxIncluded(ctx context.Context, client *ethclient.Client, txHash common.Hash) {
	s.Require().Eventually(func() bool {
		receipt, err := client.TransactionReceipt(ctx, txHash)
		if err != nil {
			return false
		}
		return receipt.Status == 1
	}, 30*time.Second, time.Second, "transaction %s was not included", txHash.Hex())
}

// EVMSingleSetupConfig holds the configuration returned after setting up an EVM single node.
type EVMSingleSetupConfig struct {
	Chain *evmsingle.Chain
	Node  *evmsingle.Node
}

// SetupEVMSingle creates and starts an evm single node with the specified image and node config.
func (s *DockerTestSuite) SetupEVMSingle(ctx context.Context, image container.Image, nodeConfig evmsingle.NodeConfig) EVMSingleSetupConfig {
	chain, err := evmsingle.NewChainBuilder(s.T()).
		WithDockerClient(s.dockerClient).
		WithDockerNetworkID(s.dockerNetworkID).
		WithImage(image).
		WithBinary("evm").
		WithNode(nodeConfig).
		Build(ctx)
	s.Require().NoError(err)
	s.Require().Len(chain.Nodes(), 1)

	node := chain.Nodes()[0]
	s.Require().NoError(node.Start(ctx))
	s.WaitForEVMHealthy(ctx, node)

	return EVMSingleSetupConfig{Chain: chain, Node: node}
}
