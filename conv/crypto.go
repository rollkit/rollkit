package conv

import (
	"errors"
	"fmt"

	"github.com/libp2p/go-libp2p-core/crypto"

	"github.com/tendermint/tendermint/p2p"
)

var (
	errNilKey             = errors.New("key can't be nil")
	errUnsupportedKeyType = errors.New("unsupported key type")
)

// GetNodeKey creates libp2p private key from Tendermints NodeKey.
func GetNodeKey(nodeKey *p2p.NodeKey) (crypto.PrivKey, error) {
	if nodeKey == nil || nodeKey.PrivKey == nil {
		return nil, errNilKey
	}
	switch nodeKey.PrivKey.Type() {
	case "ed25519":
		privKey, err := crypto.UnmarshalEd25519PrivateKey(nodeKey.PrivKey.Bytes())
		if err != nil {
			return nil, fmt.Errorf("node private key unmarshaling error: %w", err)
		}
		return privKey, nil
	default:
		return nil, errUnsupportedKeyType
	}
}
