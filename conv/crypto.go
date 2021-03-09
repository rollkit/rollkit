package conv

import (
	"errors"
	"fmt"

	"github.com/lazyledger/lazyledger-core/p2p"
	"github.com/libp2p/go-libp2p-core/crypto"
)

var ErrNilKey = errors.New("key can't be nil")
var ErrUnsupportedKeyType = errors.New("unsupported key type")

func GetNodeKey(nodeKey *p2p.NodeKey) (crypto.PrivKey, error) {
	if nodeKey == nil || nodeKey.PrivKey == nil {
		return nil, ErrNilKey
	}
	switch nodeKey.PrivKey.Type() {
	case "ed25519":
		privKey, err := crypto.UnmarshalEd25519PrivateKey(nodeKey.PrivKey.Bytes())
		if err != nil {
			return nil, fmt.Errorf("node private key unmarshaling error: %w", err)
		}
		return privKey, nil
	default:
		return nil, ErrUnsupportedKeyType
	}
}
