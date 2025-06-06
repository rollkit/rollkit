package cmd_test

import (
	"bytes"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/pkg/cmd"
	rollconf "github.com/rollkit/rollkit/pkg/config"
	"github.com/rollkit/rollkit/pkg/signer/file"
)

// setupRootCmd creates a new root command and adds the keys command to it.
// This is necessary because the keys commands rely on global flags defined on the root.
func setupRootCmd() *cobra.Command {
	root := &cobra.Command{Use: "rollkit-test"}
	// Prevent cobra from printing errors, so we can assert on them
	root.SilenceErrors = true
	// Prevent cobra from printing usage, so we can assert on output
	root.SilenceUsage = true

	rollconf.AddGlobalFlags(root, "")
	rollconf.AddFlags(root)

	root.AddCommand(cmd.KeysCmd())
	return root
}

func TestKeysCmd(t *testing.T) {
	root := setupRootCmd()
	root.SetArgs([]string{"keys", "--help"})
	err := root.Execute()
	require.NoError(t, err)
}

func TestExportKeyCmd(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	// 1. Setup: Create a temporary home directory and import a known key.
	// This gives us a predictable key to export and verify.
	homeDir := t.TempDir()
	keyPath := filepath.Join(homeDir, "config")
	passphrase := "test-password"

	privKey, _, err := crypto.GenerateKeyPair(crypto.Ed25519, 256)
	require.NoError(err)
	privKeyBytes, err := privKey.Raw()
	require.NoError(err)
	hexPrivKey := hex.EncodeToString(privKeyBytes)

	err = file.ImportPrivateKey(keyPath, privKeyBytes, []byte(passphrase))
	require.NoError(err)

	t.Run("successful export", func(t *testing.T) {
		root := setupRootCmd()
		outBuf := new(bytes.Buffer)
		errBuf := new(bytes.Buffer)
		root.SetOut(outBuf)
		root.SetErr(errBuf)

		args := []string{
			"keys", "export",
			"--" + rollconf.FlagRootDir, homeDir,
			"--" + rollconf.FlagSignerPassphrase, passphrase,
		}
		root.SetArgs(args)

		err := root.Execute()
		require.NoError(err)

		// Check stdout for the key
		exportedHexKey := strings.TrimSpace(outBuf.String())
		assert.Equal(hexPrivKey, exportedHexKey)

		// Check stderr for the warning
		assert.Contains(errBuf.String(), "WARNING: EXPORTING PRIVATE KEY")
	})

	t.Run("no passphrase", func(t *testing.T) {
		root := setupRootCmd()
		args := []string{
			"keys", "export",
			"--" + rollconf.FlagRootDir, homeDir,
		}
		root.SetArgs(args)

		err := root.Execute()
		require.Error(err)
		assert.Contains(err.Error(), "passphrase is required")
	})

	t.Run("wrong passphrase", func(t *testing.T) {
		root := setupRootCmd()
		args := []string{
			"keys", "export",
			"--" + rollconf.FlagRootDir, homeDir,
			"--" + rollconf.FlagSignerPassphrase, "wrong-password",
		}
		root.SetArgs(args)

		err := root.Execute()
		require.Error(err)
		assert.Contains(err.Error(), "wrong passphrase")
	})

	t.Run("key file not found", func(t *testing.T) {
		root := setupRootCmd()
		nonExistentHomeDir := t.TempDir()
		args := []string{
			"keys", "export",
			"--" + rollconf.FlagRootDir, nonExistentHomeDir,
			"--" + rollconf.FlagSignerPassphrase, passphrase,
		}
		root.SetArgs(args)

		err := root.Execute()
		require.Error(err)
		assert.Contains(t, err.Error(), "failed to read key file")
	})
}

