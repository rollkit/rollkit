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
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/config"
)

func TestClientStartup(t *testing.T) {
	assert := assert.New(t)
	// create temp config dir
	tempDir := t.TempDir()
	ClientInitFiles(t, tempDir)

	testCases := []struct {
		desc string
		conf config.Config
	}{
		{"blank_config", config.Config{
			RootDir: tempDir,
		}},
		{"peer_whitelisting", config.Config{
			RootDir: tempDir,
			P2P: config.P2PConfig{
				ListenAddress: "",
				Seeds:         "",
				BlockedPeers:  "",
				AllowedPeers:  "/ip4/127.0.0.1/tcp/7676/p2p/12D3KooWM1NFkZozoatQi3JvFE57eBaX56mNgBA68Lk5MTPxBE4U",
			},
		},
		},
		{
			"peer_blacklisting",
			config.Config{
				RootDir: tempDir,
				P2P: config.P2PConfig{
					ListenAddress: "",
					Seeds:         "",
					BlockedPeers:  "/ip4/127.0.0.1/tcp/7676/p2p/12D3KooWM1NFkZozoatQi3JvFE57eBaX56mNgBA68Lk5MTPxBE4U",
					AllowedPeers:  "",
				},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.desc, func(t *testing.T) {
			client, err := NewClient(testCase.conf, "TestChain",
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

			client, err := NewClient(
				config.Config{RootDir: tempDir},
				"TestNetwork",
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
	err := os.MkdirAll(configDir, 0755)
	if err != nil {
		t.Fatalf("failed to create config directory: %v", err)
	}

}
