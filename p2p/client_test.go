package p2p

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"cosmossdk.io/log"
	"github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/config"
)

func TestClientStartup(t *testing.T) {
	privKey, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	assert := assert.New(t)
	testCases := []struct {
		desc    string
		p2pconf config.P2PConfig
	}{
		{"blank_config", config.P2PConfig{}},
		{"peer_whitelisting", config.P2PConfig{
			ListenAddress: "",
			Seeds:         "",
			BlockedPeers:  "",
			AllowedPeers:  "/ip4/127.0.0.1/tcp/7676/p2p/12D3KooWM1NFkZozoatQi3JvFE57eBaX56mNgBA68Lk5MTPxBE4U",
		}},
		{"peer_blacklisting", config.P2PConfig{
			ListenAddress: "",
			Seeds:         "",
			BlockedPeers:  "/ip4/127.0.0.1/tcp/7676/p2p/12D3KooWM1NFkZozoatQi3JvFE57eBaX56mNgBA68Lk5MTPxBE4U",
			AllowedPeers:  "",
		}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.desc, func(t *testing.T) {
			client, err := NewClient(testCase.p2pconf, privKey, "TestChain",
				dssync.MutexWrap(datastore.NewMapDatastore()), log.NewTestLogger(t), NopMetrics())
			assert.NoError(err)
			assert.NotNil(client)

			err = client.Start(context.Background())
			defer func() {
				_ = client.Close()
			}()
			assert.NoError(err)
		})
	}
}

func TestBootstrapping(t *testing.T) {
	assert := assert.New(t)
	logger := log.NewTestLogger(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	clients := startTestNetwork(ctx, t, 4, map[int]hostDescr{
		1: {conns: []int{0}},
		2: {conns: []int{0, 1}},
		3: {conns: []int{0}},
	}, logger)

	// wait for clients to finish refreshing routing tables
	clients.WaitForDHT()

	// Add a small delay to allow connections to be established
	time.Sleep(100 * time.Millisecond)

	for _, client := range clients {
		assert.Equal(3, len(client.host.Network().Peers()))
	}
}

func TestDiscovery(t *testing.T) {
	assert := assert.New(t)
	logger := log.NewTestLogger(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	clients := startTestNetwork(ctx, t, 5, map[int]hostDescr{
		1: {conns: []int{0}, chainID: "ORU2"},
		2: {conns: []int{0}, chainID: "ORU2"},
		3: {conns: []int{1}, chainID: "ORU1"},
		4: {conns: []int{2}, chainID: "ORU1"},
	}, logger)

	// wait for clients to finish refreshing routing tables
	clients.WaitForDHT()

	assert.Contains(clients[3].host.Network().Peers(), clients[4].host.ID())
	assert.Contains(clients[4].host.Network().Peers(), clients[3].host.ID())
}

func TestSeedStringParsing(t *testing.T) {
	t.Parallel()

	privKey, _, _ := crypto.GenerateEd25519Key(rand.Reader)

	seed1 := "/ip4/127.0.0.1/tcp/7676/p2p/12D3KooWM1NFkZozoatQi3JvFE57eBaX56mNgBA68Lk5MTPxBE4U"
	seed1MA, err := multiaddr.NewMultiaddr(seed1)
	require.NoError(t, err)
	seed1AI, err := peer.AddrInfoFromP2pAddr(seed1MA)
	require.NoError(t, err)

	seed2 := "/ip4/127.0.0.1/tcp/7677/p2p/12D3KooWAPRFbmWF5dAXvxLnEDxiHWhUuApVDpNNZwShiFAiJqrj"
	seed2MA, err := multiaddr.NewMultiaddr(seed2)
	require.NoError(t, err)
	seed2AI, err := peer.AddrInfoFromP2pAddr(seed2MA)
	require.NoError(t, err)

	// this one is a valid multiaddr, but can't be converted to PeerID (because there is no ID)
	seed3 := "/ip4/127.0.0.1/tcp/12345"

	cases := []struct {
		name     string
		input    string
		expected []peer.AddrInfo
		nErrors  int
	}{
		{"empty input", "", []peer.AddrInfo{}, 0},
		{"one correct seed", seed1, []peer.AddrInfo{*seed1AI}, 0},
		{"two correct seeds", seed1 + "," + seed2, []peer.AddrInfo{*seed1AI, *seed2AI}, 0},
		{"one wrong, two correct", "/ip4/," + seed1 + "," + seed2, []peer.AddrInfo{*seed1AI, *seed2AI}, 1},
		{"empty, two correct", "," + seed1 + "," + seed2, []peer.AddrInfo{*seed1AI, *seed2AI}, 1},
		{"empty, correct, empty, correct ", "," + seed1 + ",," + seed2, []peer.AddrInfo{*seed1AI, *seed2AI}, 2},
		{"invalid id, two correct", seed3 + "," + seed1 + "," + seed2, []peer.AddrInfo{*seed1AI, *seed2AI}, 1},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert := assert.New(t)
			require := require.New(t)
			logger := log.NewNopLogger()
			client, err := NewClient(config.P2PConfig{}, privKey, "TestNetwork",
				dssync.MutexWrap(datastore.NewMapDatastore()), logger, NopMetrics())
			require.NoError(err)
			require.NotNil(client)
			actual := client.parseAddrInfoList(c.input)
			assert.NotNil(actual)
			assert.Equal(c.expected, actual)
		})
	}
}
