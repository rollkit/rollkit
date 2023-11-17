package config

import (
	"time"
)

const (
	// DefaultListenAddress is a default listen address for P2P client.
	DefaultListenAddress = "/ip4/0.0.0.0/tcp/7676"
	// Version is the current rollkit version and is used for checking RPC compatibility??
	Version = "0.4.0"
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
		BlockTime:   1 * time.Second,
		DABlockTime: 15 * time.Second,
	},
	DAConfig: "",
	Light:    false,
	HeaderConfig: HeaderConfig{
		TrustedHash: "",
	},
}
