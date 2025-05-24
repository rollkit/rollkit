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
	RunCmd.Flags().String(flagKVEndpoint, "", "Address and port for the KV executor HTTP server")
}

// RootCmd is the root command for Rollkit
var RootCmd = &cobra.Command{
	Use:   AppName,
	Short: "Testapp is a test application for Rollkit, it consists of a simple key-value store and a single sequencer.",
}
