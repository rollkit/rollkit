package config

import (
	"time"

	"github.com/rollkit/rollkit/types"
)

const (
	// DefaultListenAddress is a default listen address for P2P client.
	DefaultListenAddress = "/ip4/0.0.0.0/tcp/7676"
	Version              = "0.4.0"
)

// DefaultNodeConfig keeps default values of NodeConfig
var DefaultNodeConfig = NodeConfig{
	P2P: P2PConfig{
		ListenAddress: DefaultListenAddress,
		Seeds:         "",
	},
	Aggregator:     false,
	LazyAggregator: false,
	BlockManagerConfig: BlockManagerConfig{
		BlockTime:   30 * time.Second,
		NamespaceID: types.NamespaceID{},
	},
	DALayer:  "mock",
	DAConfig: "",
	Light:    false,
	HeaderConfig: HeaderConfig{
		TrustedHash: "",
	},
}
