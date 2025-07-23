package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	rollcmd "github.com/rollkit/rollkit/pkg/cmd"
	rollconf "github.com/rollkit/rollkit/pkg/config"
	rollgenesis "github.com/rollkit/rollkit/pkg/genesis"
)

// InitCmd returns the init command for initializing the gRPC single sequencer node
func InitCmd() *cobra.Command {
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize rollkit configuration files",
		Long: `Initialize configuration files for a Rollkit node with gRPC execution client.
This will create the necessary configuration structure in the specified root directory.`,
		Args: cobra.NoArgs,
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
				chainID = "grpc-test-chain"
			}

			// Initialize genesis without app state
			err = rollgenesis.CreateGenesis(homePath, chainID, 1, proposerAddress)
			genesisPath := rollgenesis.GenesisPath(homePath)
			if errors.Is(err, rollgenesis.ErrGenesisExists) {
				// check if existing genesis file is valid
				if genesis, err := rollgenesis.LoadGenesis(genesisPath); err == nil {
					if err := genesis.Validate(); err != nil {
						return fmt.Errorf("existing genesis file is invalid: %w", err)
					}
				} else {
					return fmt.Errorf("error loading existing genesis file: %w", err)
				}

				cmd.Printf("Genesis file already exists at %s, skipping creation.\n", genesisPath)
			} else if err != nil {
				return fmt.Errorf("error initializing genesis file: %w", err)
			}

			cmd.Printf("Successfully initialized config file at %s\n", cfg.ConfigPath())
			return nil
		},
	}

	// Add configuration flags
	rollconf.AddFlags(initCmd)

	return initCmd
}