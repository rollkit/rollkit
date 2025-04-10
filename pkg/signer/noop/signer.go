package noop

import (
	"crypto/sha256"
	"fmt"

	"github.com/libp2p/go-libp2p/core/crypto"

	"github.com/rollkit/rollkit/pkg/signer"
)

// NoopSigner implements the remote_signer.Signer interface.
// It generates a new Ed25519 key pair for each instance.
type NoopSigner struct {
	privKey crypto.PrivKey
	pubKey  crypto.PubKey
	address []byte
}

// NewNoopSigner creates a new signer with a fresh Ed25519 key pair.
func NewNoopSigner(privKey crypto.PrivKey) (signer.Signer, error) {
	sig := &NoopSigner{
		privKey: privKey,
		pubKey:  privKey.GetPublic(),
	}

	address, err := getAddress(sig.pubKey)
	if err != nil {
		return nil, err
	}
	sig.address = address

	return sig, nil
}

// NewNoopSignerFromPubKey creates a new signer from a public key.
// The returned signer can't be used to sign messages.
func NewNoopSignerFromPubKey(pubKey crypto.PubKey) (signer.Signer, error) {
	sig := &NoopSigner{
		pubKey: pubKey,
	}

	if pubKey == nil {
		return sig, nil
	}

	address, err := getAddress(sig.pubKey)
	if err != nil {
		return nil, err
	}
	sig.address = address

	return sig, nil
}

// NewNoopSignerFromAddress creates a new signer from an address.
func NewNoopSignerFromAddress(address []byte) (signer.Signer, error) {
	return &NoopSigner{
		address: address,
	}, nil
}

// Sign implements the Signer interface by signing the message with the Ed25519 private key.
func (n *NoopSigner) Sign(message []byte) ([]byte, error) {
	if n.privKey == nil {
		return nil, fmt.Errorf("private key not loaded")
	}
	return n.privKey.Sign(message)
}

// GetPublic implements the Signer interface by returning the Ed25519 public key.
func (n *NoopSigner) GetPublic() (crypto.PubKey, error) {
	return n.pubKey, nil
}

// GetAddress implements the Signer interface by returning the Ed25519 address.
func (n *NoopSigner) GetAddress() ([]byte, error) {
	return n.address, nil
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
