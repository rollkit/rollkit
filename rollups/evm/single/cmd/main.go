package main

import (
	"fmt"
	"os"

	cmd "github.com/rollkit/rollkit/rollups/evm/single/cmd/commands"
)

func main() {
	// Initiate the root command
	rootCmd := cmd.RootCmd

	// Add subcommands to the root command
	rootCmd.AddCommand(
		cmd.NewRunNodeCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		// Print to stderr and exit with error
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
