package config

import "time"

type NodeConfig struct {
	P2P        P2PConfig
	Aggregator bool
	BlockTime  time.Duration
	DALayer    string
	DAConfig   []byte
}
