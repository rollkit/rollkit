package p2p

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cosmossdk.io/log"
	"github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	libp2p "github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"

	"github.com/rollkit/rollkit/pkg/config"
	"github.com/rollkit/rollkit/pkg/p2p/key"
)

func TestNewClientWithHost(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)

	// Common setup
	conf := config.DefaultConfig
	conf.RootDir = t.TempDir()
	nodeKey, err := key.LoadOrGenNodeKey(filepath.Join(conf.RootDir, "config", "node_key.json"))
	require.NoError(err)
	ds := dssync.MutexWrap(datastore.NewMapDatastore())
	logger := log.NewTestLogger(t)
	metrics := NopMetrics()

	// Ensure config directory exists for nodeKey loading
	ClientInitFiles(t, conf.RootDir)

	t.Run("successful client creation with injected host", func(t *testing.T) {
		// First, create a client to get its expected gater
		baseClient, err := NewClient(conf, nodeKey, ds, logger, metrics)
		require.NoError(err)
		require.NotNil(baseClient)

		mn := mocknet.New()
		defer mn.Close()

		h, err := libp2p.New(
			libp2p.Identity(nodeKey.PrivKey),
			libp2p.ListenAddrs(multiaddr.StringCast("/ip4/127.0.0.1/tcp/0")),
			libp2p.ConnectionGater(baseClient.ConnectionGater()), // Use the gater from baseClient
		)
		require.NoError(err)
		defer h.Close()

		client, err := NewClientWithHost(conf, nodeKey, ds, logger, metrics, h)
		assert.NoError(err)
		assert.NotNil(client)
		assert.Equal(h, client.Host())
	})

	t.Run("error when injected host ID does not match node key ID", func(t *testing.T) {
		mn := mocknet.New()
		defer mn.Close()

		// Generate a different node key for the host
		otherNodeKey, err := key.GenerateNodeKey()
		require.NoError(err)

		h, err := mn.AddPeer(otherNodeKey.PrivKey, multiaddr.StringCast("/ip4/127.0.0.1/tcp/0"))
		require.NoError(err)

		client, err := NewClientWithHost(conf, nodeKey, ds, logger, metrics, h)
		assert.Error(err)
		assert.Nil(client)
		assert.Contains(err.Error(), "injected host ID")
		assert.Contains(err.Error(), "does not match node key ID")
	})

}

