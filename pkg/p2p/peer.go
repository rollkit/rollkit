package p2p

// PeerConnection describe basic information about P2P connection.
type PeerConnection struct {
	NodeInfo   NodeInfo `json:"node_info"`
	IsOutbound bool     `json:"is_outbound"`
	RemoteIP   string   `json:"remote_ip"`
}

// NodeInfo is the basic node information exchanged
// between two peers
type NodeInfo struct {
	NodeID     string `json:"id"`          // authenticated identifier
	ListenAddr string `json:"listen_addr"` // accepting incoming

	// Check compatibility.
	Network string `json:"network"` // network/chain ID
}
