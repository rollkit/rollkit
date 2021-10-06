package config

import "time"

// NodeConfig stores Optimint node configuration.
type NodeConfig struct {
	RootDir    string
	DBPath     string
	P2P        P2PConfig
	Aggregator bool
	BlockManagerConfig
	DALayer  string
	DAConfig []byte
}

// BlockManagerConfig consists of all parameters required by BlockManagerConfig
type BlockManagerConfig struct {
	BlockTime   time.Duration
	NamespaceID [8]byte
}
