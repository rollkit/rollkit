package file

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileSystemSigner(t *testing.T) {
	// Create a temporary directory for the test
	tempDir := t.TempDir()
	keyPath := filepath.Join(tempDir, "secure_keys.json")
	passphrase := []byte("test-passphrase-123")

	// Create a new signer (this will generate and save keys)
	signer, err := CreateFileSystemSigner(keyPath, passphrase)
	require.NoError(t, err)
	require.NotNil(t, signer)

	// Verify the key file was created
	_, err = os.Stat(keyPath)
	require.NoError(t, err)

	// Get the public key
	pubKey, err := signer.GetPublic()
	require.NoError(t, err)
	require.NotNil(t, pubKey)

	// Sign a message
	message := []byte("Hello, world!")
	signature, err := signer.Sign(message)
	require.NoError(t, err)
	require.NotNil(t, signature)

	// Verify the signature
	valid, err := pubKey.Verify(message, signature)
	require.NoError(t, err)
	assert.True(t, valid)

	passphrase = []byte("test-passphrase-123")
	// Create a new signer instance (this will load the existing keys)
	signer2, err := LoadFileSystemSigner(keyPath, passphrase)
	require.NoError(t, err)
	require.NotNil(t, signer2)

	// Get the public key from the second signer
	pubKey2, err := signer2.GetPublic()
	require.NoError(t, err)
	require.NotNil(t, pubKey2)

	// Verify that both signers have the same public key
	pubKeyBytes1, err := pubKey.Raw()
	require.NoError(t, err)
	pubKeyBytes2, err := pubKey2.Raw()
	require.NoError(t, err)
	assert.Equal(t, pubKeyBytes1, pubKeyBytes2)

	// Test with wrong passphrase
	wrongPassphrase := []byte("wrong-passphrase")
	_, err = LoadFileSystemSigner(keyPath, wrongPassphrase)
	assert.Error(t, err)
}

// Example demonstrates how to use the FileSystemSigner
func Example() {
	// In a real application, you would use a secure passphrase
	passphrase := []byte("your-secure-passphrase")
	keyPath := "/path/to/secure/keys.json"

	// Create or load a signer
	signer, err := CreateFileSystemSigner(keyPath, passphrase)
	if err != nil {
		panic(err)
	}

	// Sign a message
	message := []byte("Message to sign")
	signature, err := signer.Sign(message)
	if err != nil {
		panic(err)
	}

	// Get the public key
	pubKey, err := signer.GetPublic()
	if err != nil {
		panic(err)
	}

	// Verify the signature (typically done by another party)
	valid, err := pubKey.Verify(message, signature)
	if err != nil {
		panic(err)
	}

	if valid {
		// Signature is valid
	} else {
		// Signature is invalid
	}
}
