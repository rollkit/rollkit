package config

// P2PConfig stores configuration related to peer-to-peer networking.
type P2PConfig struct {
	ListenAddress string // Address to listen for incoming connections
	Seeds         string // Comma separated list of seed nodes to connect to
}
