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

		// If home is not specified, use the current directory
		if homePath == "" {
			homePath, err = os.Getwd()
			if err != nil {
				return fmt.Errorf("error getting current directory: %w", err)
			}
		}

		configFilePath := filepath.Join(homePath, rollconf.RollkitConfigYaml)
		if _, err := os.Stat(configFilePath); err == nil {
			return fmt.Errorf("%s file already exists in the specified directory", rollconf.RollkitConfigYaml)
		}

		// try find main.go file under the current directory
		dirName, entrypoint := rollconf.FindEntrypoint()
		if entrypoint == "" {
			fmt.Println("Could not find a rollup main.go entrypoint under the current directory. Please put an entrypoint in the rollkit.yaml file manually.")
		} else {
			fmt.Printf("Found rollup entrypoint: %s, adding to rollkit.yaml\n", entrypoint)
		}

		// checking for default cosmos chain config directory
		chainConfigDir, ok := rollconf.FindConfigDir(dirName)
		if !ok {
			fmt.Printf("Could not find rollup config under %s. Please put the chain.config_dir in the rollkit.yaml file manually.\n", chainConfigDir)
		} else {
			fmt.Printf("Found rollup configuration under %s, adding to rollkit.yaml\n", chainConfigDir)
		}

		// Create a config with default values
		config := rollconf.DefaultNodeConfig

		// Update with the values we found
		config.ConfigDir = chainConfigDir

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
