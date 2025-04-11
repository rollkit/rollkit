package cmd

import (
	"fmt"
	"os"

	"cosmossdk.io/log"
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

		logger := log.NewLogger(os.Stdout)

		// Create test implementations
		// TODO: we need to start the executor http server
		executor := kvexecutor.CreateDirectKVExecutor()

		homePath, err := cmd.Flags().GetString(config.FlagRootDir)
		if err != nil {
			return fmt.Errorf("error reading home flag: %w", err)
		}

		nodeConfig, err := rollcmd.ParseConfig(cmd, homePath)
		if err != nil {
			panic(err)
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

		nodeKey, err := key.LoadNodeKey(nodeConfig.ConfigDir)
		if err != nil {
			panic(err)
		}

		datastore, err := store.NewDefaultKVStore(nodeConfig.RootDir, nodeConfig.DBPath, "testapp")
		if err != nil {
			panic(err)
		}

		singleMetrics, err := single.NopMetrics()
		if err != nil {
			panic(err)
		}

		sequencer, err := single.NewSequencer(logger, datastore, daJrpc, []byte(nodeConfig.DA.Namespace), []byte(nodeConfig.ChainID), nodeConfig.Node.BlockTime.Duration, singleMetrics, nodeConfig.Node.Aggregator)
		if err != nil {
			panic(err)
		}

		p2pClient, err := p2p.NewClient(nodeConfig, nodeKey, datastore, logger, p2p.NopMetrics())
		if err != nil {
			panic(err)
		}

		return rollcmd.StartNode(logger, cmd, executor, sequencer, dac, nodeKey, p2pClient, datastore, nodeConfig)
	},
}
