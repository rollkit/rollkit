package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	tmcfg "github.com/tendermint/tendermint/config"
	"github.com/tendermint/tendermint/libs/cli"
	tmflags "github.com/tendermint/tendermint/libs/cli/flags"
	"github.com/tendermint/tendermint/libs/log"
)

var (
	tendermintConfig = tmcfg.DefaultConfig()
	logger           = log.NewTMLogger(log.NewSyncWriter(os.Stdout))
)

func init() {
	registerFlagsRootCmd(RootCmd)
}

// Used "info" as is the default log level in Tendermint
// and Rollkit does not have a default log level
func registerFlagsRootCmd(cmd *cobra.Command) {
	cmd.PersistentFlags().String("log_level", tendermintConfig.LogLevel, "log level")
}

// ParseConfig retrieves the default environment configuration,
// sets up the Rollkit root and ensures that the root exists
func ParseConfig(cmd *cobra.Command) (*tmcfg.Config, error) {
	tmconf := tmcfg.DefaultConfig()
	err := viper.Unmarshal(tmconf)
	if err != nil {
		return nil, err
	}

	var home string
	if os.Getenv("RKHOME") != "" {
		home = os.Getenv("RKHOME")
	} else {
		home, err = cmd.Flags().GetString(cli.HomeFlag)
		if err != nil {
			return nil, err
		}
	}

	tmconf.RootDir = home
	tmcfg.EnsureRoot(tmconf.RootDir)
	if err := tmconf.ValidateBasic(); err != nil {
		return nil, fmt.Errorf("error in config file: %v", err)
	}
	return tmconf, nil
}

// RootCmd is the root command for Rollkit
var RootCmd = &cobra.Command{
	Use:   "rollkit",
	Short: "A modular framework for rollups, with an ABCI-compatible client interface.",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) (err error) {
		if cmd.Name() == VersionCmd.Name() {
			return nil
		}

		tendermintConfig, err = ParseConfig(cmd)
		if err != nil {
			return err
		}

		// if config.LogFormat == cfg.LogFormatJSON {
		// 	logger = log.NewTMJSONLogger(log.NewSyncWriter(os.Stdout))
		// }

		if tendermintConfig.LogFormat == tmcfg.LogFormatJSON {
			logger = log.NewTMJSONLogger(log.NewSyncWriter(os.Stdout))
		}

		logger, err = tmflags.ParseLogLevel(tendermintConfig.LogLevel, logger, tmcfg.DefaultLogLevel)
		if err != nil {
			return err
		}

		// logger, err = tmflags.ParseLogLevel(config.LogLevel, logger, cfg.DefaultLogLevel)
		// if err != nil {
		// 	return err
		// }

		if viper.GetBool(cli.TraceFlag) {
			logger = log.NewTracingLogger(logger)
		}

		logger = logger.With("module", "main")
		return nil
	},
}
