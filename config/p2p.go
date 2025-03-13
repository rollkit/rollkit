package config

// P2PConfig stores configuration related to peer-to-peer networking.
type P2PConfig struct {
	ListenAddress string `mapstructure:"listen_address" toml:"ListenAddress" comment:"Address to listen for incoming connections"` // Address to listen for incoming connections
	Seeds         string `toml:"Seeds" comment:"Comma separated list of seed nodes to connect to"`                                 // Comma separated list of seed nodes to connect to
	BlockedPeers  string `mapstructure:"blocked_peers" toml:"BlockedPeers" comment:"Comma separated list of peer IDs to block"`    // Comma separated list of nodes to ignore
	AllowedPeers  string `mapstructure:"allowed_peers" toml:"AllowedPeers" comment:"Comma separated list of peer IDs to allow"`    // Comma separated list of nodes to whitelist
}
