package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	rollcmd "github.com/rollkit/rollkit/pkg/cmd"
	rollkitconfig "github.com/rollkit/rollkit/pkg/config"

	"github.com/rollkit/rollkit/apps/grpc/single/cmd"
)

func main() {
	// Initiate the root command
	rootCmd := &cobra.Command{
		Use:   "grpc-single",
		Short: "Rollkit with gRPC execution client; single sequencer",
		Long: `Run a Rollkit node with a gRPC-based execution client.
This allows you to connect to any execution layer that implements
the Rollkit execution gRPC interface.`,
	}

	rollkitconfig.AddGlobalFlags(rootCmd, "grpc-single")

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