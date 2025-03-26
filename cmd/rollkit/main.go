package main

import (
	"fmt"
	"os"

	cmd "github.com/rollkit/rollkit/cmd/rollkit/commands"
	rollconf "github.com/rollkit/rollkit/pkg/config"
)

func main() {
	// Initiate the root command
	rootCmd := cmd.RootCmd

	// Add subcommands to the root command
	rootCmd.AddCommand(
		cmd.DocsGenCmd,
		cmd.NewRunNodeCmd(cmd.CreateExecutor, cmd.AddNodeFlags),
		cmd.VersionCmd,
		cmd.InitCmd,
		cmd.RebuildCmd,
	)

	// Wrapper function for ReadYaml that doesn't take arguments
	readYamlWrapper := func() (rollconf.Config, error) {
		return rollconf.ReadYaml("")
	}

	// In case there is a rollkit.yaml file in the current dir or somewhere up the
	// directory tree - we want to intercept the command and execute it against an entrypoint
	// specified in the rollkit.yaml file. In case of missing yaml file or missing entrypoint key
	// or missing actual entrypoint file - the normal rootCmd command is executed.
	executed, err := cmd.InterceptCommand(
		rootCmd,
		readYamlWrapper,
		cmd.RunRollupEntrypoint,
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
