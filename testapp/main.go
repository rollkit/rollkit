package main

import (
	"context"
	"fmt"
	"os"

	rollcmd "github.com/rollkit/rollkit/pkg/cmd"
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
		rollcmd.NewDocsGenCmd(rootCmd, commands.AppName),
		rollcmd.NewRunNodeCmd(testExecutor.CreateDirectKVExecutor(ctx)),
		rollcmd.VersionCmd,
		rollcmd.InitCmd,
	)

	if err := rootCmd.Execute(); err != nil {
		// Print to stderr and exit with error
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
