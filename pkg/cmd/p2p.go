package cmd

import (
	"fmt"
	"os"

	"cosmossdk.io/log"
	"github.com/spf13/cobra"

	"github.com/rollkit/rollkit/pkg/config"
	"github.com/rollkit/rollkit/pkg/p2p"
	"github.com/rollkit/rollkit/pkg/p2p/key"
	"github.com/rollkit/rollkit/pkg/store"
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

		datastore, err := store.NewDefaultKVStore(nodeConfig.RootDir, nodeConfig.DBPath, "testapp")
		if err != nil {
			panic(err)
		}

		nodeKey, err := key.LoadNodeKey(nodeConfig.ConfigDir)
		if err != nil {
			panic(err)
		}

		p2pClient, err := p2p.NewClient(nodeConfig, nodeKey, datastore, logger, p2p.NopMetrics())
		if err != nil {
			panic(err)
		}
		addresses := p2pClient.GetAddresses()

		fmt.Println("P2P Addresses:", addresses)

		return nil
	},
}