func TestImportKeyCmd(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	// Setup: Generate a key to be used for importing.
	passphrase := "test-password"
	privKey, _, err := crypto.GenerateKeyPair(crypto.Ed25519, 256)
	require.NoError(err)
	privKeyBytes, err := privKey.Raw()
	require.NoError(err)
	hexPrivKey := hex.EncodeToString(privKeyBytes)

	t.Run("successful import", func(t *testing.T) {
		root := setupRootCmd()
		importDir := t.TempDir()
		outBuf := new(bytes.Buffer)
		root.SetOut(outBuf)

		args := []string{
			"keys", "import", hexPrivKey,
			"--" + rollconf.FlagRootDir, importDir,
			"--" + rollconf.FlagSignerPassphrase, passphrase,
		}
		root.SetArgs(args)

		err := root.Execute()
		require.NoError(err)

		// Check for success message
		assert.Contains(outBuf.String(), "Successfully imported key")

		// Verify file was created
		keyPath := filepath.Join(importDir, "config")
		_, err = os.Stat(filepath.Join(keyPath, "signer.json"))
		assert.NoError(err)

		// Verify we can load the key and it's the correct one
		signer, err := file.LoadFileSystemSigner(keyPath, []byte(passphrase))
		assert.NoError(err)
		loadedPubKey, err := signer.GetPublic()
		assert.NoError(err)
		assert.True(loadedPubKey.Equals(privKey.GetPublic()))
	})

	t.Run("no passphrase", func(t *testing.T) {
		root := setupRootCmd()
		importDir := t.TempDir()
		args := []string{
			"keys", "import", hexPrivKey,
			"--" + rollconf.FlagRootDir, importDir,
		}
		root.SetArgs(args)

		err := root.Execute()
		require.Error(err)
		assert.Contains(err.Error(), "passphrase is required")
	})

	t.Run("invalid hex key", func(t *testing.T) {
		root := setupRootCmd()
		importDir := t.TempDir()
		args := []string{
			"keys", "import", "not-a-hex-key",
			"--" + rollconf.FlagRootDir, importDir,
			"--" + rollconf.FlagSignerPassphrase, passphrase,
		}
		root.SetArgs(args)

		err := root.Execute()
		require.Error(err)
		assert.Contains(err.Error(), "failed to decode hex private key")
	})

	t.Run("no key argument", func(t *testing.T) {
		root := setupRootCmd()
		importDir := t.TempDir()
		args := []string{
			"keys", "import",
			"--" + rollconf.FlagRootDir, importDir,
			"--" + rollconf.FlagSignerPassphrase, passphrase,
		}
		root.SetArgs(args)

		err := root.Execute()
		require.Error(err)
		assert.Contains(err.Error(), "accepts 1 arg(s), received 0")
	})

	t.Run("key exists no force", func(t *testing.T) {
		root := setupRootCmd()
		importDir := t.TempDir()
		keyPath := filepath.Join(importDir, "config")
		// Create a key file first
		_, err := file.CreateFileSystemSigner(keyPath, []byte("old-password"))
		require.NoError(err)

		args := []string{
			"keys", "import", hexPrivKey,
			"--" + rollconf.FlagRootDir, importDir,
			"--" + rollconf.FlagSignerPassphrase, passphrase,
		}
		root.SetArgs(args)

		err = root.Execute()
		require.Error(err)
		assert.Contains(err.Error(), "key file already exists")
		assert.Contains(err.Error(), "Use --force to overwrite")
	})

	t.Run("key exists with force", func(t *testing.T) {
		root := setupRootCmd()
		importDir := t.TempDir()
		keyPath := filepath.Join(importDir, "config")
		outBuf := new(bytes.Buffer)
		root.SetOut(outBuf)

		// Create a key file first
		_, err := file.CreateFileSystemSigner(keyPath, []byte("old-password"))
		require.NoError(err)

		args := []string{
			"keys", "import", hexPrivKey,
			"--" + rollconf.FlagRootDir, importDir,
			"--" + rollconf.FlagSignerPassphrase, passphrase,
			"--force",
		}
		root.SetArgs(args)

		err = root.Execute()
		require.NoError(err)
		assert.Contains(outBuf.String(), "Successfully imported key")

		// Verify the new key is in place
		signer, err := file.LoadFileSystemSigner(keyPath, []byte(passphrase))
		require.NoError(err)
		pubKey, err := signer.GetPublic()
		require.NoError(err)

		originalPubKey := privKey.GetPublic()
		assert.True(pubKey.Equals(originalPubKey))
	})
}
