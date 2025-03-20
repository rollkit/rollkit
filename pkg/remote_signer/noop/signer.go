package noop

import (
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/rollkit/rollkit/pkg/remote_signer"
)

// NoopSigner implements the remote_signer.Signer interface.
// It generates a new Ed25519 key pair for each instance.
type NoopSigner struct {
	privKey crypto.PrivKey
	pubKey  crypto.PubKey
}

// NewNoopSigner creates a new signer with a fresh Ed25519 key pair.
func NewNoopSigner() (remote_signer.Signer, error) {
	// Generate new Ed25519 key pair
	priv, pub, err := crypto.GenerateKeyPair(crypto.Ed25519, 256)
	if err != nil {
		return nil, err
	}

	return &NoopSigner{
		privKey: priv,
		pubKey:  pub,
	}, nil
}

// Sign implements the Signer interface by signing the message with the Ed25519 private key.
func (n *NoopSigner) Sign(message []byte) ([]byte, error) {
	return n.privKey.Sign(message)
}

// GetPublic implements the Signer interface by returning the Ed25519 public key.
func (n *NoopSigner) GetPublic() (crypto.PubKey, error) {
	return n.pubKey, nil
}
