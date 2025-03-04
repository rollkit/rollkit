package config

import (
	"testing"
	"time"

	cmcfg "github.com/cometbft/cometbft/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
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
		{
			"Instrumentation",
			&cmcfg.Config{
				Instrumentation: &cmcfg.InstrumentationConfig{
					Prometheus:           true,
					PrometheusListenAddr: ":8888",
					MaxOpenConnections:   5,
					Namespace:            "cometbft",
				},
			},
			NodeConfig{
				Instrumentation: &InstrumentationConfig{
					Prometheus:           true,
					PrometheusListenAddr: ":8888",
					MaxOpenConnections:   5,
					Namespace:            "rollkit", // Should be converted to rollkit namespace
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var actual NodeConfig
			GetNodeConfig(&actual, c.input)

			if c.name == "Instrumentation" {
				// Special handling for Instrumentation test case
				assert.Equal(t, c.expected.Instrumentation.Prometheus, actual.Instrumentation.Prometheus)
				assert.Equal(t, c.expected.Instrumentation.PrometheusListenAddr, actual.Instrumentation.PrometheusListenAddr)
				assert.Equal(t, c.expected.Instrumentation.MaxOpenConnections, actual.Instrumentation.MaxOpenConnections)
				assert.Equal(t, c.expected.Instrumentation.Namespace, actual.Instrumentation.Namespace)
			} else {
				assert.Equal(t, c.expected, actual)
			}
		})
	}
}

// TODO need to update this test to account for all fields
func TestViperAndCobra(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	cmd := &cobra.Command{}
	AddFlags(cmd)

	v := viper.GetViper()
	assert.NoError(v.BindPFlags(cmd.Flags()))

	assert.NoError(cmd.Flags().Set(FlagAggregator, "true"))
	assert.NoError(cmd.Flags().Set(FlagDAAddress, `{"json":true}`))
	assert.NoError(cmd.Flags().Set(FlagBlockTime, "1234s"))
	assert.NoError(cmd.Flags().Set(FlagDANamespace, "0102030405060708"))

	nc := DefaultNodeConfig

	assert.NoError(nc.GetViperConfig(v))

	assert.Equal(true, nc.Aggregator)
	assert.Equal(`{"json":true}`, nc.DAAddress)
	assert.Equal(1234*time.Second, nc.BlockTime)
}
