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
		name     string
		input    *Config
		expected NodeConfig
	}{
		{"empty", nil, NodeConfig{
			RootDir: "~/.rollkit",
			DBPath:  "~/.rollkit/data",
		}}, // With defaults
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var actual NodeConfig
			GetNodeConfig(&actual)
			assert.Equal(t, c.expected, actual)
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
