package p2p

import "github.com/cometbft/cometbft/p2p"

// PeerConnection describe basic information about P2P connection.
type PeerConnection struct {
	NodeInfo         p2p.DefaultNodeInfo  `json:"node_info"`
	IsOutbound       bool                 `json:"is_outbound"`
	ConnectionStatus p2p.ConnectionStatus `json:"connection_status"`
	RemoteIP         string               `json:"remote_ip"`
}
