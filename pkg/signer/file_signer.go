package signer

import (
	"path/filepath"

	cometprivval "github.com/cometbft/cometbft/privval"
	"github.com/libp2p/go-libp2p/core/crypto"
)

// FileSigner implements the Signer interface using files to store the keys
type FileSigner struct {
	keyFile    string
	stateFile  string
	privateKey crypto.PrivKey
}

// FileSignerProvider implements the SignerProvider interface for file-based signers
type FileSignerProvider struct {
	keyFile   string
	stateFile string
}

// NewFileSignerProvider creates a new FileSignerProvider
func NewFileSignerProvider(rootDir, configDir, dbPath string) *FileSignerProvider {
	return &FileSignerProvider{
		keyFile:   filepath.Join(rootDir, configDir, "priv_validator_key.json"),
		stateFile: filepath.Join(rootDir, dbPath, "priv_validator_state.json"),
	}
}

// NewSigner creates a new FileSigner instance
func (p *FileSignerProvider) NewSigner() (Signer, error) {
	return &FileSigner{
		keyFile:   p.keyFile,
		stateFile: p.stateFile,
	}, nil
}

// GetSigningKey returns the private key used for signing
func (s *FileSigner) GetSigningKey() (crypto.PrivKey, error) {
	if s.privateKey != nil {
		return s.privateKey, nil
	}

	pval := cometprivval.LoadOrGenFilePV(s.keyFile, s.stateFile)
	privateKey, err := crypto.UnmarshalEd25519PrivateKey(pval.Key.PrivKey.Bytes())
	if err != nil {
		return nil, err
	}

	s.privateKey = privateKey
	return privateKey, nil
}
