package config

// P2PConfig stores configuration related to peer-to-peer networking.
type P2PConfig struct {
	ListenAddress string `mapstructure:"listen_address"` // Address to listen for incoming connections
	Seeds         string `mapstructure:"seeds"`          // Comma separated list of seed nodes to connect to
	BlockedPeers  string `mapstructure:"blocked_peers"`  // Comma separated list of nodes to ignore
	AllowedPeers  string `mapstructure:"allowed_peers"`  // Comma separated list of nodes to whitelist
}
