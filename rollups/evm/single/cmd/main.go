package main

import (
	"fmt"
	"os"

	rollkitconfig "github.com/rollkit/rollkit/pkg/config"
	"github.com/spf13/cobra"

	"github.com/rollkit/rollkit/pkg/cmd"
)

func main() {
	// Initiate the root command
	rootCmd := cobra.Command{
		Use:   "evm-single",
		Short: "Rollkit with EVM; single sequencer",
	}

	rollkitconfig.AddGlobalFlags(&rootCmd, "evm-single")

	// Add subcommands to the root command
	initCmd := cmd.InitCmd
	rollkitconfig.AddFlags(initCmd)

	rootCmd.AddCommand(
		initCmd,
		RunCmd,
	)

	if err := rootCmd.Execute(); err != nil {
		// Print to stderr and exit with error
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
