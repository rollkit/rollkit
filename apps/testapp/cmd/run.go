package cmd

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	kvexecutor "github.com/rollkit/rollkit/apps/testapp/kv"
	"github.com/rollkit/rollkit/da/jsonrpc"
	rollcmd "github.com/rollkit/rollkit/pkg/cmd"
	"github.com/rollkit/rollkit/pkg/p2p"
	"github.com/rollkit/rollkit/pkg/p2p/key"
	"github.com/rollkit/rollkit/pkg/store"
	"github.com/rollkit/rollkit/sequencers/single"
)

var RunCmd = &cobra.Command{
	Use:     "start",
	Aliases: []string{"node", "run"},
	Short:   "Run the testapp node",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Logger setup is now primarily handled by rollcmd.SetupLogger using ipfs/go-log/v2
		// The command line flag config.FlagLogLevel will be read by rollcmd.ParseConfig,
		// and then rollcmd.SetupLogger will use it.

		nodeConfig, err := rollcmd.ParseConfig(cmd)
		if err != nil {
			return err
		}

		// logger is now logging.EventLogger
		logger := rollcmd.SetupLogger(nodeConfig.Log)

		// Get KV endpoint flag
		kvEndpoint, _ := cmd.Flags().GetString(flagKVEndpoint)
		if kvEndpoint == "" {
			logger.Info("KV endpoint flag not set, using default from http_server")
		}

		// Create test implementations
		executor, err := kvexecutor.NewKVExecutor(nodeConfig.RootDir, nodeConfig.DBPath)
		if err != nil {
			return err
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Pass logger to NewClient, assuming it expects logging.EventLogger or compatible interface
		daJrpc, err := jsonrpc.NewClient(ctx, logger, nodeConfig.DA.Address, nodeConfig.DA.AuthToken, nodeConfig.DA.Namespace)
		if err != nil {
			return err
		}

		nodeKey, err := key.LoadNodeKey(filepath.Dir(nodeConfig.ConfigPath()))
		if err != nil {
			return err
		}

		datastore, err := store.NewDefaultKVStore(nodeConfig.RootDir, nodeConfig.DBPath, "testapp")
		if err != nil {
			return err
		}

		singleMetrics, err := single.NopMetrics()
		if err != nil {
			return err
		}

		// Start the KV executor HTTP server
		if kvEndpoint != "" { // Only start if endpoint is provided
			httpServer := kvexecutor.NewHTTPServer(executor, kvEndpoint)
			err = httpServer.Start(ctx) // Use the main context for lifecycle management
			if err != nil {
				return fmt.Errorf("failed to start KV executor HTTP server: %w", err)
			} else {
				logger.Info("KV executor HTTP server started", "endpoint", kvEndpoint)
			}
		}

		sequencer, err := single.NewSequencer(
			ctx,
			logger, // Pass logger (logging.EventLogger)
			datastore,
			&daJrpc.DA,
			[]byte(nodeConfig.ChainID),
			nodeConfig.Node.BlockTime.Duration,
			singleMetrics,
			nodeConfig.Node.Aggregator,
		)
		if err != nil {
			return err
		}

		p2pClient, err := p2p.NewClient(nodeConfig, nodeKey, datastore, logger, p2p.NopMetrics()) // Pass logger (logging.EventLogger)
		if err != nil {
			return err
		}

		return rollcmd.StartNode(logger, cmd, executor, sequencer, &daJrpc.DA, p2pClient, datastore, nodeConfig, nil) // Pass logger (logging.EventLogger)
	},
}
