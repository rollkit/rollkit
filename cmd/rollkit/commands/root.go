package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	tmcfg "github.com/cometbft/cometbft/config"
	"github.com/cometbft/cometbft/libs/cli"
	tmflags "github.com/cometbft/cometbft/libs/cli/flags"
	"github.com/cometbft/cometbft/libs/log"
)

func init() {
	registerFlagsRootCmd(RootCmd)
}

// registerFlagsRootCmd registers the flags for the root command
func registerFlagsRootCmd(cmd *cobra.Command) {
	cmd.PersistentFlags().String("log_level", tmcfg.DefaultLogLevel, "set the log level; default is info. other options include debug, info, error, none")
}

// ParseConfig retrieves the default environment configuration, sets up the
// Rollkit root and ensures that the root exists
func ParseConfig(cmd *cobra.Command) (*tmcfg.Config, error) {
	// err := viper.Unmarshal(defaultCometConfig)
	// if err != nil {
	// 	return nil, err
	// }

	// Start with the default config
	config := defaultConfig

	// Set the root directory for the config to the home directory
	home := os.Getenv("RKHOME")
	if home == "" {
		var err error
		home, err = cmd.Flags().GetString(cli.HomeFlag)
		if err != nil {
			return nil, err
		}
	}
	config.RootDir = home

	// Validate the root directory
	tmcfg.EnsureRoot(config.RootDir)

	// Validate the config
	if err := config.ValidateBasic(); err != nil {
		return nil, fmt.Errorf("error in config file: %w", err)
	}
	return config, nil
}

// RootCmd is the root command for Rollkit
var RootCmd = &cobra.Command{
	Use:   "rollkit",
	Short: "A modular framework for rollups, with an ABCI-compatible client interface.",
	Long: `
Rollkit is a modular framework for rollups, with an ABCI-compatible client interface.
The rollkit-cli uses the environment variable "RKHOME" to point to a file path where the node keys, config, and data will be stored. 
If a path is not specified for RKHOME, the rollkit command will create a folder "~/.rollkit" where it will store said data.
`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) (err error) {
		// If the command is the version command, no need to parse the
		// config
		if cmd.Name() == VersionCmd.Name() {
			return nil
		}

		// Parse the config
		config, err := ParseConfig(cmd)
		if err != nil {
			return err
		}

		// Initialize the logger
		logger := defaultLogger

		// Update log format if the flag is set
		if config.LogFormat == tmcfg.LogFormatJSON {
			logger = log.NewTMJSONLogger(log.NewSyncWriter(os.Stdout))
		}

		// Parse the log level
		logger, err = tmflags.ParseLogLevel(config.LogLevel, logger, tmcfg.DefaultLogLevel)
		if err != nil {
			return err
		}

		// Add tracing to the logger if the flag is set
		if viper.GetBool(cli.TraceFlag) {
			logger = log.NewTracingLogger(logger)
		}

		logger = logger.With("module", "main")
		return nil
	},
}
