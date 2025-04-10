package cmd

import (
	"os"
	"path/filepath"

	"cosmossdk.io/log"
	"github.com/spf13/cobra"

	coreda "github.com/rollkit/rollkit/core/da"
	"github.com/rollkit/rollkit/da"
	rollcmd "github.com/rollkit/rollkit/pkg/cmd"
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

		nodeConfig, err := rollcmd.ParseConfig(cmd)
		if err != nil {
			return err
		}

		// Create DA client with dummy DA
		dummyDA := coreda.NewDummyDA(100_000, 0, 0)
		dac := da.NewDAClient(dummyDA, 0, 1.0, []byte("test"), []byte(""), logger)

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

		sequencer, err := single.NewSequencer(
			logger,
			datastore,
			dummyDA,
			[]byte(nodeConfig.DA.Namespace),
			[]byte(nodeConfig.Node.SequencerRollupID),
			nodeConfig.Node.BlockTime.Duration,
			singleMetrics,
			nodeConfig.Node.Aggregator,
		)
		if err != nil {
			return err
		}

		p2pClient, err := p2p.NewClient(nodeConfig, "testapp", nodeKey, datastore, logger, p2p.NopMetrics())
		if err != nil {
			return err
		}

		return rollcmd.StartNode(logger, cmd, executor, sequencer, dac, nodeKey, p2pClient, datastore, nodeConfig)
	},
}
