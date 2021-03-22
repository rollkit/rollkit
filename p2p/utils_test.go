package p2p

import (
	"context"
	"crypto/rand"
	"strconv"
	"testing"

	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/require"
	"go.uber.org/multierr"

	"github.com/lazyledger/optimint/config"
	"github.com/lazyledger/optimint/log"
)

type testNet []*Client

func (tn testNet) Start(ctx context.Context) (err error) {
	for i := range tn {
		err = multierr.Append(err, tn[i].Start(ctx))
	}
	return
}

func (tn testNet) Close() (err error) {
	for i := range tn {
		err = multierr.Append(err, tn[i].Close())
	}
	return
}

func (tn testNet) WaitForDHT() {
	for i := range tn {
		<-tn[i].dht.RefreshRoutingTable()
	}
}

func makeTestNetowork(t *testing.T, n int, conns map[int][]int, logger log.Logger) testNet {
	t.Helper()
	require := require.New(t)

	basePort := 8000
	getAddr := func(i int) string {
		return "/ip4/127.0.0.1/tcp/" + strconv.Itoa(basePort+i)
	}

	privKeys := make([]crypto.PrivKey, n)
	cids := make([]peer.ID, n)
	for i := 0; i < n; i++ {
		privKeys[i], _, _ = crypto.GenerateEd25519Key(rand.Reader)
		cids[i], _ = peer.IDFromPrivateKey(privKeys[i])
	}

	// prepare seed node lists
	seeds := make([]string, n)
	for src, dsts := range conns {
		require.Less(src, n)
		for _, dst := range dsts {
			require.Less(dst, n)
			seeds[src] += getAddr(dst) + "/p2p/" + cids[dst].Pretty() + ","
		}

	}

	clients := make([]*Client, n)
	for i := 0; i < n; i++ {
		client, err := NewClient(config.P2PConfig{
			Seeds:         seeds[i],
			ListenAddress: getAddr(i)},
			privKeys[i],
			logger)
		require.NoError(err)
		require.NotNil(client)

		clients[i] = client
	}

	return clients
}
