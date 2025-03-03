package commands

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"cosmossdk.io/log"
	"github.com/spf13/cobra"
)

func init() {
	registerFlagsRootCmd(RootCmd)
}

const (
	DefaultLogLevel = "info"
)

// registerFlagsRootCmd registers the flags for the root command
func registerFlagsRootCmd(cmd *cobra.Command) {
	cmd.PersistentFlags().String("log_level", DefaultLogLevel, "set the log level; default is info. other options include debug, info, error, none")
}

// RootCmd is the root command for Rollkit
var RootCmd = &cobra.Command{
	Use:   "rollkit",
	Short: "The first sovereign rollup framework that allows you to launch a sovereign, customizable blockchain as easily as a smart contract.",
	Long: `
Rollkit is the first sovereign rollup framework that allows you to launch a sovereign, customizable blockchain as easily as a smart contract.
The rollkit-cli uses the environment variable "RKHOME" to point to a file path where the node keys, config, and data will be stored. 
If a path is not specified for RKHOME, the rollkit command will create a folder "~/.rollkit" where it will store said data.
`,
}

// fileExists checks if a file exists
func fileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return !os.IsNotExist(err)
}

// TrapSignal catches the SIGTERM/SIGINT and executes cb function. After that it exits
// with code 0.
func trapSignal(logger log.Logger, cb func()) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		for sig := range c {
			logger.Info("signal trapped", "msg", fmt.Sprintf("captured %v, exiting...", sig))
			if cb != nil {
				cb()
			}
			os.Exit(0)
		}
	}()
}
