package commands

import (
	"fmt"
	"os"

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
	Short: fmt.Sprintf("Initialize a new %s file", rollconf.RollkitToml),
	Long:  fmt.Sprintf("This command initializes a new %s file in the current directory.", rollconf.RollkitToml),
	Run: func(cmd *cobra.Command, args []string) {
		if _, err := os.Stat(rollconf.RollkitToml); err == nil {
			fmt.Printf("%s file already exists in the current directory.\n", rollconf.RollkitToml)
			os.Exit(1)
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

		config := rollconf.TomlConfig{
			Entrypoint: entrypoint,
			Chain: rollconf.ChainTomlConfig{
				ConfigDir: chainConfigDir,
			},
		}

		// marshal the config to a toml file in the current directory
		if err := rollconf.WriteTomlConfig(config); err != nil {
			fmt.Println("Error writing rollkit.toml file:", err)
			os.Exit(1)
		}

		fmt.Printf("Initialized %s file in the current directory.\n", rollconf.RollkitToml)
	},
}
