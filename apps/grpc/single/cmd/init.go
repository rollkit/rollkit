package cmd

import (
	"github.com/spf13/cobra"

	rollcmd "github.com/rollkit/rollkit/pkg/cmd"
	"github.com/rollkit/rollkit/pkg/config"
)

// InitCmd returns the init command for initializing the gRPC single sequencer node
func InitCmd() *cobra.Command {
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize rollkit configuration files",
		Long: `Initialize configuration files for a Rollkit node with gRPC execution client.
This will create the necessary configuration structure in the specified root directory.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Parse config from command flags
			parsedConfig, err := rollcmd.ParseConfig(cmd)
			if err != nil {
				return err
			}

			// Create configuration in the root directory
			return rollcmd.CreateConfiguration(parsedConfig.RootDir, parsedConfig.ConfigPath())
		},
	}

	// Add configuration flags
	config.AddFlags(initCmd)

	return initCmd
}