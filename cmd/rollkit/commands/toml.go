package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

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
	Run: func(cmd *cobra.Command, args []string) {
		if _, err := os.Stat(rollconf.RollkitConfigToml); err == nil {
			fmt.Printf("%s file already exists in the current directory.\n", rollconf.RollkitConfigToml)
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

		// Create a config with default values
		config := rollconf.DefaultNodeConfig

		// Update with the values we found
		config.Entrypoint = entrypoint
		config.Chain.ConfigDir = chainConfigDir

		// Set the root directory to the current directory
		currentDir, err := os.Getwd()
		if err != nil {
			fmt.Println("Error getting current directory:", err)
			os.Exit(1)
		}
		config.RootDir = currentDir

		// Create a new Viper instance to avoid conflicts with any existing configuration
		v := viper.New()

		// Configure Viper to use the structure directly
		v.SetConfigName(rollconf.ConfigBaseName)
		v.SetConfigType(rollconf.ConfigExtension)
		v.AddConfigPath(currentDir)

		// Create a map with the configuration structure
		// We need to handle time.Duration values specially to ensure they are serialized as human-readable strings
		rollkitConfig := map[string]interface{}{
			"aggregator":          config.Node.Aggregator,
			"light":               config.Node.Light,
			"block_time":          config.Node.BlockTime.String(),
			"max_pending_blocks":  config.Node.MaxPendingBlocks,
			"lazy_aggregator":     config.Node.LazyAggregator,
			"lazy_block_time":     config.Node.LazyBlockTime.String(),
			"trusted_hash":        config.Node.TrustedHash,
			"sequencer_address":   config.Node.SequencerAddress,
			"sequencer_rollup_id": config.Node.SequencerRollupID,
			"executor_address":    config.Node.ExecutorAddress,
		}

		daConfig := map[string]interface{}{
			"address":        config.DA.Address,
			"auth_token":     config.DA.AuthToken,
			"gas_price":      config.DA.GasPrice,
			"gas_multiplier": config.DA.GasMultiplier,
			"submit_options": config.DA.SubmitOptions,
			"namespace":      config.DA.Namespace,
			"block_time":     config.DA.BlockTime.String(),
			"start_height":   config.DA.StartHeight,
			"mempool_ttl":    config.DA.MempoolTTL,
		}

		// Set the configuration values in Viper
		v.Set("entrypoint", config.Entrypoint)
		v.Set("chain", config.Chain)
		v.Set("rollkit", rollkitConfig)
		v.Set("root_dir", config.RootDir)
		v.Set("da", daConfig)

		// Write the configuration file
		if err := v.WriteConfigAs(rollconf.RollkitConfigToml); err != nil {
			fmt.Println("Error writing rollkit.toml file:", err)
			os.Exit(1)
		}

		fmt.Printf("Initialized %s file in the current directory.\n", rollconf.RollkitConfigToml)
	},
}
