package file

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/libp2p/go-libp2p/core/crypto"
	"golang.org/x/crypto/argon2"

	"github.com/rollkit/rollkit/pkg/signer"
)

// FileSystemSigner implements a signer that securely stores keys on disk
// and loads them into memory only when needed.
type FileSystemSigner struct {
	privateKey crypto.PrivKey
	publicKey  crypto.PubKey
	keyFile    string
	mu         sync.RWMutex
}

// keyData represents the encrypted key data stored on disk
type keyData struct {
	PrivKeyEncrypted []byte `json:"priv_key_encrypted"`
	Nonce            []byte `json:"nonce"`
	PubKeyBytes      []byte `json:"pub_key"`
	Salt             []byte `json:"salt,omitempty"`
}

// CreateFileSystemSigner creates a new key pair and saves it encrypted to disk.
func CreateFileSystemSigner(keyPath string, passphrase []byte) (signer.Signer, error) {
	defer zeroBytes(passphrase) // Wipe passphrase from memory after use

	filePath := filepath.Join(keyPath, "signer.json")

	// Create directory if it doesn't exist
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	// Check if key file already exists
	if _, err := os.Stat(filePath); err == nil {
		return nil, fmt.Errorf("key file already exists at %s", filePath)
	} else if !os.IsNotExist(err) {
		// Handle other potential errors from os.Stat
		return nil, fmt.Errorf("failed to check key file status: %w", err)
	}

	// Generate new Ed25519 key pair
	privKey, pubKey, err := crypto.GenerateKeyPair(crypto.Ed25519, 256)
	if err != nil {
		return nil, fmt.Errorf("failed to generate key pair: %w", err)
	}

	signer := &FileSystemSigner{
		privateKey: privKey,
		publicKey:  pubKey,
		keyFile:    filePath,
	}

	// Save keys to disk
	if err := signer.saveKeys(passphrase); err != nil {
		// Attempt to clean up the file if saving failed partially
		_ = os.Remove(filePath)
		return nil, fmt.Errorf("failed to save keys: %w", err)
	}

	return signer, nil
}

// LoadFileSystemSigner loads existing keys from an encrypted file on disk.
func LoadFileSystemSigner(keyPath string, passphrase []byte) (signer.Signer, error) {
	defer zeroBytes(passphrase) // Wipe passphrase from memory after use

	filePath := filepath.Join(keyPath, "signer.json")

	// Check if key file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("key file not found at %s: %w", filePath, err)
	} else if err != nil {
		// Handle other potential errors from os.Stat
		return nil, fmt.Errorf("failed to check key file status: %w", err)
	}

	// Load existing keys
	signer := &FileSystemSigner{
		keyFile: filePath,
	}

	// Load keys into memory
	if err := signer.loadKeys(passphrase); err != nil {
		return nil, err // loadKeys already provides context
	}

	return signer, nil
}

// saveKeys encrypts and saves the private key to disk
func (s *FileSystemSigner) saveKeys(passphrase []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.privateKey == nil || s.publicKey == nil {
		return fmt.Errorf("keys not initialized")
	}

	// Create or reuse a random salt for Argon2
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return fmt.Errorf("failed to generate salt: %w", err)
	}

	// Get raw private key bytes
	privKeyBytes, err := s.privateKey.Raw()
	if err != nil {
		return fmt.Errorf("failed to get raw private key: %w", err)
	}

	// Get raw public key bytes
	pubKeyBytes, err := s.publicKey.Raw()
	if err != nil {
		return fmt.Errorf("failed to get raw public key: %w", err)
	}

	// Derive a key with Argon2
	derivedKey := deriveKeyArgon2(passphrase, salt, 32)

	// Zero out passphrase from memory once we have our derived key
	zeroBytes(passphrase)

	// Create AES cipher
	block, err := aes.NewCipher(derivedKey)
	if err != nil {
		return fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("failed to create GCM: %w", err)
	}

	// Create a nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt the private key
	encryptedPrivKey := gcm.Seal(nil, nonce, privKeyBytes, nil)

	// Create key data structure
	data := keyData{
		PrivKeyEncrypted: encryptedPrivKey,
		Nonce:            nonce,
		PubKeyBytes:      pubKeyBytes,
		Salt:             salt,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal key data: %w", err)
	}

	// Write to file with secure permissions
	if err := os.WriteFile(s.keyFile, jsonData, 0600); err != nil {
		return fmt.Errorf("failed to write key file: %w", err)
	}

	// Zero out derivedKey bytes for security
	zeroBytes(derivedKey)
	zeroBytes(privKeyBytes)

	return nil
}

