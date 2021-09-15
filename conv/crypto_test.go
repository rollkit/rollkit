package conv

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pb "github.com/libp2p/go-libp2p-core/crypto/pb"
	"github.com/tendermint/tendermint/crypto/ed25519"
	"github.com/tendermint/tendermint/crypto/secp256k1"
	"github.com/tendermint/tendermint/p2p"
)

func TestGetNodeKey(t *testing.T) {
	t.Parallel()

	privKey := ed25519.GenPrivKey()
	valid := p2p.NodeKey{
		PrivKey: privKey,
	}
	invalid := p2p.NodeKey{
		PrivKey: secp256k1.GenPrivKey(),
	}

	cases := []struct {
		name         string
		input        *p2p.NodeKey
		expectedType pb.KeyType
		err          error
	}{
		{"nil", nil, pb.KeyType(-1), errNilKey},
		{"empty", &p2p.NodeKey{}, pb.KeyType(-1), errNilKey},
		{"invalid", &invalid, pb.KeyType(-1), errUnsupportedKeyType},
		{"valid", &valid, pb.KeyType_Ed25519, nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actual, err := GetNodeKey(c.input)
			if c.err != nil {
				assert.Nil(t, actual)
				assert.Error(t, err)
				assert.ErrorIs(t, c.err, err)
			} else {
				require.NotNil(t, actual)
				assert.NoError(t, err)
				assert.Equal(t, c.expectedType, actual.Type())
			}
		})
	}
}
