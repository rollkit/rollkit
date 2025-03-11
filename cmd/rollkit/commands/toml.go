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
		v.SetConfigName(rollconf.RollkitToml[:len(rollconf.RollkitToml)-5]) // Remove the .toml extension
		v.SetConfigType("toml")
		v.AddConfigPath(currentDir)

		// Create a map with the configuration structure
		// We need to handle time.Duration values specially to ensure they are serialized as human-readable strings
		rollkitConfig := map[string]interface{}{
			"aggregator":          config.Rollkit.Aggregator,
			"light":               config.Rollkit.Light,
			"da_address":          config.Rollkit.DAAddress,
			"da_auth_token":       config.Rollkit.DAAuthToken,
			"da_gas_price":        config.Rollkit.DAGasPrice,
			"da_gas_multiplier":   config.Rollkit.DAGasMultiplier,
			"da_submit_options":   config.Rollkit.DASubmitOptions,
			"da_namespace":        config.Rollkit.DANamespace,
			"block_time":          config.Rollkit.BlockTime.String(),
			"da_block_time":       config.Rollkit.DABlockTime.String(),
			"da_start_height":     config.Rollkit.DAStartHeight,
			"da_mempool_ttl":      config.Rollkit.DAMempoolTTL,
			"max_pending_blocks":  config.Rollkit.MaxPendingBlocks,
			"lazy_aggregator":     config.Rollkit.LazyAggregator,
			"lazy_block_time":     config.Rollkit.LazyBlockTime.String(),
			"trusted_hash":        config.Rollkit.TrustedHash,
			"sequencer_address":   config.Rollkit.SequencerAddress,
			"sequencer_rollup_id": config.Rollkit.SequencerRollupID,
			"executor_address":    config.Rollkit.ExecutorAddress,
		}

		// Set the configuration values in Viper
		v.Set("entrypoint", config.Entrypoint)
		v.Set("chain", config.Chain)
		v.Set("rollkit", rollkitConfig)
		v.Set("root_dir", config.RootDir)

		// Write the configuration file
		if err := v.WriteConfigAs(rollconf.RollkitToml); err != nil {
			fmt.Println("Error writing rollkit.toml file:", err)
			os.Exit(1)
		}

		fmt.Printf("Initialized %s file in the current directory.\n", rollconf.RollkitToml)
	},
}
