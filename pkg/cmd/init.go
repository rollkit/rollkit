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
	"github.com/rollkit/rollkit/pkg/hash"
	"github.com/rollkit/rollkit/pkg/p2p/key"
	"github.com/rollkit/rollkit/pkg/signer/file"
)

// InitializeConfig creates and initializes the configuration with default values
func InitializeConfig(homePath string, aggregator bool) rollconf.Config {
	config := rollconf.DefaultConfig
	config.RootDir = homePath
	config.Node.Aggregator = aggregator
	return config
}

// InitializeSigner sets up the signer configuration and creates necessary files
func InitializeSigner(config *rollconf.Config, homePath string, passphrase string) ([]byte, error) {
	if config.Signer.SignerType == "file" && config.Node.Aggregator {
		if passphrase == "" {
			return nil, fmt.Errorf("passphrase is required when using local file signer")
		}

		signerDir := filepath.Join(homePath, "config")
		if err := os.MkdirAll(signerDir, 0o750); err != nil {
			return nil, fmt.Errorf("failed to create signer directory: %w", err)
		}

		config.Signer.SignerPath = signerDir

		signer, err := file.CreateFileSystemSigner(config.Signer.SignerPath, []byte(passphrase))
		if err != nil {
			return nil, fmt.Errorf("failed to initialize signer: %w", err)
		}

		pubKey, err := signer.GetPublic()
		if err != nil {
			return nil, fmt.Errorf("failed to get public key: %w", err)
		}

		bz, err := pubKey.Raw()
		if err != nil {
			return nil, fmt.Errorf("failed to get public key raw bytes: %w", err)
		}

		proposerAddress := hash.SumTruncated(bz)

		return proposerAddress, nil
	} else if config.Signer.SignerType != "file" && config.Node.Aggregator {
		return nil, fmt.Errorf("remote signer not implemented for aggregator nodes, use local signer instead")
	}
	return nil, nil
}

// InitializeNodeKey creates the node key file
func InitializeNodeKey(homePath string) error {
	nodeKeyFile := filepath.Join(homePath, "config")
	_, err := key.LoadOrGenNodeKey(nodeKeyFile)
	if err != nil {
		return fmt.Errorf("failed to create node key: %w", err)
	}
	return nil
}

// InitializeGenesis creates and saves a genesis file with the given app state
func InitializeGenesis(homePath string, chainID string, initialHeight uint64, proposerAddress, appState []byte) error {
	// Create the config directory path first
	configDir := filepath.Join(homePath, "config")
	// Determine the genesis file path
	genesisPath := filepath.Join(configDir, "genesis.json")

	// Check if the genesis file already exists
	if _, err := os.Stat(genesisPath); err == nil {
		// File exists, return successfully without overwriting
		fmt.Printf("Genesis file already exists: %s\n", genesisPath)
		return nil
	} else if !os.IsNotExist(err) {
		// An error other than "not exist" occurred (e.g., permissions)
		return fmt.Errorf("failed to check for existing genesis file at %s: %w", genesisPath, err)
	}
	// If os.IsNotExist(err) is true, the file doesn't exist, so we proceed.

	// Create the config directory if it doesn't exist (needed before saving genesis)
	if err := os.MkdirAll(configDir, 0o750); err != nil {
		return fmt.Errorf("error creating config directory: %w", err)
	}

	// Create the genesis data struct since the file doesn't exist
	genesisData := genesispkg.NewGenesis(
		chainID,
		initialHeight,
		time.Now(),                // Current time as genesis DA start height
		proposerAddress,           // Proposer address
		json.RawMessage(appState), // App state from parameters
	)

	// Save the new genesis file
	if err := genesispkg.SaveGenesis(genesisData, genesisPath); err != nil {
		return fmt.Errorf("error writing genesis file: %w", err)
	}

	fmt.Printf("Initialized new genesis file: %s\n", genesisPath)
	return nil
}

// InitCmd initializes a new rollkit.yaml file in the current directory
var InitCmd = &cobra.Command{
	Use:   "init",
	Short: fmt.Sprintf("Initialize rollkit config"),
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

		cfg := InitializeConfig(homePath, aggregator)
		if err := cfg.Validate(); err != nil {
			return fmt.Errorf("error validating config: %w", err)
		}

		passphrase, err := cmd.Flags().GetString(rollconf.FlagSignerPassphrase)
		if err != nil {
			return fmt.Errorf("error reading passphrase flag: %w", err)
		}

		proposerAddress, err := InitializeSigner(&cfg, homePath, passphrase)
		if err != nil {
			return err
		}

		if err := cfg.SaveAsYaml(); err != nil {
			return fmt.Errorf("error writing rollkit.yaml file: %w", err)
		}

		if err := InitializeNodeKey(homePath); err != nil {
			return err
		}

		// get chain ID or use default
		chainID, _ := cmd.Flags().GetString(rollconf.FlagChainID)
		if chainID == "" {
			chainID = "rollkit-test"
		}

		// Initialize genesis with empty app state
		if err := InitializeGenesis(homePath, chainID, 1, proposerAddress, []byte("{}")); err != nil {
			return fmt.Errorf("error initializing genesis file: %w", err)
		}

		cmd.Printf("Successfully initialized config file at %s\n", cfg.ConfigPath())
		return nil
	},
}
