package signer

import (
	"github.com/libp2p/go-libp2p/core/crypto"
)

// KeyProvider is an interface that provides access to signing keys
type KeyProvider interface {
	// GetSigningKey returns the private key used for signing
	GetSigningKey() (crypto.PrivKey, error)
}