func TestClientStartup(t *testing.T) {
	assert := assert.New(t)
	// create temp config dir
	tempDir := t.TempDir()
	ClientInitFiles(t, tempDir)

	nodeKey, err := key.LoadOrGenNodeKey(filepath.Join(tempDir, "config", "node_key.json"))
	assert.NoError(err)

	defaultConfig := config.DefaultConfig

	testCases := []struct {
		desc string
		conf config.Config
	}{
		{"peer_whitelisting", config.Config{
			ChainID: "TestChain",
			RootDir: tempDir,
			P2P: config.P2PConfig{
				ListenAddress: defaultConfig.P2P.ListenAddress,
				Peers:         "",
				BlockedPeers:  "",
				AllowedPeers:  "/ip4/127.0.0.1/tcp/7676/p2p/12D3KooWM1NFkZozoatQi3JvFE57eBaX56mNgBA68Lk5MTPxBE4U",
			},
		},
		},
		{
			"peer_blacklisting",
			config.Config{
				ChainID: "TestChain",
				RootDir: tempDir,
				P2P: config.P2PConfig{
					ListenAddress: defaultConfig.P2P.ListenAddress,
					Peers:         "",
					BlockedPeers:  "/ip4/127.0.0.1/tcp/7676/p2p/12D3KooWM1NFkZozoatQi3JvFE57eBaX56mNgBA68Lk5MTPxBE4U",
					AllowedPeers:  "",
				},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.desc, func(t *testing.T) {
			client, err := NewClient(testCase.conf, nodeKey,
				dssync.MutexWrap(datastore.NewMapDatastore()), log.NewTestLogger(t), NopMetrics())
			assert.NoError(err)
			assert.NotNil(client)

			ctx, cancel := context.WithCancel(context.Background())
			err = client.Start(ctx)
			defer func() {
				cancel()
				_ = client.Close()
			}()
			assert.NoError(err)
		})
	}
}

func TestBootstrapping(t *testing.T) {
	assert := assert.New(t)
	logger := log.NewTestLogger(t)

	clients := startTestNetwork(t.Context(), t, 4, map[int]hostDescr{
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

	clients := startTestNetwork(t.Context(), t, 5, map[int]hostDescr{
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
			tempDir := t.TempDir()
			ClientInitFiles(t, tempDir)

			nodeKey, err := key.GenerateNodeKey()
			require.NoError(err)

			client, err := NewClient(
				config.Config{RootDir: tempDir, ChainID: "TestChain"},
				nodeKey,
				dssync.MutexWrap(datastore.NewMapDatastore()),
				logger,
				NopMetrics(),
			)
			require.NoError(err)
			require.NotNil(client)
			actual := client.parseAddrInfoList(c.input)
			assert.NotNil(actual)
			assert.Equal(c.expected, actual)
		})
	}
}

// ClientInitFiles creates the config directory and nodekey file for the client
func ClientInitFiles(t *testing.T, tempDir string) {
	// Create config directory
	configDir := filepath.Join(tempDir, "config")
	err := os.MkdirAll(configDir, 0755) //nolint:gosec
	if err != nil {
		t.Fatalf("failed to create config directory: %v", err)
	}

}

func TestClientInfoMethods(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)
	logger := log.NewTestLogger(t)

	tempDir := t.TempDir()
	ClientInitFiles(t, tempDir)
	conf := config.DefaultConfig
	conf.RootDir = tempDir
	conf.ChainID = "test-chain"

	mn := mocknet.New()
	defer mn.Close()

	var clients []*Client
	var hosts []host.Host
	var err error

	for i := 0; i < 3; i++ {
		nodeKey, e := key.GenerateNodeKey()
		require.NoError(e)
		h, e := mn.AddPeer(nodeKey.PrivKey, multiaddr.StringCast("/ip4/127.0.0.1/tcp/0"))
		require.NoError(e)
		c, e := NewClientWithHost(conf, nodeKey, dssync.MutexWrap(datastore.NewMapDatastore()), logger, NopMetrics(), h)
		require.NoError(e)
		clients = append(clients, c)
		hosts = append(hosts, h)
		defer c.Close()
	}

	client0 := clients[0]
	client1 := clients[1]
	client2 := clients[2]

	// Link all peers in the mocknet
	err = mn.LinkAll()
	require.NoError(err)
	err = mn.ConnectAllButSelf()
	require.NoError(err)

	// Start clients
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for _, c := range clients {
		err = c.Start(ctx)
		require.NoError(err)
	}

	// Wait for connections to establish and DHT to settle
	err = waitForCondition(5*time.Second, func() bool {
		connectedPeers := client0.Peers()
		return len(connectedPeers) == 2
	})
	require.NoError(err)
	t.Run("GetNetworkInfo", func(t *testing.T) {
		netInfo, err := client0.GetNetworkInfo()
		assert.NoError(err)
		assert.Equal(client0.host.ID().String(), netInfo.ID)
		assert.Contains(netInfo.ListenAddress[0], hosts[0].Addrs()[0].String()) // Use h0.Addrs()[0].String()
		assert.ElementsMatch([]peer.ID{client1.host.ID(), client2.host.ID()}, netInfo.ConnectedPeers)
	})

	t.Run("GetPeers", func(t *testing.T) {
		peers, err := client0.GetPeers()
		assert.NoError(err)
		expectedPeerIDs := []peer.ID{client1.host.ID(), client2.host.ID()}
		actualPeerIDs := make([]peer.ID, len(peers))
		for i, p := range peers {
			actualPeerIDs[i] = p.ID
		}
		assert.ElementsMatch(expectedPeerIDs, actualPeerIDs)
	})

	t.Run("Peers", func(t *testing.T) {
		connectedPeers := client0.Peers()
		assert.Len(connectedPeers, 2)

		foundClient1 := false
		foundClient2 := false
		for _, p := range connectedPeers {
			if p.NodeInfo.NodeID == client1.host.ID().String() {
				foundClient1 = true
			} else if p.NodeInfo.NodeID == client2.host.ID().String() {
				foundClient2 = true
			}
			assert.Equal(client0.conf.ListenAddress, p.NodeInfo.ListenAddr)
			assert.Equal(client0.chainID, p.NodeInfo.Network)
			assert.NotEmpty(p.RemoteIP)
		}
		assert.True(foundClient1, "client1 not found in connected peers")
		assert.True(foundClient2, "client2 not found in connected peers")
	})

	t.Run("PeerIDs", func(t *testing.T) {
		peerIDs := client0.PeerIDs()
		assert.ElementsMatch([]peer.ID{client1.host.ID(), client2.host.ID()}, peerIDs)
	})

	t.Run("Info", func(t *testing.T) {
		nodeID, listenAddr, chainID, err := client0.Info()
		assert.NoError(err)
		assert.NotEmpty(nodeID)
		assert.Equal(client0.conf.ListenAddress, listenAddr)
		assert.Equal(client0.chainID, chainID)
	})
}
