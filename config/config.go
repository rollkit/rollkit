package config

import "time"

// NodeConfig stores Optimint node configuration.
type NodeConfig struct {
	P2P        P2PConfig
	Aggregator bool
	AggregatorConfig
	DALayer  string
	DAConfig []byte
}

type AggregatorConfig struct {
	BlockTime       time.Duration
	NamespaceID     [8]byte
	ProposerAddress []byte
}
