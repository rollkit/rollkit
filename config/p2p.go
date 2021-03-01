package config

type P2PConfig struct {
	ListenAddress string // Address to listen for incoming connections
	Seeds         string // Comma separated list of seed nodes to connect to
}
