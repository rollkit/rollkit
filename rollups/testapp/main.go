package main

import (
	"fmt"
	"os"

	rollcmd "github.com/rollkit/rollkit/pkg/cmd"
	rollkitconfig "github.com/rollkit/rollkit/pkg/config"
	cmds "github.com/rollkit/rollkit/rollups/testapp/cmd"
)

func main() {
	// Initiate the root command
	rootCmd := cmds.RootCmd

	initCmd := rollcmd.InitCmd

	rollkitconfig.AddFlags(initCmd)

	// Add subcommands to the root command
	rootCmd.AddCommand(
		rollcmd.NewDocsGenCmd(rootCmd, cmds.AppName),
		cmds.RunCmd,
		rollcmd.VersionCmd,
		rollcmd.NodeInfoCmd,
		rollcmd.StoreUnsafeCleanCmd,
		initCmd,
	)

	if err := rootCmd.Execute(); err != nil {
		// Print to stderr and exit with error
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
