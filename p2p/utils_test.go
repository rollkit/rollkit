package p2p

import (
	"context"
	"testing"

	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/multierr"

	"github.com/lazyledger/optimint/config"
	"github.com/lazyledger/optimint/log"
)

type testNet []*Client

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

func startTestNetwork(ctx context.Context, t *testing.T, n int, conns map[int][]int, logger log.Logger) testNet {
	t.Helper()
	require := require.New(t)

	mnet, err := mocknet.FullMeshLinked(ctx, n)
	require.NoError(err)

	// prepare seed node lists
	seeds := make([]string, n)
	for src, dsts := range conns {
		require.Less(src, n)
		for _, dst := range dsts {
			require.Less(dst, n)
			seeds[src] += mnet.Hosts()[dst].Addrs()[0].String() + "/p2p/" + mnet.Peers()[dst].Pretty() + ","
		}

	}

	clients := make([]*Client, n)
	for i := 0; i < n; i++ {
		client, err := NewClient(config.P2PConfig{
			Seeds: seeds[i]},
			mnet.Hosts()[i].Peerstore().PrivKey(mnet.Hosts()[i].ID()),
			logger)
		require.NoError(err)
		require.NotNil(client)

		clients[i] = client
	}

	for i, c := range clients {
		c.startWithHost(ctx, mnet.Hosts()[i])
	}

	return clients
}
