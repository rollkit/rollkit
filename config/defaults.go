package config

import "time"

const (
	// DefaultListenAddress is a default listen address for P2P client.
	DefaultListenAddress = "/ip4/0.0.0.0/tcp/7676"
)

// DefaultNodeConfig keeps default values of NodeConfig
var DefaultNodeConfig = NodeConfig{
	P2P: P2PConfig{
		ListenAddress: DefaultListenAddress,
		Seeds:         "",
	},
	Aggregator: false,
	BlockManagerConfig: BlockManagerConfig{
		BlockTime:   30 * time.Second,
		NamespaceID: [8]byte{},
	},
	DALayer:  "mock",
	DAConfig: "",
}
