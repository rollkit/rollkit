package main

import (
	"fmt"
	"os"

	rollcmd "github.com/rollkit/rollkit/pkg/cmd"
	rollkitconfig "github.com/rollkit/rollkit/pkg/config"
	cmds "github.com/rollkit/rollkit/rollups/testapp/cmd"
)

var (
	// GitSHA is set at build time
	GitSHA string

	// Version is set at build time
	Version string
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
		initCmd,
	)

	if err := rootCmd.Execute(); err != nil {
		// Print to stderr and exit with error
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
