package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	rollconf "github.com/rollkit/rollkit/pkg/config"
	genesispkg "github.com/rollkit/rollkit/pkg/genesis"
	"github.com/rollkit/rollkit/pkg/p2p/key"
	"github.com/rollkit/rollkit/pkg/signer/file"
)

// ValidateHomePath checks if the home path is valid and not already initialized
func ValidateHomePath(homePath string) error {
	if homePath == "" {
		return fmt.Errorf("home path is required")
	}

	configFilePath := filepath.Join(homePath, rollconf.RollkitConfigYaml)
	if _, err := os.Stat(configFilePath); err == nil {
		return fmt.Errorf("%s file already exists in the specified directory", rollconf.RollkitConfigYaml)
	}

	return nil
}

// InitializeConfig creates and initializes the configuration with default values
func InitializeConfig(homePath string, aggregator bool) rollconf.Config {
	config := rollconf.DefaultNodeConfig
	config.ConfigDir = homePath
	config.RootDir = homePath
	config.Node.Aggregator = aggregator
	return config
}

// InitializeSigner sets up the signer configuration and creates necessary files
func InitializeSigner(config *rollconf.Config, homePath string, passphrase string) error {
	if config.Signer.SignerType == "file" && config.Node.Aggregator {
		if passphrase == "" {
			return fmt.Errorf("passphrase is required when using local file signer")
		}

		signerDir := filepath.Join(homePath, "config")
		if err := os.MkdirAll(signerDir, rollconf.DefaultDirPerm); err != nil {
			return fmt.Errorf("failed to create signer directory: %w", err)
		}

		config.Signer.SignerPath = filepath.Join(signerDir, "priv_key.json")

		_, err := file.NewFileSystemSigner(config.Signer.SignerPath, []byte(passphrase))
		if err != nil {
			return fmt.Errorf("failed to initialize signer: %w", err)
		}
	}
	return nil
}

// InitializeNodeKey creates the node key file
func InitializeNodeKey(homePath string) error {
	nodeKeyFile := filepath.Join(homePath, "config", "node_key.json")
	_, err := key.LoadOrGenNodeKey(nodeKeyFile)
	if err != nil {
		return fmt.Errorf("failed to create node key: %w", err)
	}
	return nil
}

// InitializeGenesis creates and saves a genesis file with the given app state
func InitializeGenesis(homePath string, chainID string, initialHeight uint64, appState []byte) error {
	// Create an empty genesis file
	genesisData := genesispkg.NewGenesis(
		chainID,
		initialHeight,
		time.Now(), // Current time as genesis DA start height
		genesispkg.GenesisExtraData{},
		json.RawMessage(appState), // App state from parameters
	)

	// Create the config directory if it doesn't exist
	configDir := filepath.Join(homePath, "config")
	if err := os.MkdirAll(configDir, rollconf.DefaultDirPerm); err != nil {
		return fmt.Errorf("error creating config directory: %w", err)
	}

	// Save the genesis file
	genesisPath := filepath.Join(configDir, "genesis.json")
	if err := genesispkg.SaveGenesis(genesisData, genesisPath); err != nil {
		return fmt.Errorf("error writing genesis file: %w", err)
	}

	return nil
}

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

		if err := ValidateHomePath(homePath); err != nil {
			return err
		}

		if err := os.MkdirAll(homePath, rollconf.DefaultDirPerm); err != nil {
			return fmt.Errorf("error creating directory %s: %w", homePath, err)
		}

		aggregator, err := cmd.Flags().GetBool(rollconf.FlagAggregator)
		if err != nil {
			return fmt.Errorf("error reading aggregator flag: %w", err)
		}

		config := InitializeConfig(homePath, aggregator)

		passphrase, err := cmd.Flags().GetString(rollconf.FlagSignerPassphrase)
		if err != nil {
			return fmt.Errorf("error reading passphrase flag: %w", err)
		}

		if err := InitializeSigner(&config, homePath, passphrase); err != nil {
			return err
		}

		if err := rollconf.WriteYamlConfig(config); err != nil {
			return fmt.Errorf("error writing rollkit.yaml file: %w", err)
		}

		if err := InitializeNodeKey(homePath); err != nil {
			return err
		}

		// Get chain ID or use default
		chainID, err := cmd.Flags().GetString(rollconf.FlagChainID)
		if err != nil {
			return fmt.Errorf("error reading chain ID flag: %w", err)
		}
		if chainID == "" {
			chainID = "rollkit-test"
		}

		// Initialize genesis with empty app state
		if err := InitializeGenesis(homePath, chainID, 1, []byte("{}")); err != nil {
			return fmt.Errorf("error initializing genesis file: %w", err)
		}

		fmt.Printf("Initialized %s file in %s\n", rollconf.RollkitConfigYaml, homePath)
		return nil
	},
}

func init() {
	InitFlags(InitCmd)
}

func SignerFlags(cmd *cobra.Command) {
	// Add passphrase flag
	cmd.Flags().String(rollconf.FlagSignerPassphrase, "", "Passphrase for encrypting the local signer key (required when using local file signer)")
	cmd.Flags().Bool(rollconf.FlagAggregator, false, "Run node in aggregator mode")
}

// InitFlags adds init command flags
func InitFlags(cmd *cobra.Command) {
	SignerFlags(cmd)
	cmd.Flags().String(rollconf.FlagChainID, "rollkit-test", "Chain ID for the genesis file")
}
