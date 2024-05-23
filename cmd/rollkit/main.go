package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cometbft/cometbft/libs/cli"

	cmd "github.com/rollkit/rollkit/cmd/rollkit/commands"
)

func main() {
	// In case there is a rollkit.toml file in the current or somewhere up the directory tree
	// we want to intercept the command and execute it against an entrypoint
	// specified in the toml file. In case of missing toml file or missing entrypoint -
	// the normal rootCmd command execution will be done.
	if err := cmd.InterceptCommand(); err == nil {
		return
	}

	// Initiate the root command
	rootCmd := cmd.RootCmd

	// Add subcommands to the root command
	rootCmd.AddCommand(
		cmd.DocsGenCmd,
		cmd.NewRunNodeCmd(),
		cmd.VersionCmd,
	)

	// Prepare the base command and execute
	executor := cli.PrepareBaseCmd(rootCmd, "RK", os.ExpandEnv(filepath.Join("$HOME", ".rollkit")))
	if err := executor.Execute(); err != nil {
		// Print to stderr and exit with error
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
