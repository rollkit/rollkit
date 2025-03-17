package commands

import (
	"fmt"
	"os"

	"github.com/goccy/go-yaml"
	"github.com/spf13/cobra"

	rollconf "github.com/rollkit/rollkit/config"
)

// NewYamlCmd creates a new cobra command group for YAML file operations.
func NewYamlCmd() *cobra.Command {
	yamlCmd := &cobra.Command{
		Use:     "yaml",
		Short:   "YAML file operations",
		Long:    `This command group is used to interact with YAML files.`,
		Example: `  rollkit yaml init`,
	}

	yamlCmd.AddCommand(initYamlCmd)

	return yamlCmd
}

var initYamlCmd = &cobra.Command{
	Use:   "init",
	Short: fmt.Sprintf("Initialize a new %s file", rollconf.RollkitConfigYaml),
	Long:  fmt.Sprintf("This command initializes a new %s file in the current directory.", rollconf.RollkitConfigYaml),
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := os.Stat(rollconf.RollkitConfigYaml); err == nil {
			return fmt.Errorf("%s file already exists in the current directory", rollconf.RollkitConfigYaml)
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
		config.Entrypoint = entrypoint
		config.ConfigDir = chainConfigDir

		// Set the root directory to the current directory
		currentDir, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("error getting current directory: %w", err)
		}
		config.RootDir = currentDir

		// Marshal the config to YAML format
		data, err := yaml.Marshal(config)
		if err != nil {
			return fmt.Errorf("error marshaling YAML data: %w", err)
		}

		// Write the YAML data to the file
		if err := os.WriteFile(rollconf.RollkitConfigYaml, data, 0600); err != nil {
			return fmt.Errorf("error writing rollkit.yaml file: %w", err)
		}

		fmt.Printf("Initialized %s file in the current directory.\n", rollconf.RollkitConfigYaml)
		return nil
	},
}
