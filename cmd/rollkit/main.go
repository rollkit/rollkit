package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cometbft/cometbft/libs/cli"

	cmd "github.com/rollkit/rollkit/cmd/rollkit/commands"
	rollconf "github.com/rollkit/rollkit/config"
)

func main() {
	// In case there is a rollkit.toml file in the current dir or somewhere up the
	// directory tree - we want to intercept the command and execute it against an entrypoint
	// specified in the rollkit.toml file. In case of missing toml file or missing entrypoint key
	// or missing actual entrypoint file - the normal rootCmd command is executed.
	if err := cmd.InterceptCommand(
		rollconf.ReadToml,
		cmd.RunRollupEntrypoint,
	); err == nil {
		return
	}
	
	fmt.Fprintf(os.Stdout, "No rollkit.toml file found: %v\nStarting fresh rollup in .rollkit directory\n", err)

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
