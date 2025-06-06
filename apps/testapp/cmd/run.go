package cmd

import (
	"context"
	"fmt"
	"path/filepath"

	"cosmossdk.io/log"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"

	kvexecutor "github.com/rollkit/rollkit/apps/testapp/kv"
	"github.com/rollkit/rollkit/block"
	"github.com/rollkit/rollkit/da/jsonrpc"
	rollcmd "github.com/rollkit/rollkit/pkg/cmd"
	"github.com/rollkit/rollkit/pkg/config"
	"github.com/rollkit/rollkit/pkg/p2p"
	"github.com/rollkit/rollkit/pkg/p2p/key"
	"github.com/rollkit/rollkit/pkg/store"
	"github.com/rollkit/rollkit/sequencers/single"
	"github.com/rollkit/rollkit/types"
)

var RunCmd = &cobra.Command{
	Use:     "start",
	Aliases: []string{"node", "run"},
	Short:   "Run the testapp node",
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := []log.Option{}
		logLevel, _ := cmd.Flags().GetString(config.FlagLogLevel)
		if logLevel != "" {
			zl, err := zerolog.ParseLevel(logLevel)
			if err != nil {
				return err
			}
			opts = append(opts, log.LevelOption(zl))
		}

		nodeConfig, err := rollcmd.ParseConfig(cmd)
		if err != nil {
			return err
		}

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
			logger,
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

		p2pClient, err := p2p.NewClient(nodeConfig, nodeKey, datastore, logger, p2p.NopMetrics())
		if err != nil {
			return err
		}

		return rollcmd.StartNode(
			logger,
			cmd,
			executor,
			sequencer,
			&daJrpc.DA,
			p2pClient,
			datastore,
			nodeConfig,
			block.WithSignaturePayloadProvider(types.CreateDefaultSignaturePayloadProvider()),
			block.WithCommitHashProvider(types.CreateDefaultCommitHashProvider()),
		)
	},
}
