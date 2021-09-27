package config

import "time"

// NodeConfig stores Optimint node configuration.
type NodeConfig struct {
	RootDir    string
	DBPath     string
	P2P        P2PConfig
	Aggregator bool
	AggregatorConfig
	DALayer  string
	DAConfig []byte
}

// AggregatorConfig consists of all parameters required by Aggregator.
type AggregatorConfig struct {
	BlockTime   time.Duration
	NamespaceID [8]byte
}
