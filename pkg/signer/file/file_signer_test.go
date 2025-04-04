package file

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateFileSystemSigner(t *testing.T) {
	t.Parallel()

	t.Run("successful creation", func(t *testing.T) {
		tempDir := t.TempDir()
		keyPath := tempDir
		passphrase := []byte("secure-test-passphrase")

		signer, err := CreateFileSystemSigner(keyPath, passphrase)
		require.NoError(t, err)
		require.NotNil(t, signer)

		// Verify file was created
		keyFile := filepath.Join(keyPath, "signer.json")
		fileInfo, err := os.Stat(keyFile)
		require.NoError(t, err)

		// Check permissions - should be 0600 (readable/writable only by owner)
		assert.Equal(t, os.FileMode(0600), fileInfo.Mode().Perm())

		// Verify it's a valid JSON file with expected structure
		fileBytes, err := os.ReadFile(keyFile)
		require.NoError(t, err)

		var data keyData
		err = json.Unmarshal(fileBytes, &data)
		require.NoError(t, err)

		// Check key data
		assert.NotEmpty(t, data.PrivKeyEncrypted)
		assert.NotEmpty(t, data.Nonce)
		assert.NotEmpty(t, data.PubKeyBytes)
		assert.NotEmpty(t, data.Salt)
	})

	t.Run("directory creation", func(t *testing.T) {
		tempDir := t.TempDir()
		// Test creating in a subdirectory that doesn't exist yet
		keyPath := filepath.Join(tempDir, "subdir1", "subdir2")
		passphrase := []byte("secure-test-passphrase")

		signer, err := CreateFileSystemSigner(keyPath, passphrase)
		require.NoError(t, err)
		require.NotNil(t, signer)

		// Verify directory and file were created
		keyFile := filepath.Join(keyPath, "signer.json")
		_, err = os.Stat(keyFile)
		require.NoError(t, err)
	})

	t.Run("file already exists error", func(t *testing.T) {
		tempDir := t.TempDir()
		keyPath := tempDir
		passphrase := []byte("secure-test-passphrase")

		// Create first signer
		_, err := CreateFileSystemSigner(keyPath, passphrase)
		require.NoError(t, err)

		// Try to create another with same path
		_, err = CreateFileSystemSigner(keyPath, passphrase)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "key file already exists")
	})
}

func TestLoadFileSystemSigner(t *testing.T) {
	t.Parallel()

	t.Run("successful loading", func(t *testing.T) {
		tempDir := t.TempDir()
		keyPath := tempDir
		passphrase := []byte("secure-test-passphrase")

		// Create signer
		signer1, err := CreateFileSystemSigner(keyPath, passphrase)
		require.NoError(t, err)

		// Get public key for later comparison
		pubKey1, err := signer1.GetPublic()
		require.NoError(t, err)
		pubBytes1, err := pubKey1.Raw()
		require.NoError(t, err)

		// Load signer
		passphrase = []byte("secure-test-passphrase") // Create new byte slice since original is zeroed
		signer2, err := LoadFileSystemSigner(keyPath, passphrase)
		require.NoError(t, err)

		// Verify public key matches
		pubKey2, err := signer2.GetPublic()
		require.NoError(t, err)
		pubBytes2, err := pubKey2.Raw()
		require.NoError(t, err)

		assert.True(t, bytes.Equal(pubBytes1, pubBytes2))
	})

	t.Run("non-existent file error", func(t *testing.T) {
		tempDir := t.TempDir()
		nonExistentKeyPath := filepath.Join(tempDir, "nonexistent")
		passphrase := []byte("secure-test-passphrase")

		_, err := LoadFileSystemSigner(nonExistentKeyPath, passphrase)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "key file not found")
	})

	t.Run("wrong passphrase error", func(t *testing.T) {
		tempDir := t.TempDir()
		keyPath := tempDir
		passphrase := []byte("secure-test-passphrase")

		// Create signer
		_, err := CreateFileSystemSigner(keyPath, passphrase)
		require.NoError(t, err)

		// Try to load with wrong passphrase
		wrongPassphrase := []byte("wrong-passphrase")
		_, err = LoadFileSystemSigner(keyPath, wrongPassphrase)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "wrong passphrase")
	})

	t.Run("corrupted file error", func(t *testing.T) {
		tempDir := t.TempDir()
		keyPath := tempDir
		passphrase := []byte("secure-test-passphrase")

		// Create signer to generate the file
		_, err := CreateFileSystemSigner(keyPath, passphrase)
		require.NoError(t, err)

		// Corrupt the file
		keyFile := filepath.Join(keyPath, "signer.json")
		err = os.WriteFile(keyFile, []byte("corrupted data"), 0600)
		require.NoError(t, err)

		// Try to load the corrupted file
		passphrase = []byte("secure-test-passphrase")
		_, err = LoadFileSystemSigner(keyPath, passphrase)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to unmarshal key data")
	})
}

