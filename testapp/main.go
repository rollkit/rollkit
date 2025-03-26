package main

import (
	"context"
	"fmt"
	"os"

	rollcmd "github.com/rollkit/rollkit/cmd/rollkit/commands"
	rollconf "github.com/rollkit/rollkit/pkg/config"
	testExecutor "github.com/rollkit/rollkit/test/executors/kv"
	commands "github.com/rollkit/rollkit/testapp/commands"
)

func main() {
	// Initiate the root command
	rootCmd := commands.RootCmd

	// Create context for the executor
	ctx := context.Background()

	// Add subcommands to the root command
	rootCmd.AddCommand(
		rollcmd.DocsGenCmd,
		rollcmd.NewRunNodeCmd(testExecutor.CreateDirectKVExecutor(ctx)),
		rollcmd.VersionCmd,
		rollcmd.InitCmd,
		rollcmd.RebuildCmd,
	)

	// Wrapper function for ReadYaml that doesn't take arguments
	readYamlWrapper := func() (rollconf.Config, error) {
		return rollconf.ReadYaml("")
	}

	// In case there is a rollkit.yaml file in the current dir or somewhere up the
	// directory tree - we want to intercept the command and execute it against an entrypoint
	// specified in the rollkit.yaml file. In case of missing yaml file or missing entrypoint key
	// or missing actual entrypoint file - the normal rootCmd command is executed.
	executed, err := rollcmd.InterceptCommand(
		rootCmd,
		readYamlWrapper,
		rollcmd.RunRollupEntrypoint,
	)
	if err != nil {
		fmt.Println("Error intercepting command: ", err)
	}
	if executed {
		return
	}

	if err := rootCmd.Execute(); err != nil {
		// Print to stderr and exit with error
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
