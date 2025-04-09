package p2p

import "github.com/libp2p/go-libp2p/core/peer"

// PeerManager defines the interface for managing peer connections
type PeerManager interface {
	// GetPeers returns information about connected peers
	GetPeers() ([]peer.AddrInfo, error)
	// GetNetworkInfo returns network information
	GetNetworkInfo() (NetworkInfo, error)
}

// NetworkInfo represents network information
type NetworkInfo struct {
	ID             string
	ListenAddress  string
	ConnectedPeers []peer.ID
}
