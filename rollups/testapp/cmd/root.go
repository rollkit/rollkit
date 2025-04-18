package cmd

import (
	"github.com/spf13/cobra"

	rollkitconfig "github.com/rollkit/rollkit/pkg/config"
)

const (
	// AppName is the name of the application, the name of the command, and the name of the home directory.
	AppName = "testapp"
)

const (
	flagKVEndpoint = "kv-endpoint"
)

func init() {
	rollkitconfig.AddGlobalFlags(RootCmd, AppName)
	rollkitconfig.AddFlags(RunCmd)
	// Add the KV endpoint flag specifically to the RunCmd
	RunCmd.Flags().String(flagKVEndpoint, "localhost:8080", "Address and port for the KV executor HTTP server")
}

// RootCmd is the root command for Rollkit
var RootCmd = &cobra.Command{
	Use:   AppName,
	Short: "The first sovereign rollup framework that allows you to launch a sovereign, customizable blockchain as easily as a smart contract.",
	Long: `
Rollkit is the first sovereign rollup framework that allows you to launch a sovereign, customizable blockchain as easily as a smart contract.
If the --home flag is not specified, the rollkit command will create a folder "~/.testapp" where it will store node keys, config, and data.`,
}
