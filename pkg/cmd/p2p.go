package cmd

import (
	"fmt"
	"os"

	"cosmossdk.io/log"
	"github.com/multiformats/go-multiaddr"
	"github.com/spf13/cobra"

	"github.com/rollkit/rollkit/pkg/config"
)

// GetNodeID returns the node ID of the node
var GetNodeAddressCmd = &cobra.Command{
	Use:   "get-node-address",
	Short: "Retrieve the address of the node",
	Long:  "This command retrieves the address of the node in the specified directory (or current directory if not specified).",
	RunE: func(cmd *cobra.Command, args []string) error {

		logger := log.NewLogger(os.Stdout)

		homePath, err := cmd.Flags().GetString(config.FlagRootDir)
		if err != nil {
			return fmt.Errorf("error reading home flag: %w", err)
		}

		nodeConfig, err := ParseConfig(cmd, homePath)
		if err != nil {
			panic(err)
		}

		maddr, err := multiaddr.NewMultiaddr(nodeConfig.P2P.ListenAddress)
		if err != nil {
			return fmt.Errorf("error creating multiaddr: %w", err)
		}

		logger.Info(fmt.Sprintf("Node address: %s", maddr.String()))

		return nil
	},
}
