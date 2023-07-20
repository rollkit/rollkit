package conv

import (
	"testing"

	"github.com/stretchr/testify/assert"

	cmcfg "github.com/cometbft/cometbft/config"

	"github.com/rollkit/rollkit/config"
)

func TestGetNodeConfig(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		input    *cmcfg.Config
		expected config.NodeConfig
	}{
		{"empty", nil, config.NodeConfig{}},
		{"Seeds", &cmcfg.Config{P2P: &cmcfg.P2PConfig{Seeds: "seeds"}}, config.NodeConfig{P2P: config.P2PConfig{Seeds: "seeds"}}},
		{"ListenAddress", &cmcfg.Config{P2P: &cmcfg.P2PConfig{ListenAddress: "127.0.0.1:7676"}}, config.NodeConfig{P2P: config.P2PConfig{ListenAddress: "127.0.0.1:7676"}}},
		{"RootDir", &cmcfg.Config{BaseConfig: cmcfg.BaseConfig{RootDir: "~/root"}}, config.NodeConfig{RootDir: "~/root"}},
		{"DBPath", &cmcfg.Config{BaseConfig: cmcfg.BaseConfig{DBPath: "./database"}}, config.NodeConfig{DBPath: "./database"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var actual config.NodeConfig
			GetNodeConfig(&actual, c.input)
			assert.Equal(t, c.expected, actual)
		})
	}
}
