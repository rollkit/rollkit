package commands

import (
	"github.com/spf13/cobra"

	rollkitconfig "github.com/rollkit/rollkit/pkg/config"
)

const (
	AppName = "testapp"
)

func init() {
	rollkitconfig.AddBasicFlags(RootCmd, AppName)
}

// RootCmd is the root command for Rollkit
var RootCmd = &cobra.Command{
	Use:   AppName,
	Short: "Test Application",
	Long: `
Test Application is a simple application that allows you to test the Rollkit framework.
`,
}
