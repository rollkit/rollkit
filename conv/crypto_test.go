package conv

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lazyledger/lazyledger-core/crypto/secp256k1"
	"github.com/lazyledger/lazyledger-core/p2p"
	pb "github.com/libp2p/go-libp2p-core/crypto/pb"
)

func TestGetNodeKey(t *testing.T) {
	t.Parallel()

	valid := p2p.GenNodeKey()
	invalid := p2p.NodeKey{
		PrivKey: secp256k1.GenPrivKey(),
	}

	cases := []struct {
		name         string
		input        *p2p.NodeKey
		expectedType pb.KeyType
		err          error
	}{
		{"nil", nil, pb.KeyType(-1), ErrNilKey},
		{"empty", &p2p.NodeKey{}, pb.KeyType(-1), ErrNilKey},
		{"invalid", &invalid, pb.KeyType(-1), ErrUnsupportedKeyType},
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
