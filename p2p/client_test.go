package p2p

import (
	"context"
	"crypto/rand"
	"sync"
	"testing"
	"time"

	"github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/celestiaorg/optimint/config"
	"github.com/celestiaorg/optimint/log/test"
)

func TestClientStartup(t *testing.T) {
	privKey, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	client, err := NewClient(config.P2PConfig{}, privKey, "TestChain", &test.TestLogger{T: t})
	assert := assert.New(t)
	assert.NoError(err)
	assert.NotNil(client)

	err = client.Start(context.Background())
	defer client.Close()
	assert.NoError(err)
}

func TestBootstrapping(t *testing.T) {
	_ = log.SetLogLevel("dht", "INFO")
	//log.SetDebugLogging()

	assert := assert.New(t)
	logger := &test.TestLogger{T: t}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	clients := startTestNetwork(ctx, t, 4, map[int]hostDescr{
		1: {conns: []int{0}},
		2: {conns: []int{0, 1}},
		3: {conns: []int{0}},
	}, make([]GossipValidator, 4), logger)

	// wait for clients to finish refreshing routing tables
	clients.WaitForDHT()

	for _, client := range clients {
		assert.Equal(3, len(client.host.Network().Peers()))
	}
}

func TestDiscovery(t *testing.T) {
	assert := assert.New(t)
	logger := &test.TestLogger{T: t}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	clients := startTestNetwork(ctx, t, 5, map[int]hostDescr{
		1: {conns: []int{0}, chainID: "ORU2"},
		2: {conns: []int{0}, chainID: "ORU2"},
		3: {conns: []int{1}, chainID: "ORU1"},
		4: {conns: []int{2}, chainID: "ORU1"},
	}, make([]GossipValidator, 5), logger)

	// wait for clients to finish refreshing routing tables
	clients.WaitForDHT()

	assert.Contains(clients[3].host.Network().Peers(), clients[4].host.ID())
	assert.Contains(clients[4].host.Network().Peers(), clients[3].host.ID())
}

func TestGossiping(t *testing.T) {
	assert := assert.New(t)
	logger := &test.TestLogger{T: t}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var expectedMsg = []byte("foobar")
	var wg sync.WaitGroup

	wg.Add(3)

	// ensure that Tx is delivered to client
	assertRecv := func(tx *GossipMessage) bool {
		logger.Debug("received tx", "body", string(tx.Data), "from", tx.From)
		assert.Equal(expectedMsg, tx.Data)
		wg.Done()
		return true
	}

	// ensure that Tx is not delivered to client
	assertNotRecv := func(*GossipMessage) bool {
		t.Fatal("unexpected Tx received")
		return false
	}

	validators := []GossipValidator{assertRecv, assertNotRecv, assertNotRecv, assertRecv, assertRecv}

	// network connections topology: 3<->1<->0<->2<->4
	clients := startTestNetwork(ctx, t, 5, map[int]hostDescr{
		0: {conns: []int{}, chainID: "2"},
		1: {conns: []int{0}, chainID: "1", realKey: true},
		2: {conns: []int{0}, chainID: "1", realKey: true},
		3: {conns: []int{1}, chainID: "2", realKey: true},
		4: {conns: []int{2}, chainID: "2", realKey: true},
	}, validators, logger)

	// wait for clients to finish refreshing routing tables
	clients.WaitForDHT()

	// this sleep is required for pubsub to "propagate" subscription information
	// TODO(tzdybal): is there a better way to wait for readiness?
	time.Sleep(1 * time.Second)

	// gossip from client 4
	err := clients[4].GossipTx(ctx, expectedMsg)
	assert.NoError(err)

	// wait for clients that should receive Tx
	wg.Wait()
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
			logger := &test.MockLogger{}
			client, err := NewClient(config.P2PConfig{}, privKey, "TestNetwork", logger)
			require.NoError(err)
			require.NotNil(client)
			actual := client.getSeedAddrInfo(c.input)
			assert.NotNil(actual)
			assert.Equal(c.expected, actual)
			// ensure that errors are logged
			assert.Equal(c.nErrors, len(logger.ErrLines))
		})
	}
}
