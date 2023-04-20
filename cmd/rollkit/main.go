package main

import (
	"os"
	"path/filepath"

	"github.com/tendermint/tendermint/libs/cli"

	cmd "github.com/rollkit/rollkit/cmd/rollkit/commands"
)

func main() {
	rootCmd := cmd.RootCmd
	rootCmd.AddCommand(
		cmd.InitFilesCmd,
		cmd.VersionCmd,
	)

	rootCmd.AddCommand(cmd.NewRunNodeCmd())

	cmd := cli.PrepareBaseCmd(rootCmd, "RK", os.ExpandEnv(filepath.Join("$HOME", ".rollkit")))
	if err := cmd.Execute(); err != nil {
		panic(err)
	}
}