// loadKeys decrypts and loads the keys from disk into memory
func (s *FileSystemSigner) loadKeys(passphrase []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer zeroBytes(passphrase) // Wipe passphrase from memory after use

	// Read the key file
	jsonData, err := os.ReadFile(s.keyFile)
	if err != nil {
		return fmt.Errorf("failed to read key file: %w", err)
	}

	// Unmarshal JSON
	var data keyData
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return fmt.Errorf("failed to unmarshal key data: %w", err)
	}

	// If there's no salt in the file, fallback to older naive deriveKey (for backward-compatibility)
	var derivedKey []byte
	if len(data.Salt) == 0 {
		// fallback to naive approach
		derivedKey = fallbackDeriveKey(passphrase, 32)
	} else {
		derivedKey = deriveKeyArgon2(passphrase, data.Salt, 32)
	}

	block, err := aes.NewCipher(derivedKey)
	if err != nil {
		return fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("failed to create GCM: %w", err)
	}

	// Decrypt the private key
	privKeyBytes, err := gcm.Open(nil, data.Nonce, data.PrivKeyEncrypted, nil)
	if err != nil {
		return fmt.Errorf("failed to decrypt private key (wrong passphrase?): %w", err)
	}

	// Unmarshal the private key
	privKey, err := crypto.UnmarshalEd25519PrivateKey(privKeyBytes)
	if err != nil {
		return fmt.Errorf("failed to unmarshal private key: %w", err)
	}

	// Unmarshal the public key
	pubKey, err := crypto.UnmarshalEd25519PublicKey(data.PubKeyBytes)
	if err != nil {
		return fmt.Errorf("failed to unmarshal public key: %w", err)
	}

	// Set the keys
	s.privateKey = privKey
	s.publicKey = pubKey

	// Zero out sensitive data
	zeroBytes(derivedKey)

	return nil
}

// Sign signs a message using the private key
func (s *FileSystemSigner) Sign(message []byte) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.privateKey == nil {
		return nil, fmt.Errorf("private key not loaded")
	}

	return s.privateKey.Sign(message)
}

// GetPublic returns the public key
func (s *FileSystemSigner) GetPublic() (crypto.PubKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.publicKey == nil {
		return nil, fmt.Errorf("public key not loaded")
	}

	return s.publicKey, nil
}

// GetAddress returns the address of the signer
func (s *FileSystemSigner) GetAddress() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return getAddress(s.publicKey)
}

// deriveKeyArgon2 uses Argon2id for key derivation
func deriveKeyArgon2(passphrase, salt []byte, keyLen uint32) []byte {
	// Using some default parameters:
	// Time (iterations) = 3
	// Memory = 32MB
	// Threads = 4
	// You can tune these parameters for your security/performance needs.
	return argon2.IDKey(passphrase, salt, 3, 32*1024, 4, keyLen)
}

// fallbackDeriveKey is the old naive approach for backwards compatibility.
// Will be used if a key file has no salt field.
func fallbackDeriveKey(passphrase []byte, keyLen int) []byte {
	if len(passphrase) >= keyLen {
		return passphrase[:keyLen]
	}

	key := make([]byte, keyLen)
	copy(key, passphrase)
	for i := len(passphrase); i < keyLen; i++ {
		key[i] = passphrase[i%len(passphrase)] ^ byte(i)
	}
	return key
}

// zeroBytes overwrites a byte slice with zeros
func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// getAddress returns the Ed25519 address of the signer.
func getAddress(pubKey crypto.PubKey) ([]byte, error) {
	bz, err := pubKey.Raw()
	if err != nil {
		return nil, err
	}

	address := sha256.Sum256(bz)
	return address[:], nil
}