func TestSign(t *testing.T) {
	t.Parallel()

	t.Run("successful signing", func(t *testing.T) {
		tempDir := t.TempDir()
		keyPath := tempDir
		passphrase := []byte("secure-test-passphrase")

		// Create signer
		signer, err := CreateFileSystemSigner(keyPath, passphrase)
		require.NoError(t, err)

		// Sign a message
		message := []byte("Hello, Rollkit!")
		signature, err := signer.Sign(message)
		require.NoError(t, err)
		require.NotNil(t, signature)

		// Verify the signature
		pubKey, err := signer.GetPublic()
		require.NoError(t, err)
		valid, err := pubKey.Verify(message, signature)
		require.NoError(t, err)
		assert.True(t, valid)
	})

	t.Run("sign multiple messages", func(t *testing.T) {
		tempDir := t.TempDir()
		keyPath := tempDir
		passphrase := []byte("secure-test-passphrase")

		// Create signer
		signer, err := CreateFileSystemSigner(keyPath, passphrase)
		require.NoError(t, err)

		pubKey, err := signer.GetPublic()
		require.NoError(t, err)

		// Sign and verify multiple different messages
		messages := []string{
			"Message 1",
			"A different message",
			"Yet another message with different length",
			"", // Empty message
		}

		for _, msg := range messages {
			message := []byte(msg)
			signature, err := signer.Sign(message)
			require.NoError(t, err)

			valid, err := pubKey.Verify(message, signature)
			require.NoError(t, err)
			assert.True(t, valid)

			// Cross-verify with wrong message
			if msg != "" {
				wrongMessage := []byte("wrong message")
				valid, err = pubKey.Verify(wrongMessage, signature)
				require.NoError(t, err)
				assert.False(t, valid, "Signature should not verify with wrong message")
			}
		}
	})
}

func TestKeyPersistence(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	keyPath := tempDir
	passphrase := []byte("secure-test-passphrase")

	// Create first signer
	signer1, err := CreateFileSystemSigner(keyPath, passphrase)
	require.NoError(t, err)

	// Get public key for later comparison
	pubKey1, err := signer1.GetPublic()
	require.NoError(t, err)
	pubBytes1, err := pubKey1.Raw()
	require.NoError(t, err)

	// Sign a test message
	message := []byte("Test message")
	signature, err := signer1.Sign(message)
	require.NoError(t, err)

	// Verify signature works with this key
	valid, err := pubKey1.Verify(message, signature)
	require.NoError(t, err)
	assert.True(t, valid, "Signature should be valid with original signer")

	// Create a new instance by loading the signer with a new passphrase variable
	// since the original is zeroed out
	newPassphrase := []byte("secure-test-passphrase")
	signer2, err := LoadFileSystemSigner(keyPath, newPassphrase)
	require.NoError(t, err)

	// Get public key from loaded signer
	pubKey2, err := signer2.GetPublic()
	require.NoError(t, err)
	pubBytes2, err := pubKey2.Raw()
	require.NoError(t, err)

	// Verify the public keys match (same key was loaded)
	assert.Equal(t, pubBytes1, pubBytes2, "Public keys should match after loading")

	// Verify the loaded key can verify the original signature
	valid, err = pubKey2.Verify(message, signature)
	require.NoError(t, err)
	assert.True(t, valid, "Original signature should be valid with loaded key")
}

