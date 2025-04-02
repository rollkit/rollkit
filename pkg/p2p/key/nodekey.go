package key

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/libp2p/go-libp2p/core/crypto"

	rollhash "github.com/rollkit/rollkit/pkg/hash"
	rollos "github.com/rollkit/rollkit/pkg/os"
)

// NodeKey is the persistent peer key.
// It contains the nodes private key for authentication.
type NodeKey struct {
	PrivKey crypto.PrivKey // our priv key
	PubKey  crypto.PubKey  // our pub key
}

type nodeKeyJSON struct {
	PrivKeyBytes []byte `json:"priv_key"`
	PubKeyBytes  []byte `json:"pub_key"`
}

// MarshalJSON implements the json.Marshaler interface.
func (nodeKey *NodeKey) MarshalJSON() ([]byte, error) {
	if nodeKey.PrivKey == nil || nodeKey.PubKey == nil {
		return nil, fmt.Errorf("nodeKey has nil key(s)")
	}

	privBytes, err := nodeKey.PrivKey.Raw()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal private key: %w", err)
	}

	pubBytes, err := nodeKey.PubKey.Raw()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal public key: %w", err)
	}

	return json.Marshal(nodeKeyJSON{
		PrivKeyBytes: privBytes,
		PubKeyBytes:  pubBytes,
	})
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (nodeKey *NodeKey) UnmarshalJSON(data []byte) error {
	aux := nodeKeyJSON{}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	privKey, err := crypto.UnmarshalEd25519PrivateKey(aux.PrivKeyBytes)
	if err != nil {
		return fmt.Errorf("failed to unmarshal private key: %w", err)
	}

	pubKey, err := crypto.UnmarshalEd25519PublicKey(aux.PubKeyBytes)
	if err != nil {
		return fmt.Errorf("failed to unmarshal public key: %w", err)
	}

	nodeKey.PrivKey = privKey
	nodeKey.PubKey = pubKey

	return nil
}

// ID returns the peer's canonical ID - the hash of its public key.
func (nodeKey *NodeKey) ID() string {
	return PubKeyToID(nodeKey.PubKey)
}

// SaveAs persists the NodeKey to filePath.
func (nodeKey *NodeKey) SaveAs(filePath string) error {
	jsonBytes, err := json.Marshal(nodeKey)
	if err != nil {
		return err
	}
	// create directory if it doesn't exist
	err = os.MkdirAll(filepath.Dir(filePath), 0755)
	if err != nil {
		return err
	}

	err = os.WriteFile(filePath, jsonBytes, 0600)
	if err != nil {
		return err
	}
	return nil
}

// PubKeyToID returns the ID corresponding to the given PubKey.
// It's the hex-encoding of the pubKey.Address().
func PubKeyToID(pubKey crypto.PubKey) string {
	if pubKey == nil {
		return ""
	}
	raw, err := pubKey.Raw()
	if err != nil {
		return ""
	}

	return hex.EncodeToString(rollhash.SumTruncated(raw))
}

// LoadOrGenNodeKey attempts to load the NodeKey from the given filePath. If
// the file does not exist, it generates and saves a new NodeKey.
func LoadOrGenNodeKey(filePath string) (*NodeKey, error) {
	if rollos.FileExists(filePath) {
		nodeKey, err := LoadNodeKey(filePath)
		if err != nil {
			return nil, err
		}
		return nodeKey, nil
	}

	privKey, pubKey, err := crypto.GenerateKeyPair(crypto.Ed25519, 256)
	if err != nil {
		return nil, err
	}
	nodeKey := &NodeKey{
		PrivKey: privKey,
		PubKey:  pubKey,
	}

	if err := nodeKey.SaveAs(filePath); err != nil {
		return nil, err
	}

	return nodeKey, nil
}

// LoadNodeKey loads NodeKey located in filePath.
func LoadNodeKey(filePath string) (*NodeKey, error) {
	jsonBytes, err := os.ReadFile(filePath) //nolint:gosec
	if err != nil {
		return nil, err
	}
	nodeKey := new(NodeKey)
	err = json.Unmarshal(jsonBytes, nodeKey)
	if err != nil {
		return nil, err
	}
	return nodeKey, nil
}
