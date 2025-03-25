package p2p

// PeerConnection describe basic information about P2P connection.
type PeerConnection struct {
	NodeInfo   DefaultNodeInfo `json:"node_info"`
	IsOutbound bool            `json:"is_outbound"`
	RemoteIP   string          `json:"remote_ip"`
}

// DefaultNodeInfo is the basic node information exchanged
// between two peers
type DefaultNodeInfo struct {
	DefaultNodeID string `json:"id"`          // authenticated identifier
	ListenAddr    string `json:"listen_addr"` // accepting incoming

	// Check compatibility.
	Network string `json:"network"` // network/chain ID
}