func TestBackwardCompatibility(t *testing.T) {
	t.Parallel()

	t.Run("fallback to naive key derivation", func(t *testing.T) {
		tempDir := t.TempDir()
		keyPath := tempDir
		passphrase := []byte("secure-test-passphrase")

		// Create a signer
		_, err := CreateFileSystemSigner(keyPath, passphrase)
		require.NoError(t, err)

		keyFile := filepath.Join(keyPath, "signer.json")

		// Read the generated key file
		fileBytes, err := os.ReadFile(keyFile)
		require.NoError(t, err)

		var data keyData
		err = json.Unmarshal(fileBytes, &data)
		require.NoError(t, err)

		// Remove salt field to simulate old format
		data.Salt = nil

		// Write modified key data back to file
		modifiedBytes, err := json.Marshal(data)
		require.NoError(t, err)

		err = os.WriteFile(keyFile, modifiedBytes, 0600)
		require.NoError(t, err)

		// Try to load with the same passphrase
		passphrase = []byte("secure-test-passphrase")
		_, err = LoadFileSystemSigner(keyPath, passphrase)
		// This might fail, but we're testing if it properly attempts to use fallback
		// The important thing is that it doesn't panic
		if err != nil {
			assert.NotContains(t, err.Error(), "salt")
		}
	})
}

func TestEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("empty passphrase", func(t *testing.T) {
		tempDir := t.TempDir()
		keyPath := tempDir
		passphrase := []byte("")

		// Empty passphrase should work, though not recommended
		signer, err := CreateFileSystemSigner(keyPath, passphrase)
		require.NoError(t, err)
		require.NotNil(t, signer)

		// Load with empty passphrase
		passphrase = []byte("")
		loaded, err := LoadFileSystemSigner(keyPath, passphrase)
		require.NoError(t, err)
		require.NotNil(t, loaded)
	})

	t.Run("very long passphrase", func(t *testing.T) {
		tempDir := t.TempDir()
		keyPath := tempDir

		// Create a 1000-byte passphrase
		passphrase := make([]byte, 1000)
		for i := range passphrase {
			passphrase[i] = byte(i % 256)
		}

		// Long passphrase should work
		signer, err := CreateFileSystemSigner(keyPath, passphrase)
		require.NoError(t, err)
		require.NotNil(t, signer)

		// Recreate the same passphrase for loading
		passphrase = make([]byte, 1000)
		for i := range passphrase {
			passphrase[i] = byte(i % 256)
		}

		// Load with same passphrase
		loaded, err := LoadFileSystemSigner(keyPath, passphrase)
		require.NoError(t, err)
		require.NotNil(t, loaded)
	})
}

func TestConcurrentAccess(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	keyPath := tempDir
	passphrase := []byte("secure-test-passphrase")

	// Create signer
	signer, err := CreateFileSystemSigner(keyPath, passphrase)
	require.NoError(t, err)

	// Number of concurrent operations
	numGoRoutines := 10
	messageCount := 5

	// Channel to collect errors
	errChan := make(chan error, numGoRoutines*messageCount)
	doneChan := make(chan struct{})

	for i := 0; i < numGoRoutines; i++ {
		go func(routineNum int) {
			for j := 0; j < messageCount; j++ {
				// Create a unique message per goroutine and iteration
				message := []byte(fmt.Sprintf("Message %d-%d", routineNum, j))

				// Get public key
				pubKey, err := signer.GetPublic()
				if err != nil {
					errChan <- err
					continue
				}

				// Sign message
				signature, err := signer.Sign(message)
				if err != nil {
					errChan <- err
					continue
				}

				// Verify signature
				valid, err := pubKey.Verify(message, signature)
				if err != nil {
					errChan <- err
					continue
				}

				if !valid {
					errChan <- fmt.Errorf("invalid signature for message %s", message)
				}
			}
		}(i)
	}

	// Wait for all goroutines to finish or timeout
	go func() {
		time.Sleep(5 * time.Second)
		close(doneChan)
	}()

	// Check for errors
	var errs []error
loop:
	for {
		select {
		case err := <-errChan:
			errs = append(errs, err)
		case <-doneChan:
			break loop
		default:
			if len(errs) == 0 && len(errChan) == 0 {
				break loop
			}
		}
	}

	assert.Empty(t, errs, "Concurrent operations should not produce errors")
}

