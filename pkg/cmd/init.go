package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	rollconf "github.com/rollkit/rollkit/pkg/config"
	"github.com/rollkit/rollkit/pkg/p2p/key"
	"github.com/rollkit/rollkit/pkg/signer/file"
)

// InitCmd initializes a new rollkit.yaml file in the current directory
var InitCmd = &cobra.Command{
	Use:   "init",
	Short: fmt.Sprintf("Initialize a new %s file", rollconf.RollkitConfigYaml),
	Long:  fmt.Sprintf("This command initializes a new %s file in the specified directory (or current directory if not specified).", rollconf.RollkitConfigYaml),
	RunE: func(cmd *cobra.Command, args []string) error {
		homePath, err := cmd.Flags().GetString(rollconf.FlagRootDir)
		if err != nil {
			return fmt.Errorf("error reading home flag: %w", err)
		}

		if homePath == "" {
			return fmt.Errorf("home path is required")
		}

		configFilePath := filepath.Join(homePath, rollconf.RollkitConfigYaml)
		if _, err := os.Stat(configFilePath); err == nil {
			return fmt.Errorf("%s file already exists in the specified directory", rollconf.RollkitConfigYaml)
		}

		// Create a config with default values
		config := rollconf.DefaultNodeConfig

		// Update with the values we found
		config.ConfigDir = homePath

		// Set the root directory to the specified home path
		config.RootDir = homePath

		// Make sure the home directory exists
		if err := os.MkdirAll(homePath, rollconf.DefaultDirPerm); err != nil {
			return fmt.Errorf("error creating directory %s: %w", homePath, err)
		}

		aggregator, err := cmd.Flags().GetBool(rollconf.FlagAggregator)
		if err != nil {
			return fmt.Errorf("error reading aggregator flag: %w", err)
		}

		if aggregator {
			config.Node.Aggregator = true
		}

		// If using local file signer, initialize the key
		if config.Signer.SignerType == "file" && aggregator {
			// Get passphrase if local signing is enabled
			passphrase, err := cmd.Flags().GetString(rollconf.FlagSignerPassphrase)
			if err != nil {
				return fmt.Errorf("error reading passphrase flag: %w", err)
			}

			if passphrase == "" {
				return fmt.Errorf("passphrase is required when using local file signer")
			}

			// Create signer directory if it doesn't exist
			signerDir := filepath.Join(homePath, "signer")
			if err := os.MkdirAll(signerDir, rollconf.DefaultDirPerm); err != nil {
				return fmt.Errorf("failed to create signer directory: %w", err)
			}

			// Set signer path
			config.Signer.SignerPath = filepath.Join(signerDir, "priv_key.json")

			// Initialize the signer
			_, err = file.NewFileSystemSigner(config.Signer.SignerPath, []byte(passphrase))
			if err != nil {
				return fmt.Errorf("failed to initialize signer: %w", err)
			}
		}

		// Use writeYamlConfig instead of manual marshaling and file writing
		if err := rollconf.WriteYamlConfig(config); err != nil {
			return fmt.Errorf("error writing rollkit.yaml file: %w", err)
		}

		nodeKeyFile := filepath.Join(homePath, "config", "node_key.json")
		_, err = key.LoadOrGenNodeKey(nodeKeyFile)
		if err != nil {
			return fmt.Errorf("failed to create node key: %w", err)
		}

		fmt.Printf("Initialized %s file in %s\n", rollconf.RollkitConfigYaml, homePath)
		return nil
	},
}

func init() {
	// Add passphrase flag
	InitCmd.Flags().String(rollconf.FlagSignerPassphrase, "", "Passphrase for encrypting the local signer key (required when using local file signer)")
	InitCmd.Flags().Bool(rollconf.FlagAggregator, false, "Run node in aggregator mode")
}
