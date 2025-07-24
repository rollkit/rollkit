package cmd

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"text/tabwriter"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/emptypb"

	rpc "github.com/evstack/ev-node/types/pb/rollkit/v1/v1connect"
)

// NodeInfoCmd returns information about the running node via RPC
var NetInfoCmd = &cobra.Command{
	Use:   "net-info",
	Short: "Get information about a running node via RPC",
	Long:  "This command retrieves the node information via RPC from a running node in the specified directory (or current directory if not specified).",
	RunE: func(cmd *cobra.Command, args []string) error {
		nodeConfig, err := ParseConfig(cmd)
		if err != nil {
			return fmt.Errorf("error parsing config: %w", err)
		}

		// Get RPC address from config
		rpcAddress := nodeConfig.RPC.Address
		if rpcAddress == "" {
			return fmt.Errorf("RPC address not found in node configuration")
		}

		// Create HTTP client
		httpClient := http.Client{
			Transport: http.DefaultTransport,
		}

		baseURL := fmt.Sprintf("http://%s", rpcAddress)

		// Create P2P client
		p2pClient := rpc.NewP2PServiceClient(
			&httpClient,
			baseURL,
		)

		// Call GetNetInfo RPC
		resp, err := p2pClient.GetNetInfo(
			context.Background(),
			connect.NewRequest(&emptypb.Empty{}),
		)
		if err != nil {
			return fmt.Errorf("error calling GetNetInfo RPC: %w", err)
		}

		netInfo := resp.Msg.NetInfo
		nodeID := netInfo.Id

		out := cmd.OutOrStdout()
		w := tabwriter.NewWriter(out, 2, 0, 2, ' ', 0)

		fmt.Fprintf(w, "%s", strings.Repeat("=", 50))
		fmt.Fprintf(w, "📊 NODE INFORMATION")
		fmt.Fprintf(w, "%s\n", strings.Repeat("=", 50))
		fmt.Fprintf(w, "🆔 Node ID:      \033[1;36m%s\033[0m\n", nodeID) // Print Node ID once

		// Iterate through all listen addresses
		fmt.Fprintf(w, "📡 Listen Addrs:")
		for i, addr := range netInfo.ListenAddresses {
			fullAddress := fmt.Sprintf("%s/p2p/%s", addr, nodeID)
			fmt.Fprintf(w, "   [%d] Addr: \033[1;36m%s\033[0m\n", i+1, addr)
			fmt.Fprintf(w, "       Full: \033[1;32m%s\033[0m\n", fullAddress)
		}

		fmt.Fprintf(w, "%s\n", strings.Repeat("-", 50))
		// Also get peer information
		peerResp, err := p2pClient.GetPeerInfo(
			context.Background(),
			connect.NewRequest(&emptypb.Empty{}),
		)
		if err != nil {
			return fmt.Errorf("error calling GetPeerInfo RPC: %w", err)
		}

		// Print connected peers in a table-like format
		peerCount := len(peerResp.Msg.Peers)
		fmt.Fprintf(w, "👥 CONNECTED PEERS: \033[1;33m%d\033[0m\n", peerCount)

		if peerCount > 0 {
			fmt.Fprintf(w, "%s\n", strings.Repeat("-", 50))
			fmt.Fprintf(w, "%-5s %-20s %s\n", "NO.", "PEER ID", "ADDRESS")
			fmt.Fprintf(w, "%s\n", strings.Repeat("-", 50))

			for i, peer := range peerResp.Msg.Peers {
				// Truncate peer ID if it's too long for display
				peerID := peer.Id
				if len(peerID) > 18 {
					peerID = peerID[:15] + "..."
				}
				fmt.Fprintf(w, "%-5d \033[1;34m%-20s\033[0m %s\n", i+1, peerID, peer.Address)
			}
		} else {
			fmt.Fprintf(w, "\n\033[3;33mNo peers connected\033[0m")
		}

		fmt.Fprintf(w, "%s\n", strings.Repeat("=", 50))
		w.Flush()

		return nil
	},
}
