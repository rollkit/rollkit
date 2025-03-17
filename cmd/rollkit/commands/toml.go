package commands

import (
	"bytes"
	"fmt"
	"os"

	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"

	rollconf "github.com/rollkit/rollkit/config"
)

// NewTomlCmd creates a new cobra command group for TOML file operations.
func NewTomlCmd() *cobra.Command {
	TomlCmd := &cobra.Command{
		Use:     "toml",
		Short:   "TOML file operations",
		Long:    `This command group is used to interact with TOML files.`,
		Example: `  rollkit toml init`,
	}

	TomlCmd.AddCommand(initCmd)

	return TomlCmd
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: fmt.Sprintf("Initialize a new %s file", rollconf.RollkitConfigToml),
	Long:  fmt.Sprintf("This command initializes a new %s file in the current directory.", rollconf.RollkitConfigToml),
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := os.Stat(rollconf.RollkitConfigToml); err == nil {
			return fmt.Errorf("%s file already exists in the current directory", rollconf.RollkitConfigToml)
		}

		// try find main.go file under the current directory
		dirName, entrypoint := rollconf.FindEntrypoint()
		if entrypoint == "" {
			fmt.Println("Could not find a rollup main.go entrypoint under the current directory. Please put an entrypoint in the rollkit.toml file manually.")
		} else {
			fmt.Printf("Found rollup entrypoint: %s, adding to rollkit.toml\n", entrypoint)
		}

		// checking for default cosmos chain config directory
		chainConfigDir, ok := rollconf.FindConfigDir(dirName)
		if !ok {
			fmt.Printf("Could not find rollup config under %s. Please put the chain.config_dir in the rollkit.toml file manually.\n", chainConfigDir)
		} else {
			fmt.Printf("Found rollup configuration under %s, adding to rollkit.toml\n", chainConfigDir)
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

		// Marshal the config with comments to TOML format
		var buf bytes.Buffer
		encoder := toml.NewEncoder(&buf)
		err = encoder.Encode(config)
		if err != nil {
			return fmt.Errorf("error marshaling TOML data: %w", err)
		}

		// Write the TOML data to the file
		if err := os.WriteFile(rollconf.RollkitConfigToml, buf.Bytes(), 0600); err != nil {
			return fmt.Errorf("error writing rollkit.toml file: %w", err)
		}

		fmt.Printf("Initialized %s file in the current directory.\n", rollconf.RollkitConfigToml)
		return nil
	},
}
