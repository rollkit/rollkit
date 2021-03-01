package conv

import (
	"testing"

	"github.com/stretchr/testify/assert"

	tmcfg "github.com/lazyledger/lazyledger-core/config"
	"github.com/lazyledger/optimint/config"
)

func TestGetNodeConfig(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		input    *tmcfg.Config
		expected config.NodeConfig
	}{
		{"empty", nil, config.NodeConfig{}},
		{"Seeds", &tmcfg.Config{P2P: &tmcfg.P2PConfig{Seeds: "seeds"}}, config.NodeConfig{P2P: config.P2PConfig{Seeds: "seeds"}}},
		{"ListenAddress", &tmcfg.Config{P2P: &tmcfg.P2PConfig{ListenAddress: "127.0.0.1:7676"}}, config.NodeConfig{P2P: config.P2PConfig{ListenAddress: "127.0.0.1:7676"}}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actual := GetNodeConfig(c.input)
			assert.Equal(t, c.expected, actual)
		})
	}
}
