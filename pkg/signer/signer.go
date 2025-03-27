package signer

import (
	"github.com/libp2p/go-libp2p/core/crypto"
)

// Signer is an interface that abstracts the signing key management
type Signer interface {
	// GetSigningKey returns the private key used for signing
	GetSigningKey() (crypto.PrivKey, error)
}

// SignerProvider is an interface for creating new signers
type SignerProvider interface {
	// NewSigner creates a new signer instance
	NewSigner() (Signer, error)
}
