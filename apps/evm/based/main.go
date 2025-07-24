package main

import (
	"context"
	"fmt"
	"os"

	"github.com/evstack/ev-node/apps/evm/based/cmd"
	rollcmd "github.com/evstack/ev-node/pkg/cmd"
	"github.com/spf13/cobra"
)

const (
	AppName = "evm-based"
)

var RootCmd = &cobra.Command{
	Use:   AppName,
	Short: "evm-based is a based evm execution environment for rollkit, out of the box it works with reth",
}

func main() {
	rootCmd := RootCmd
	ctx := context.Background()
	rootCmd.AddCommand(
		cmd.NewExtendedRunNodeCmd(ctx),
		cmd.InitCmd(),

		rollcmd.VersionCmd,
		rollcmd.NetInfoCmd,
		rollcmd.StoreUnsafeCleanCmd,
		rollcmd.KeysCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
