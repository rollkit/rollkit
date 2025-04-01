package p2p

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"testing"

	"cosmossdk.io/log"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/sync"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/pkg/config"
	"github.com/rollkit/rollkit/pkg/p2p/key"
)

type testNet []*Client

func (tn testNet) Close() (err error) {
	for i := range tn {
		err = errors.Join(err, tn[i].Close())
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
var unicastAddr = net.ParseIP("2000::")

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
	ip := append(net.IP{}, unicastAddr...)
	copy(ip[net.IPv6len-len(suffix):], suffix)
	a, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip6/%s/tcp/4242", ip))
	if err != nil {
		return nil, fmt.Errorf("failed to create test multiaddr: %w", err)
	}
	return a, nil
}

func startTestNetwork(ctx context.Context, t *testing.T, n int, conf map[int]hostDescr, logger log.Logger) testNet {
	t.Helper()
	require := require.New(t)

	mnet := mocknet.New()
	for i := 0; i < n; i++ {
		var descr hostDescr
		if d, ok := conf[i]; ok {
			descr = d
		}
		// workaround to always generate our own addr
		descr.realKey = true
		if descr.realKey {
			privKey, _, _ := crypto.GenerateEd25519Key(rand.Reader)
			addr, err := getAddr(privKey)
			require.NoError(err)
			host, err := mnet.AddPeer(privKey, addr)
			require.NoError(err)
			require.NotNil(host)
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
			seeds[src] += mnet.Hosts()[dst].Addrs()[0].String() + "/p2p/" + mnet.Peers()[dst].Loggable()["peerID"].(string) + ","
		}
		seeds[src] = strings.TrimSuffix(seeds[src], ",")
	}

	clients := make([]*Client, n)
	for i := 0; i < n; i++ {
		tempDir := filepath.Join(t.TempDir(), fmt.Sprintf("client_%d", i))
		ClientInitFiles(t, tempDir)
		nodeKey, err := key.LoadOrGenNodeKey(filepath.Join(tempDir, "config", "node_key.json"))
		require.NoError(err)

		client, err := NewClient(
			config.Config{
				RootDir: tempDir,
				P2P: config.P2PConfig{
					Seeds: seeds[i],
				},
			},
			conf[i].chainID,
			sync.MutexWrap(datastore.NewMapDatastore()),
			logger,
			NopMetrics(),
			*nodeKey,
		)
		require.NoError(err)
		require.NotNil(client)

		clients[i] = client
	}

	for i, c := range clients {
		err := c.startWithHost(ctx, mnet.Hosts()[i])
		require.NoError(err)
	}

	return clients
}
