package p2p

// PeerConnection describe basic information about P2P connection.
type PeerConnection struct {
	NodeInfo   DefaultNodeInfo `json:"node_info"`
	IsOutbound bool            `json:"is_outbound"`
	RemoteIP   string          `json:"remote_ip"`
}

// ProtocolVersion contains the protocol versions for the software.
type ProtocolVersion struct {
	P2P   uint64 `json:"p2p"`
	Block uint64 `json:"block"`
	App   uint64 `json:"app"`
}

// DefaultNodeInfo is the basic node information exchanged
// between two peers
type DefaultNodeInfo struct {
	DefaultNodeID string `json:"id"`          // authenticated identifier
	ListenAddr    string `json:"listen_addr"` // accepting incoming

	// Check compatibility.
	Network string `json:"network"` // network/chain ID
}
