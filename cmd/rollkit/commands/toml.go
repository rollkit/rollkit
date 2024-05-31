package commands

import (
	"fmt"
	"os"

	rollconf "github.com/rollkit/rollkit/config"
	"github.com/spf13/cobra"
)

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
		// we want to find a main.go file under the cmd directory and use that as the entrypoint
		// for the rollkit.toml file
		// also we want to read the directory of that main.go so we can check for configDir based on that

		// try find main.go file
		dirName, entrypoint := rollconf.FindEntrypoint()
		if entrypoint == "" {
			fmt.Println("Could not find a main.go file under the current directory. Please put an entrypoint in the rollkit.toml file manually.\n")
		}

		// checking ~/.{dirName} for configDir, which is default for cosmos-sdk chains
		chainConfigDir := rollconf.CheckConfigDir(dirName)
		if chainConfigDir == "" {
			fmt.Printf("Could not find a rollup config under %s. Please put the chain.config_dir in the rollkit.toml file manually.\n", chainConfigDir)
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
	},
}
