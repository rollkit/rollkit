package docker_e2e

import (
	"context"
	tastoradocker "github.com/celestiaorg/tastora/framework/docker"
	"github.com/celestiaorg/tastora/framework/testutil/toml"
	tastoratypes "github.com/celestiaorg/tastora/framework/types"
	"github.com/cosmos/cosmos-sdk/types/module/testutil"
	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/cosmos/cosmos-sdk/x/bank"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"testing"
)

func defaultRollkitProvider(t *testing.T) tastoratypes.Provider {
	encConfig := testutil.MakeTestEncodingConfig(auth.AppModuleBasic{}, bank.AppModuleBasic{})
	numValidators := 1
	numFullNodes := 1
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
			ChainID:       "test",
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
				"--timeout-commit", "1s", // shorter block time.
			},
		},
		DataAvailabilityNetworkConfig: &tastoradocker.DataAvailabilityNetworkConfig{
			BridgeNodeCount: 1,
			Image: tastoradocker.DockerImage{
				Repository: "ghcr.io/celestiaorg/celestia-node",
				// TODO: includes fix for signer which enables the funding of accounts mid test.
				Version: "pr-4283",
				UIDGID:  "10001:10001",
			},
		},
		RollkitChainConfig: &tastoradocker.RollkitChainConfig{
			ChainID:              "rollkit-test",
			Bin:                  "testapp",
			AggregatorPassphrase: "12345678",
			NumNodes:             1,
			Image: tastoradocker.DockerImage{
				Repository: "ghcr.io/rollkit/rollkit",
				Version:    "latest",
				UIDGID:     "10001:10001",
			},
		},
	}

	return tastoradocker.NewProvider(cfg, t)
}

func TestBasicDockerE2E(t *testing.T) {
	ctx := context.Background()
	provider := defaultRollkitProvider(t)

	chain, err := provider.GetChain(ctx)
	require.NoError(t, err)

	err = chain.Start(ctx)
	require.NoError(t, err)
}

// enable indexing of transactions so Broadcasting of transactions works.
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
