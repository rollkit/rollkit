package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	rollconf "github.com/rollkit/rollkit/pkg/config"
	genesispkg "github.com/rollkit/rollkit/pkg/genesis"
	"github.com/rollkit/rollkit/pkg/hash"
	"github.com/rollkit/rollkit/pkg/p2p/key"
	"github.com/rollkit/rollkit/pkg/signer/file"
)

// CreateSigner sets up the signer configuration and creates necessary files
func CreateSigner(config *rollconf.Config, homePath string, passphrase string) ([]byte, error) {
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

// LoadOrGenNodeKey creates the node key file.
func LoadOrGenNodeKey(homePath string) error {
	nodeKeyFile := filepath.Join(homePath, "config")

	_, err := key.LoadOrGenNodeKey(nodeKeyFile)
	if err != nil {
		return fmt.Errorf("failed to create node key: %w", err)
	}

	return nil
}

var ErrGenesisExists = fmt.Errorf("genesis file already exists")

// CreateGenesis creates and saves a genesis file with the given app state.
// If the genesis file already exists, it skips the creation and returns ErrGenesisExists.
// The genesis file is saved in the config directory of the specified home path.
func CreateGenesis(homePath string, chainID string, initialHeight uint64, proposerAddress, appState []byte) error {
	configDir := filepath.Join(homePath, "config")
	genesisPath := filepath.Join(configDir, "genesis.json")

	// Check if the genesis file already exists
	if _, err := os.Stat(genesisPath); err == nil {
		return ErrGenesisExists
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to check for existing genesis file at %s: %w", genesisPath, err)
	}

	// If the directory doesn't exist, create it
	if err := os.MkdirAll(configDir, 0o750); err != nil {
		return fmt.Errorf("error creating config directory: %w", err)
	}

	genesisData := genesispkg.NewGenesis(
		chainID,
		initialHeight,
		time.Now(),      // Current time as genesis DA start height
		proposerAddress, // Proposer address
	)

	if err := genesisData.Save(genesisPath); err != nil {
		return fmt.Errorf("error writing genesis file: %w", err)
	}

	return nil
}
