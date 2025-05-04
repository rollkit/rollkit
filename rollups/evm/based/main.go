package main

import (
	"context"
	"fmt"
	"os"

	rollcmd "github.com/rollkit/rollkit/pkg/cmd"
	"github.com/rollkit/rollkit/rollups/evm/based/cmd"
	"github.com/spf13/cobra"
)

const (
	AppName = "based"
)

var RootCmd = &cobra.Command{
	Use:   AppName,
	Short: "The first sovereign rollup framework that allows you to launch a sovereign, customizable blockchain as easily as a smart contract.",
	Long: `
Rollkit is the first sovereign rollup framework that allows you to launch a sovereign, customizable blockchain as easily as a smart contract.
If the --home flag is not specified, the rollkit command will create a folder "~/.testapp" where it will store node keys, config, and data.
`,
}

func main() {
	rootCmd := RootCmd
	ctx := context.Background()
	rootCmd.AddCommand(
		cmd.NewExtendedRunNodeCmd(ctx),
		rollcmd.VersionCmd,
		cmd.InitCmd(),
		rollcmd.NetInfoCmd,
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
