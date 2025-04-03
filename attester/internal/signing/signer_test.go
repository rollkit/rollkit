package signing

import (
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/rollkit/rollkit/attester/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to generate a new Ed25519 key pair and save the private key to a temp file.
func createTempEd25519KeyFile(t *testing.T) (privKey ed25519.PrivateKey, pubKey ed25519.PublicKey, keyFilePath string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err, "Failed to generate ed25519 key")

	dir := t.TempDir()
	tmpfn := filepath.Join(dir, "test_ed25519_key.priv")
	err = os.WriteFile(tmpfn, priv, 0600) // Write private key bytes
	require.NoError(t, err, "Failed to write temporary private key file")

	return priv, pub, tmpfn
}

// Helper function to create a dummy file with specific content.
func createDummyFileWithContent(t *testing.T, name string, content []byte) string {
	t.Helper()
	dir := t.TempDir()
	tmpfn := filepath.Join(dir, name)
	err := os.WriteFile(tmpfn, content, 0666)
	require.NoError(t, err, "Failed to write dummy file with content")
	return tmpfn
}

func TestLoadSigner_Ed25519_Success(t *testing.T) {
	_, _, keyPath := createTempEd25519KeyFile(t)

	signingCfg := config.SigningConfig{
		PrivateKeyPath: keyPath,
		Scheme:         "ed25519",
	}

	signer, err := LoadSigner(signingCfg)

	require.NoError(t, err, "LoadSigner should succeed for valid ed25519 config")
	require.NotNil(t, signer, "Signer should not be nil")
	assert.Equal(t, "ed25519", signer.Scheme(), "Signer scheme should be ed25519")

	// Check if it's the correct type (optional but good)
	_, ok := signer.(*Ed25519Signer)
	assert.True(t, ok, "Signer should be of type *Ed25519Signer")
}

func TestLoadSigner_KeyFileNotFound(t *testing.T) {
	signingCfg := config.SigningConfig{
		PrivateKeyPath: "/path/to/nonexistent/key.priv",
		Scheme:         "ed25519",
	}

	_, err := LoadSigner(signingCfg)
	require.Error(t, err, "LoadSigner should fail if key file does not exist")
	// Consider checking for os.ErrNotExist if more specific error checking is needed
}

func TestLoadSigner_InvalidKeySize(t *testing.T) {
	// Create a file with content that is not the correct ed25519 private key size
	invalidKeyBytes := []byte("too short")
	keyPath := createDummyFileWithContent(t, "invalid_key.priv", invalidKeyBytes)

	signingCfg := config.SigningConfig{
		PrivateKeyPath: keyPath,
		Scheme:         "ed25519",
	}

	_, err := LoadSigner(signingCfg)
	require.Error(t, err, "LoadSigner should fail for invalid key size")
	assert.Contains(t, err.Error(), "invalid private key size", "Error message should indicate invalid key size")
}

func TestLoadSigner_UnknownScheme(t *testing.T) {
	_, _, keyPath := createTempEd25519KeyFile(t) // Key content doesn't matter here

	signingCfg := config.SigningConfig{
		PrivateKeyPath: keyPath,
		Scheme:         "unknown-scheme",
	}

	_, err := LoadSigner(signingCfg)
	require.Error(t, err, "LoadSigner should fail for unknown scheme")
	assert.Contains(t, err.Error(), "unknown signing scheme", "Error message should indicate unknown scheme")
}

func TestLoadSigner_BLSScheme_Unimplemented(t *testing.T) {
	// Create a dummy file, content doesn't matter as BLS is checked first
	keyPath := createDummyFileWithContent(t, "bls_key.priv", []byte("dummy bls key data"))

	signingCfg := config.SigningConfig{
		PrivateKeyPath: keyPath,
		Scheme:         "bls",
	}

	_, err := LoadSigner(signingCfg)
	require.Error(t, err, "LoadSigner should fail for unimplemented BLS scheme")
	assert.Contains(t, err.Error(), "BLS signing scheme not yet implemented", "Error message should indicate BLS unimplemented")
}

func TestEd25519Signer_SignAndVerify(t *testing.T) {
	privKey, pubKey, _ := createTempEd25519KeyFile(t)

	signer, err := NewEd25519Signer(privKey)
	require.NoError(t, err, "NewEd25519Signer should succeed with valid key")

	message := []byte("message to be signed")
	signature, err := signer.Sign(message)
	require.NoError(t, err, "Sign should not produce an error")
	require.NotEmpty(t, signature, "Signature should not be empty")

	// Verify the signature using the original public key
	verified := ed25519.Verify(pubKey, message, signature)
	assert.True(t, verified, "Signature should be verifiable with the correct public key")

	// Verify using the signer's public key method
	verifiedWithSignerPub := ed25519.Verify(signer.PublicKey(), message, signature)
	assert.True(t, verifiedWithSignerPub, "Signature should be verifiable with the signer's public key")

	// Tamper with the message and verify failure
	incorrectMessage := []byte("incorrect message")
	verifiedIncorrect := ed25519.Verify(pubKey, incorrectMessage, signature)
	assert.False(t, verifiedIncorrect, "Signature verification should fail for incorrect message")
}

func TestEd25519Signer_PublicKey(t *testing.T) {
	privKey, pubKey, _ := createTempEd25519KeyFile(t)

	signer, err := NewEd25519Signer(privKey)
	require.NoError(t, err, "NewEd25519Signer should succeed")

	assert.Equal(t, pubKey, ed25519.PublicKey(signer.PublicKey()), "PublicKey() should return the correct public key bytes")
}

func TestEd25519Signer_Scheme(t *testing.T) {
	privKey, _, _ := createTempEd25519KeyFile(t)

	signer, err := NewEd25519Signer(privKey)
	require.NoError(t, err, "NewEd25519Signer should succeed")

	assert.Equal(t, "ed25519", signer.Scheme(), "Scheme() should return 'ed25519'")
}
