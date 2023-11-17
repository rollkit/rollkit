package config

import (
	"testing"
	"time"

	cmcfg "github.com/cometbft/cometbft/config"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"

	"github.com/rollkit/rollkit/types"
)

func TestGetNodeConfig(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		input    *cmcfg.Config
		expected NodeConfig
	}{
		{"empty", nil, NodeConfig{}},
		{"Seeds", &cmcfg.Config{P2P: &cmcfg.P2PConfig{Seeds: "seeds"}}, NodeConfig{P2P: P2PConfig{Seeds: "seeds"}}},
		{"ListenAddress", &cmcfg.Config{P2P: &cmcfg.P2PConfig{ListenAddress: "127.0.0.1:7676"}}, NodeConfig{P2P: P2PConfig{ListenAddress: "127.0.0.1:7676"}}},
		{"RootDir", &cmcfg.Config{BaseConfig: cmcfg.BaseConfig{RootDir: "~/root"}}, NodeConfig{RootDir: "~/root"}},
		{"DBPath", &cmcfg.Config{BaseConfig: cmcfg.BaseConfig{DBPath: "./database"}}, NodeConfig{DBPath: "./database"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var actual NodeConfig
			GetNodeConfig(&actual, c.input)
			assert.Equal(t, c.expected, actual)
		})
	}
}
func TestViperAndCobra(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	cmd := &cobra.Command{}
	AddFlags(cmd)

	v := viper.GetViper()
	assert.NoError(v.BindPFlags(cmd.Flags()))

	assert.NoError(cmd.Flags().Set(flagAggregator, "true"))
	assert.NoError(cmd.Flags().Set(flagDAConfig, `{"json":true}`))
	assert.NoError(cmd.Flags().Set(flagBlockTime, "1234s"))
	assert.NoError(cmd.Flags().Set(flagNamespaceID, "0102030405060708"))

	nc := DefaultNodeConfig
	assert.NoError(nc.GetViperConfig(v))

	assert.Equal(true, nc.Aggregator)
	assert.Equal(`{"json":true}`, nc.DAConfig)
	assert.Equal(1234*time.Second, nc.BlockTime)
	assert.Equal(types.NamespaceID{1, 2, 3, 4, 5, 6, 7, 8}, nc.NamespaceID)
}
