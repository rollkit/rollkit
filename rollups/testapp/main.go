package main

import (
	"context"
	"fmt"
	"os"

	"cosmossdk.io/log"

	coreda "github.com/rollkit/rollkit/core/da"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	"github.com/rollkit/rollkit/da"
	rollcmd "github.com/rollkit/rollkit/pkg/cmd"
	"github.com/rollkit/rollkit/pkg/config"
	"github.com/rollkit/rollkit/pkg/p2p"
	"github.com/rollkit/rollkit/pkg/p2p/key"
	filesigner "github.com/rollkit/rollkit/pkg/signer/file"
	"github.com/rollkit/rollkit/pkg/store"
	commands "github.com/rollkit/rollkit/rollups/testapp/cmd"
	testExecutor "github.com/rollkit/rollkit/rollups/testapp/kv"
)

func main() {
	// Initiate the root command
	rootCmd := commands.RootCmd

	// Create context for the executor
	ctx := context.Background()

	// Create test implementations
	// TODO: we need to start the executor http server
	executor := testExecutor.CreateDirectKVExecutor(ctx)
	sequencer := coresequencer.NewDummySequencer()

	// Create DA client with dummy DA
	dummyDA := coreda.NewDummyDA(100_000, 0, 0)
	logger := log.NewLogger(os.Stdout)
	dac := da.NewDAClient(dummyDA, 0, 1.0, []byte("test"), []byte(""), logger)

	// Create key provider
	keyProvider, err := filesigner.NewFileSystemSigner("config/data/node_key.json", []byte{})
	if err != nil {
		panic(err)
	}

	nodeKey, err := key.LoadOrGenNodeKey("config/data/node_key.json")
	if err != nil {
		panic(err)
	}

	datastore, err := store.NewDefaultInMemoryKVStore()
	if err != nil {
		panic(err)
	}

	p2pClient, err := p2p.NewClient(config.DefaultNodeConfig, "testapp", nodeKey, datastore, logger, nil)
	if err != nil {
		panic(err)
	}

	// Add subcommands to the root command
	rootCmd.AddCommand(
		rollcmd.NewDocsGenCmd(rootCmd, commands.AppName),
		rollcmd.NewRunNodeCmd(executor, sequencer, dac, keyProvider, nodeKey, p2pClient, datastore),
		rollcmd.VersionCmd,
		rollcmd.InitCmd,
	)

	if err := rootCmd.Execute(); err != nil {
		// Print to stderr and exit with error
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
