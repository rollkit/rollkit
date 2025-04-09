package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"connectrpc.com/connect"
	"cosmossdk.io/log"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/rollkit/rollkit/pkg/config"
	rpc "github.com/rollkit/rollkit/types/pb/rollkit/v1/v1connect"
)

// NodeInfoCmd returns information about the running node via RPC
var NodeInfoCmd = &cobra.Command{
	Use:   "node-info",
	Short: "Get information about a running node via RPC",
	Long:  "This command retrieves the node information via RPC from a running node in the specified directory (or current directory if not specified).",
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := log.NewLogger(os.Stdout)

		homePath, err := cmd.Flags().GetString(config.FlagRootDir)
		if err != nil {
			return fmt.Errorf("error reading home flag: %w", err)
		}

		nodeConfig, err := ParseConfig(cmd, homePath)
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

		// Create P2P client
		p2pClient := rpc.NewP2PServiceClient(
			&httpClient,
			rpcAddress,
		)

		// Call GetNetInfo RPC
		resp, err := p2pClient.GetNetInfo(
			context.Background(),
			connect.NewRequest(&emptypb.Empty{}),
		)
		if err != nil {
			return fmt.Errorf("error calling GetNetInfo RPC: %w", err)
		}

		// Print node information
		netInfo := resp.Msg.NetInfo
		logger.Info("Node information",
			"id", netInfo.Id,
			"listen_address", netInfo.ListenAddress)

		// Also get peer information
		peerResp, err := p2pClient.GetPeerInfo(
			context.Background(),
			connect.NewRequest(&emptypb.Empty{}),
		)
		if err != nil {
			return fmt.Errorf("error calling GetPeerInfo RPC: %w", err)
		}

		// Print connected peers
		logger.Info(fmt.Sprintf("Connected peers: %d", len(peerResp.Msg.Peers)))
		for i, peer := range peerResp.Msg.Peers {
			logger.Info(fmt.Sprintf("Peer #%d", i+1),
				"id", peer.Id,
				"address", peer.Address)
		}

		return nil
	},
}
