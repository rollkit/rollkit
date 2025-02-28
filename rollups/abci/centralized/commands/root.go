package commands

import (
	"github.com/spf13/cobra"
)

func init() {
	registerFlagsRootCmd(RootCmd)
}

// registerFlagsRootCmd registers the flags for the root command
func registerFlagsRootCmd(cmd *cobra.Command) {
	cmd.PersistentFlags().String("log_level", "info", "set the log level; default is info. other options include debug, info, error, none")
}

// RootCmd is the root command for Rollkit
var RootCmd = &cobra.Command{
	Use:   "centra",
	Short: "The first sovereign rollup framework that allows you to launch a sovereign, customizable blockchain as easily as a smart contract.",
	Long: `
Rollkit is the first sovereign rollup framework that allows you to launch a sovereign, customizable blockchain as easily as a smart contract.
The rollkit-cli uses the environment variable "RKHOME" to point to a file path where the node keys, config, and data will be stored. 
If a path is not specified for RKHOME, the rollkit command will create a folder "~/.rollkit" where it will store said data.
`,
}
