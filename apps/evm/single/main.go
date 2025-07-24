package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	rollcmd "github.com/evstack/ev-node/pkg/cmd"
	rollkitconfig "github.com/evstack/ev-node/pkg/config"

	"github.com/evstack/ev-node/apps/evm/single/cmd"
)

func main() {
	// Initiate the root command
	rootCmd := &cobra.Command{
		Use:   "evm-single",
		Short: "Rollkit with EVM; single sequencer",
	}

	rollkitconfig.AddGlobalFlags(rootCmd, "evm-single")

	rootCmd.AddCommand(
		cmd.InitCmd(),
		cmd.RunCmd,
		rollcmd.VersionCmd,
		rollcmd.NetInfoCmd,
		rollcmd.StoreUnsafeCleanCmd,
		rollcmd.KeysCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		// Print to stderr and exit with error
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
