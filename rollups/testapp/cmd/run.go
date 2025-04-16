package cmd

import (
	"context"
	"os"
	"path/filepath"

	"cosmossdk.io/log"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"

	"github.com/rollkit/rollkit/da"
	"github.com/rollkit/rollkit/da/proxy"
	rollcmd "github.com/rollkit/rollkit/pkg/cmd"
	"github.com/rollkit/rollkit/pkg/config"
	"github.com/rollkit/rollkit/pkg/p2p"
	"github.com/rollkit/rollkit/pkg/p2p/key"
	"github.com/rollkit/rollkit/pkg/store"
	kvexecutor "github.com/rollkit/rollkit/rollups/testapp/kv"
	"github.com/rollkit/rollkit/sequencers/single"
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

		logger := log.NewLogger(os.Stdout, opts...)

		// Create test implementations
		// TODO: we need to start the executor http server
		executor := kvexecutor.CreateDirectKVExecutor()

		nodeConfig, err := rollcmd.ParseConfig(cmd)
		if err != nil {
			return err
		}

		daJrpc, err := proxy.NewClient(nodeConfig.DA.Address, nodeConfig.DA.AuthToken)
		if err != nil {
			panic(err)
		}

		dac := da.NewDAClient(
			daJrpc,
			nodeConfig.DA.GasPrice,
			nodeConfig.DA.GasMultiplier,
			[]byte(nodeConfig.DA.Namespace),
			[]byte(nodeConfig.DA.SubmitOptions),
			logger,
		)

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

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		sequencer, err := single.NewSequencer(
			ctx,
			logger,
			datastore,
			daJrpc,
			[]byte(nodeConfig.DA.Namespace),
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

		return rollcmd.StartNode(logger, cmd, executor, sequencer, dac, nodeKey, p2pClient, datastore, nodeConfig)
	},
}
