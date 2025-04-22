package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	rollcmd "github.com/rollkit/rollkit/pkg/cmd"
	rollconf "github.com/rollkit/rollkit/pkg/config"
)

// InitCmd initializes a new rollkit.yaml file in the current directory
func InitCmd() *cobra.Command {
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize rollkit config",
		Long:  fmt.Sprintf("This command initializes a new %s file in the specified directory (or current directory if not specified).", rollconf.ConfigName),
		RunE: func(cmd *cobra.Command, args []string) error {
			homePath, err := cmd.Flags().GetString(rollconf.FlagRootDir)
			if err != nil {
				return fmt.Errorf("error reading home flag: %w", err)
			}

			aggregator, err := cmd.Flags().GetBool(rollconf.FlagAggregator)
			if err != nil {
				return fmt.Errorf("error reading aggregator flag: %w", err)
			}

			// ignore error, as we are creating a new config
			// we use load in order to parse all the flags
			cfg, _ := rollconf.Load(cmd)
			cfg.Node.Aggregator = aggregator
			if err := cfg.Validate(); err != nil {
				return fmt.Errorf("error validating config: %w", err)
			}

			passphrase, err := cmd.Flags().GetString(rollconf.FlagSignerPassphrase)
			if err != nil {
				return fmt.Errorf("error reading passphrase flag: %w", err)
			}

			proposerAddress, err := rollcmd.CreateSigner(&cfg, homePath, passphrase)
			if err != nil {
				return err
			}

			if err := cfg.SaveAsYaml(); err != nil {
				return fmt.Errorf("error writing rollkit.yaml file: %w", err)
			}

			if err := rollcmd.LoadOrGenNodeKey(homePath); err != nil {
				return err
			}

			// get chain ID or use default
			chainID, _ := cmd.Flags().GetString(rollconf.FlagChainID)
			if chainID == "" {
				chainID = "rollkit-test"
			}

			// Initialize genesis with empty app state
			if err := rollcmd.InitializeGenesis(homePath, chainID, 1, proposerAddress, []byte("{}")); err != nil {
				return fmt.Errorf("error initializing genesis file: %w", err)
			}

			cmd.Printf("Successfully initialized config file at %s\n", cfg.ConfigPath())
			return nil
		},
	}

	// Add flags to the command
	rollconf.AddFlags(initCmd)

	return initCmd
}
