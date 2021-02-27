package node

import (
	"github.com/lazyledger/lazyledger-core/p2p"
	crypto_pb "github.com/libp2p/go-libp2p-core/crypto/pb"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestLoadPrivKey(t *testing.T) {
	n := &Node{}

	err := n.loadPrivateKey(p2p.GenNodeKey())
	assert.NoError(t, err)
	assert.NotEmpty(t, n.privKey)
	assert.Equal(t, n.privKey.Type(), crypto_pb.KeyType_Ed25519)
}