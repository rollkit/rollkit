package signer

import (
	"path/filepath"

	cometprivval "github.com/cometbft/cometbft/privval"
	"github.com/libp2p/go-libp2p/core/crypto"
)

// FileKeyProvider implements the KeyProvider interface using files to store the keys
type FileKeyProvider struct {
	keyFile    string
	stateFile  string
	privateKey crypto.PrivKey
}

// NewFileKeyProvider creates a new FileKeyProvider
func NewFileKeyProvider(rootDir, configDir, dbPath string) *FileKeyProvider {
	return &FileKeyProvider{
		keyFile:   filepath.Join(rootDir, configDir, "priv_validator_key.json"),
		stateFile: filepath.Join(rootDir, dbPath, "priv_validator_state.json"),
	}
}

// GetSigningKey returns the private key used for signing
func (p *FileKeyProvider) GetSigningKey() (crypto.PrivKey, error) {
	if p.privateKey != nil {
		return p.privateKey, nil
	}

	pval := cometprivval.LoadOrGenFilePV(p.keyFile, p.stateFile)
	privateKey, err := crypto.UnmarshalEd25519PrivateKey(pval.Key.PrivKey.Bytes())
	if err != nil {
		return nil, err
	}

	p.privateKey = privateKey
	return privateKey, nil
}
