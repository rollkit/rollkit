package config

import (
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestGetNodeConfig(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		viperValues map[string]interface{}
		expected    NodeConfig
	}{
		{
			"empty",
			map[string]interface{}{},
			NodeConfig{},
		},
		{
			"Seeds",
			map[string]interface{}{
				"p2p.seeds": "seeds",
			},
			NodeConfig{P2P: P2PConfig{Seeds: "seeds"}},
		},
		{
			"ListenAddress",
			map[string]interface{}{
				"p2p.laddr": "127.0.0.1:7676",
			},
			NodeConfig{P2P: P2PConfig{ListenAddress: "127.0.0.1:7676"}},
		},
		{
			"RootDir",
			map[string]interface{}{
				"root_dir": "~/root",
			},
			NodeConfig{RootDir: "~/root"},
		},
		{
			"DBPath",
			map[string]interface{}{
				"db_path": "./database",
			},
			NodeConfig{DBPath: "./database"},
		},
		{
			"Instrumentation",
			map[string]interface{}{
				"instrumentation.prometheus":             true,
				"instrumentation.prometheus_listen_addr": ":8888",
				"instrumentation.max_open_connections":   5,
				"instrumentation.namespace":              "rollkit",
			},
			NodeConfig{
				Instrumentation: &InstrumentationConfig{
					Prometheus:           true,
					PrometheusListenAddr: ":8888",
					MaxOpenConnections:   5,
					Namespace:            "rollkit",
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Create a new Viper instance and set the values directly
			v := viper.New()
			for key, value := range c.viperValues {
				v.Set(key, value)
			}

			// Read the config from Viper
			var actual NodeConfig
			err := actual.GetViperConfig(v)
			assert.NoError(t, err)

			if c.name == "Instrumentation" {
				// Special handling for Instrumentation test case
				assert.Equal(t, c.expected.Instrumentation.Prometheus, actual.Instrumentation.Prometheus)
				assert.Equal(t, c.expected.Instrumentation.PrometheusListenAddr, actual.Instrumentation.PrometheusListenAddr)
				assert.Equal(t, c.expected.Instrumentation.MaxOpenConnections, actual.Instrumentation.MaxOpenConnections)
				assert.Equal(t, c.expected.Instrumentation.Namespace, actual.Instrumentation.Namespace)
			} else {
				assert.Equal(t, c.expected.RootDir, actual.RootDir)
				assert.Equal(t, c.expected.DBPath, actual.DBPath)
				if c.name == "Seeds" {
					assert.Equal(t, c.expected.P2P.Seeds, actual.P2P.Seeds)
				} else if c.name == "ListenAddress" {
					assert.Equal(t, c.expected.P2P.ListenAddress, actual.P2P.ListenAddress)
				}
			}
		})
	}
}

// TestConfigFromViper tests that the config can be read directly from Viper
func TestConfigFromViper(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		viperValues map[string]interface{}
		expected    NodeConfig
	}{
		{
			"basic_config",
			map[string]interface{}{
				"root_dir":  "~/root",
				"db_path":   "./database",
				"p2p.laddr": "127.0.0.1:7676",
				"p2p.seeds": "seeds",
			},
			NodeConfig{
				RootDir: "~/root",
				DBPath:  "./database",
				P2P: P2PConfig{
					ListenAddress: "127.0.0.1:7676",
					Seeds:         "seeds",
				},
			},
		},
		{
			"instrumentation_config",
			map[string]interface{}{
				"instrumentation.prometheus":             true,
				"instrumentation.prometheus_listen_addr": ":8888",
				"instrumentation.max_open_connections":   5,
			},
			NodeConfig{
				Instrumentation: &InstrumentationConfig{
					Prometheus:           true,
					PrometheusListenAddr: ":8888",
					MaxOpenConnections:   5,
					Namespace:            "rollkit",
				},
			},
		},
		{
			"rollkit_specific_config",
			map[string]interface{}{
				FlagAggregator: true,
				FlagDAAddress:  "da-address",
				FlagBlockTime:  "10s",
			},
			NodeConfig{
				Aggregator: true,
				DAAddress:  "da-address",
				BlockManagerConfig: BlockManagerConfig{
					BlockTime: 10 * time.Second,
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := viper.New()
			for key, value := range c.viperValues {
				v.Set(key, value)
			}

			var actual NodeConfig
			err := actual.GetViperConfig(v)
			assert.NoError(t, err)

			if c.name == "instrumentation_config" {
				// Special handling for Instrumentation test case
				assert.Equal(t, c.expected.Instrumentation.Prometheus, actual.Instrumentation.Prometheus)
				assert.Equal(t, c.expected.Instrumentation.PrometheusListenAddr, actual.Instrumentation.PrometheusListenAddr)
				assert.Equal(t, c.expected.Instrumentation.MaxOpenConnections, actual.Instrumentation.MaxOpenConnections)
				assert.Equal(t, c.expected.Instrumentation.Namespace, actual.Instrumentation.Namespace)
			} else if c.name == "rollkit_specific_config" {
				assert.Equal(t, c.expected.Aggregator, actual.Aggregator)
				assert.Equal(t, c.expected.DAAddress, actual.DAAddress)
				assert.Equal(t, c.expected.BlockManagerConfig.BlockTime, actual.BlockManagerConfig.BlockTime)
			} else {
				assert.Equal(t, c.expected.RootDir, actual.RootDir)
				assert.Equal(t, c.expected.DBPath, actual.DBPath)
				assert.Equal(t, c.expected.P2P.ListenAddress, actual.P2P.ListenAddress)
				assert.Equal(t, c.expected.P2P.Seeds, actual.P2P.Seeds)
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
