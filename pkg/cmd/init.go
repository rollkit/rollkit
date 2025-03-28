package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	rollconf "github.com/rollkit/rollkit/pkg/config"
)

// InitCmd initializes a new rollkit.yaml file in the current directory
var InitCmd = &cobra.Command{
	Use:   "init",
	Short: fmt.Sprintf("Initialize a new %s file", rollconf.RollkitConfigYaml),
	Long:  fmt.Sprintf("This command initializes a new %s file in the specified directory (or current directory if not specified).", rollconf.RollkitConfigYaml),
	RunE: func(cmd *cobra.Command, args []string) error {
		homePath, err := cmd.Flags().GetString(rollconf.FlagRootDir)
		if err != nil {
			return fmt.Errorf("error reading home flag: %w", err)
		}

		if homePath == "" {
			return fmt.Errorf("home path is required")
		}

		configFilePath := filepath.Join(homePath, rollconf.RollkitConfigYaml)
		if _, err := os.Stat(configFilePath); err == nil {
			return fmt.Errorf("%s file already exists in the specified directory", rollconf.RollkitConfigYaml)
		}

		// Create a config with default values
		config := rollconf.DefaultNodeConfig

		// Update with the values we found
		config.ConfigDir = homePath

		// Set the root directory to the specified home path
		config.RootDir = homePath

		// Make sure the home directory exists
		if err := os.MkdirAll(homePath, rollconf.DefaultDirPerm); err != nil {
			return fmt.Errorf("error creating directory %s: %w", homePath, err)
		}

		// Use writeYamlConfig instead of manual marshaling and file writing
		if err := rollconf.WriteYamlConfig(config); err != nil {
			return fmt.Errorf("error writing rollkit.yaml file: %w", err)
		}

		fmt.Printf("Initialized %s file in %s\n", rollconf.RollkitConfigYaml, homePath)
		return nil
	},
}
