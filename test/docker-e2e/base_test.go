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

	aggregatorPass := "12345678"

	cfg := tastoradocker.Config{
		Logger:          zaptest.NewLogger(t),
		DockerClient:    client,
		DockerNetworkID: network,
		ChainConfig: &tastoradocker.ChainConfig{
			ConfigFileOverrides: map[string]any{
				"config/app.toml":    appOverrides(),
				"config/config.toml": configOverrides(),
			},
			Type:          "rollkit",
			Name:          "celestia",
			Version:       "latest",
			NumValidators: &numValidators,
			NumFullNodes:  &numFullNodes,
			ChainID:       "testing",
			Images: []tastoradocker.DockerImage{
				{
					Repository: "rollkit",
					Version:    "latest",
					UIDGID:     "10001:10001",
				},
			},
			Bin:            "testapp",
			Bech32Prefix:   "celestia",
			Denom:          "utia",
			CoinType:       "118",
			GasPrices:      "0.025utia",
			GasAdjustment:  1.3,
			EncodingConfig: &encConfig,
			ChainNodeConfig: []tastoradocker.ChainNodeConfig{
				{
					AdditionalStartArgs: []string{},
					AdditionalInitArgs:  []string{},
				},
				{

					AdditionalStartArgs: []string{
						"--rollkit.node.aggregator",
						"--rollkit.signer.passphrase=" + aggregatorPass,
						"--rollkit.node.block_time=5ms",
						"--rollkit.da.block_time=15ms",
						"--kv-endpoint=0.0.0.0:9090",
					},
					AdditionalInitArgs: []string{
						"--rollkit.node.aggregator",
						"--rollkit.signer.passphrase=" + aggregatorPass,
					},
				},
				//{
				//	AdditionalStartArgs: nil,
				//	AdditionalInitArgs:  nil,
				//},
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
