package p2p

import (
	"context"
	"crypto/rand"
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
	"go.uber.org/multierr"

	"github.com/celestiaorg/optimint/config"
	"github.com/celestiaorg/optimint/log"
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

type hostDescr struct {
	chainID string
	conns   []int
	realKey bool
}

// copied from libp2p net/mock
var blackholeIP6 = net.ParseIP("100::")

// copied from libp2p net/mock
func getAddr(sk crypto.PrivKey) (multiaddr.Multiaddr, error) {
	id, err := peer.IDFromPrivateKey(sk)
	if err != nil {
		return nil, err
	}
	suffix := id
	if len(id) > 8 {
		suffix = id[len(id)-8:]
	}
	ip := append(net.IP{}, blackholeIP6...)
	copy(ip[net.IPv6len-len(suffix):], suffix)
	a, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip6/%s/tcp/4242", ip))
	if err != nil {
		return nil, fmt.Errorf("failed to create test multiaddr: %s", err)
	}
	return a, nil
}

func startTestNetwork(ctx context.Context, t *testing.T, n int, conf map[int]hostDescr, validators []GossipValidator, logger log.Logger) testNet {
	t.Helper()
	require := require.New(t)

	mnet := mocknet.New(ctx)
	for i := 0; i < n; i++ {
		var descr hostDescr
		if d, ok := conf[i]; ok {
			descr = d
		}
		if descr.realKey {
			privKey, _, _ := crypto.GenerateEd25519Key(rand.Reader)
			addr, err := getAddr(privKey)
			require.NoError(err)
			host, err := mnet.AddPeer(privKey, addr)
			require.NoError(err)
			require.NotEmpty(host)
		} else {
			_, err := mnet.GenPeer()
			require.NoError(err)
		}
	}

	err := mnet.LinkAll()
	require.NoError(err)

	// prepare seed node lists
	seeds := make([]string, n)
	for src, descr := range conf {
		require.Less(src, n)
		for _, dst := range descr.conns {
			require.Less(dst, n)
			seeds[src] += mnet.Hosts()[dst].Addrs()[0].String() + "/p2p/" + mnet.Peers()[dst].Pretty() + ","
		}
		seeds[src] = strings.TrimSuffix(seeds[src], ",")
	}

	clients := make([]*Client, n)
	for i := 0; i < n; i++ {
		client, err := NewClient(config.P2PConfig{
			Seeds: seeds[i]},
			mnet.Hosts()[i].Peerstore().PrivKey(mnet.Hosts()[i].ID()),
			conf[i].chainID,
			logger)
		require.NoError(err)
		require.NotNil(client)

		client.SetTxValidator(validators[i])
		clients[i] = client
	}

	for i, c := range clients {
		err := c.startWithHost(ctx, mnet.Hosts()[i])
		require.NoError(err)
	}

	return clients
}
