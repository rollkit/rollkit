package config

import "time"

// NodeConfig stores Optimint node configuration.
type NodeConfig struct {
	P2P        P2PConfig
	Aggregator bool
	BlockTime  time.Duration
	DALayer    string
	DAConfig   []byte
}