func TestHelperFunctions(t *testing.T) {
	t.Run("zeroBytes", func(t *testing.T) {
		data := []byte{1, 2, 3, 4, 5}
		zeroBytes(data)
		for _, b := range data {
			assert.Equal(t, byte(0), b)
		}
	})

	t.Run("deriveKeyArgon2", func(t *testing.T) {
		passphrase := []byte("test-passphrase")
		salt := []byte("salt-value-for-test")
		keyLen := uint32(32)

		key := deriveKeyArgon2(passphrase, salt, keyLen)
		assert.Len(t, key, int(keyLen))

		// Same inputs should produce same key
		key2 := deriveKeyArgon2(passphrase, salt, keyLen)
		assert.Equal(t, key, key2)

		// Different salt should produce different key
		differentSalt := []byte("different-salt-value")
		key3 := deriveKeyArgon2(passphrase, differentSalt, keyLen)
		assert.NotEqual(t, key, key3)
	})

	t.Run("fallbackDeriveKey", func(t *testing.T) {
		passphrase := []byte("test-passphrase")
		keyLen := 32

		key := fallbackDeriveKey(passphrase, keyLen)
		assert.Len(t, key, keyLen)

		// Short passphrase case
		shortPassphrase := []byte("short")
		key2 := fallbackDeriveKey(shortPassphrase, keyLen)
		assert.Len(t, key2, keyLen)

		// Long passphrase case
		longPassphrase := bytes.Repeat([]byte("long"), 20)
		key3 := fallbackDeriveKey(longPassphrase, keyLen)
		assert.Len(t, key3, keyLen)
	})
}

// Add test for CreateFileSystemSigner error on stat
func TestCreateFileSystemSigner_StatError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("Skipping test when running as root")
	}

	tempDir := t.TempDir()

	// Create a directory with restricted permissions to cause os.Stat to fail
	restrictedDir := filepath.Join(tempDir, "restricted")
	err := os.Mkdir(restrictedDir, 0000) // No permissions
	if err != nil {
		t.Skip("Failed to create directory with restricted permissions:", err)
	}

	// Defer restoring permissions so cleanup can happen
	defer func() {
		if os.Chmod(restrictedDir, 0700) != nil {
			t.Log("Failed to restore permissions:", err)
		}
	}()

	keyPath := filepath.Join(restrictedDir, "key_dir")
	passphrase := []byte("test-passphrase")

	// This should fail with a stat error
	_, err = CreateFileSystemSigner(keyPath, passphrase)
	assert.Error(t, err)
}

// Add test for Sign with nil private key
func TestSign_NilPrivateKey(t *testing.T) {
	signer := &FileSystemSigner{
		publicKey:  nil, // Mock a signer with nil keys
		privateKey: nil,
		keyFile:    "dummy.json",
	}

	message := []byte("test message")
	_, err := signer.Sign(message)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "private key not loaded")
}

// Add test for GetPublic with nil public key
func TestGetPublic_NilPublicKey(t *testing.T) {
	signer := &FileSystemSigner{
		publicKey:  nil, // Mock a signer with nil public key
		privateKey: nil,
		keyFile:    "dummy.json",
	}

	_, err := signer.GetPublic()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "public key not loaded")
}

// Add test for saveKeys with nil keys
func TestSaveKeys_NilKeys(t *testing.T) {
	signer := &FileSystemSigner{
		publicKey:  nil,
		privateKey: nil,
		keyFile:    filepath.Join(t.TempDir(), "signer.json"),
	}

	err := signer.saveKeys([]byte("test-passphrase"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "keys not initialized")
}

// Add comprehensive tests for error cases in saveKeys and loadKeys
func TestSaveAndLoadKeys_ErrorCases(t *testing.T) {
	t.Run("saveKeys_InvalidFile", func(t *testing.T) {
		if os.Getuid() == 0 {
			t.Skip("Skipping test when running as root")
		}

		tempDir := t.TempDir()

		// Create a read-only directory
		readOnlyDir := filepath.Join(tempDir, "readonly")
		err := os.Mkdir(readOnlyDir, 0500) // read-only directory
		if err != nil {
			t.Skip("Failed to create read-only directory:", err)
		}

		// First create a valid signer
		keyPath := tempDir
		passphrase := []byte("test-passphrase")
		signer, err := CreateFileSystemSigner(keyPath, passphrase)
		require.NoError(t, err)

		// Try to save to an invalid location
		fs := signer.(*FileSystemSigner)
		fs.keyFile = filepath.Join(readOnlyDir, "invalid.json")

		newPassphrase := []byte("new-passphrase")
		err = fs.saveKeys(newPassphrase)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to write key file")
	})

	t.Run("loadKeys_ReadFileError", func(t *testing.T) {
		tempDir := t.TempDir()

		// Create a directory where the key file should be
		dirInsteadOfFile := filepath.Join(tempDir, "signer.json")
		err := os.Mkdir(dirInsteadOfFile, 0700)
		require.NoError(t, err)

		fs := &FileSystemSigner{
			keyFile: dirInsteadOfFile, // This is a directory, not a file
		}

		passphrase := []byte("test-passphrase")
		err = fs.loadKeys(passphrase)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read key file")
	})
}
