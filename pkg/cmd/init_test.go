package cmd_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rollkit/rollkit/pkg/cmd"
	rollconf "github.com/rollkit/rollkit/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test CreateSigner scenarios
func TestCreateSigner(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	// Case 1: File signer, Aggregator, No passphrase -> Error
	t.Run("FileSigner_Aggregator_NoPassphrase", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := &rollconf.Config{
			Signer: rollconf.SignerConfig{SignerType: "file"},
			Node:   rollconf.NodeConfig{Aggregator: true},
		}
		_, err := cmd.CreateSigner(cfg, tmpDir, "")
		require.Error(err)
		assert.Contains(err.Error(), "passphrase is required")
	})

	// Case 2: File signer, Aggregator, With passphrase -> Success
	t.Run("FileSigner_Aggregator_WithPassphrase", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := &rollconf.Config{
			Signer: rollconf.SignerConfig{SignerType: "file"},
			Node:   rollconf.NodeConfig{Aggregator: true},
		}
		addr, err := cmd.CreateSigner(cfg, tmpDir, "testpass")
		require.NoError(err)
		assert.NotNil(addr)
		assert.NotEmpty(addr)
		// Check if signer files were created
		signerDir := filepath.Join(tmpDir, "config")
		_, err = os.Stat(filepath.Join(signerDir, "signer.json"))
		assert.NoError(err, "signer file should exist")
	})

	// Case 3: Non-File signer, Aggregator -> Error (Remote signer not implemented)
	t.Run("RemoteSigner_Aggregator", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := &rollconf.Config{
			Signer: rollconf.SignerConfig{SignerType: "remote"},
			Node:   rollconf.NodeConfig{Aggregator: true},
		}
		_, err := cmd.CreateSigner(cfg, tmpDir, "")
		require.Error(err)
		assert.Contains(err.Error(), "remote signer not implemented")
	})

	// Case 4: Not Aggregator -> No-op (returns nil, nil)
	t.Run("NonAggregator", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := &rollconf.Config{
			Signer: rollconf.SignerConfig{SignerType: "file"}, // Signer type doesn't matter here
			Node:   rollconf.NodeConfig{Aggregator: false},
		}
		addr, err := cmd.CreateSigner(cfg, tmpDir, "testpass")
		require.NoError(err)
		assert.Nil(addr)
	})

	// Case 5: Error creating signer directory (simulate by making dir a file)
	t.Run("FileSigner_MkdirError", func(t *testing.T) {
		tmpDir := t.TempDir()
		// Create a file where the directory should be
		signerDir := filepath.Join(tmpDir, "config")
		f, err := os.Create(signerDir)
		require.NoError(err)
		f.Close()

		cfg := &rollconf.Config{
			Signer: rollconf.SignerConfig{SignerType: "file"},
			Node:   rollconf.NodeConfig{Aggregator: true},
		}
		_, err = cmd.CreateSigner(cfg, tmpDir, "testpass")
		require.Error(err)
		assert.Contains(err.Error(), "failed to create signer directory")
	})
}

// Test LoadOrGenNodeKey scenarios
func TestLoadOrGenNodeKey(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	// Case 1: Key doesn't exist -> Create key
	t.Run("GenerateKey", func(t *testing.T) {
		tmpDir := t.TempDir()
		err := cmd.LoadOrGenNodeKey(tmpDir)
		require.NoError(err)
		// Check if node key file was created
		nodeKeyPath := filepath.Join(tmpDir, "config", "node_key.json")
		_, err = os.Stat(nodeKeyPath)
		assert.NoError(err, "node key file should exist")
	})

	// Case 2: Key exists -> Load key (Idempotency check)
	t.Run("LoadExistingKey", func(t *testing.T) {
		tmpDir := t.TempDir()
		// First call creates the key
		err := cmd.LoadOrGenNodeKey(tmpDir)
		require.NoError(err)
		nodeKeyPath := filepath.Join(tmpDir, "config", "node_key.json")
		stat1, err := os.Stat(nodeKeyPath)
		require.NoError(err)

		// Second call should load the existing key without error
		err = cmd.LoadOrGenNodeKey(tmpDir)
		require.NoError(err)
		stat2, err := os.Stat(nodeKeyPath)
		require.NoError(err)

		// Ensure the file wasn't modified (optional check)
		assert.Equal(stat1.ModTime(), stat2.ModTime(), "ModTime should be the same")
		assert.Equal(stat1.Size(), stat2.Size(), "Size should be the same")
	})

	// Case 3: Error during key generation/loading (simulate by making dir a file)
	t.Run("LoadOrGenKeyError", func(t *testing.T) {
		tmpDir := t.TempDir()
		// Create a file where the config directory should be
		configDir := filepath.Join(tmpDir, "config")
		f, err := os.Create(configDir)
		require.NoError(err)
		f.Close()

		err = cmd.LoadOrGenNodeKey(tmpDir)
		require.Error(err)
		// The underlying error comes from os.MkdirAll in key.LoadOrGenNodeKey
		assert.Contains(err.Error(), "failed to create node key")
	})
}
