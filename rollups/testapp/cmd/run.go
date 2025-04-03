package cmd

import (
	"fmt"
	"os"

	"cosmossdk.io/log"
	"github.com/spf13/cobra"

	coreda "github.com/rollkit/rollkit/core/da"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	"github.com/rollkit/rollkit/da"
	rollcmd "github.com/rollkit/rollkit/pkg/cmd"
	"github.com/rollkit/rollkit/pkg/config"
	"github.com/rollkit/rollkit/pkg/p2p"
	"github.com/rollkit/rollkit/pkg/p2p/key"
	"github.com/rollkit/rollkit/pkg/store"
	kvexecutor "github.com/rollkit/rollkit/rollups/testapp/kv"
)

var RunCmd = &cobra.Command{
	Use:     "start",
	Aliases: []string{"node", "run"},
	Short:   "Run the testapp node",
	RunE: func(cmd *cobra.Command, args []string) error {

		// Create test implementations
		// TODO: we need to start the executor http server
		executor := kvexecutor.CreateDirectKVExecutor()
		sequencer := coresequencer.NewDummySequencer()

		homePath, err := cmd.Flags().GetString(config.FlagRootDir)
		if err != nil {
			return fmt.Errorf("error reading home flag: %w", err)
		}

		nodeConfig, err := rollcmd.ParseConfig(cmd, homePath)
		if err != nil {
			panic(err)
		}

		// Create DA client with dummy DA
		dummyDA := coreda.NewDummyDA(100_000, 0, 0)
		logger := log.NewLogger(os.Stdout)
		dac := da.NewDAClient(dummyDA, 0, 1.0, []byte("test"), []byte(""), logger)

		nodeKey, err := key.LoadOrGenNodeKey(nodeConfig.ConfigDir)
		if err != nil {
			panic(err)
		}

		datastore, err := store.NewDefaultInMemoryKVStore()
		if err != nil {
			panic(err)
		}

		p2pClient, err := p2p.NewClient(nodeConfig, "testapp", nodeKey, datastore, logger, nil)
		if err != nil {
			panic(err)
		}

		return rollcmd.StartNode(cmd, executor, sequencer, dac, nodeKey, p2pClient, datastore, nodeConfig)
	},
}
