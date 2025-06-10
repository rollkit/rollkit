package main

import (
	"fmt"
	"os"

	cmds "github.com/rollkit/rollkit/apps/testapp/cmd"
	rollcmd "github.com/rollkit/rollkit/pkg/cmd"
)

func main() {
	// Initiate the root command
	rootCmd := cmds.RootCmd
	initCmd := cmds.InitCmd()

	// Add subcommands to the root command
	rootCmd.AddCommand(
		cmds.RunCmd,
		rollcmd.VersionCmd,
		rollcmd.NetInfoCmd,
		rollcmd.StoreUnsafeCleanCmd,
		rollcmd.KeysCmd(),
		initCmd,
	)

	if err := rootCmd.Execute(); err != nil {
		// Print to stderr and exit with error
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
