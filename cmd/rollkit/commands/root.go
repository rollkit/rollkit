package commands

import (
	"github.com/spf13/cobra"

	rollkitconfig "github.com/rollkit/rollkit/pkg/config"
)

func init() {
	registerFlagsRootCmd(RootCmd)
}

// registerFlagsRootCmd registers the flags for the root command
func registerFlagsRootCmd(cmd *cobra.Command) {
	cmd.PersistentFlags().String("log_level", rollkitconfig.DefaultLogLevel, "set the log level; default is info. other options include debug, info, error, none")
	cmd.PersistentFlags().String("log_format", "plain", "set the log format; options include plain and json")
	cmd.PersistentFlags().Bool("trace", false, "print out full stack trace on errors")
	cmd.PersistentFlags().String(rollkitconfig.FlagRootDir, rollkitconfig.DefaultRootDir(), "root directory for Rollkit")
}

// RootCmd is the root command for Rollkit
var RootCmd = &cobra.Command{
	Use:   "rollkit",
	Short: "The first sovereign rollup framework that allows you to launch a sovereign, customizable blockchain as easily as a smart contract.",
	Long: `
Rollkit is the first sovereign rollup framework that allows you to launch a sovereign, customizable blockchain as easily as a smart contract.
If the --home flag is not specified, the rollkit command will create a folder "~/.rollkit" where it will store node keys, config, and data.
`,
}
