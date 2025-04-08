package signing

import (
	"crypto/ed25519"
	"fmt"
	"os"

	// Reference the config package within the same module
	"github.com/rollkit/rollkit/attester/internal/config"
)

// Signer defines the interface for signing data.
type Signer interface {
	Sign(data []byte) ([]byte, error)
	PublicKey() []byte // Returns the raw bytes of the public key
	Scheme() string    // Returns the name of the scheme (e.g., "ed25519")
}

// Ed25519Signer implements the Signer interface for Ed25519.
type Ed25519Signer struct {
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
}

// NewEd25519Signer creates a new Ed25519 signer from a private key.
func NewEd25519Signer(privKey ed25519.PrivateKey) (*Ed25519Signer, error) {
	if len(privKey) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("invalid ed25519 private key size: %d", len(privKey))
	}
	pubKey, ok := privKey.Public().(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("failed to derive public key from private key")
	}
	if len(pubKey) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid ed25519 public key size derived: %d", len(pubKey))
	}
	return &Ed25519Signer{
		privateKey: privKey,
		publicKey:  pubKey,
	}, nil
}

// Sign signs the data using the Ed25519 private key.
func (s *Ed25519Signer) Sign(data []byte) ([]byte, error) {
	return ed25519.Sign(s.privateKey, data), nil
}

// PublicKey returns the bytes of the Ed25519 public key.
func (s *Ed25519Signer) PublicKey() []byte {
	return s.publicKey
}

// Scheme returns the name of the signing scheme.
func (s *Ed25519Signer) Scheme() string {
	return "ed25519"
}

// LoadSigner loads the private key from the path specified in the configuration
// and creates the appropriate Signer implementation based on the scheme.
func LoadSigner(cfg config.SigningConfig) (Signer, error) {
	privKeyBytes, err := os.ReadFile(cfg.PrivateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key file %s: %w", cfg.PrivateKeyPath, err)
	}

	switch cfg.Scheme {
	case "ed25519":
		if len(privKeyBytes) != ed25519.PrivateKeySize {
			return nil, fmt.Errorf("invalid private key size for ed25519 in file %s: expected %d, got %d",
				cfg.PrivateKeyPath, ed25519.PrivateKeySize, len(privKeyBytes))
		}
		privKey := ed25519.PrivateKey(privKeyBytes)
		return NewEd25519Signer(privKey)
	default:
		return nil, fmt.Errorf("unknown signing scheme specified in config: '%s'", cfg.Scheme)
	}
}
