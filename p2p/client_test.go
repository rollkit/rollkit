package p2p

import (
	"context"
	"crypto/rand"
	"fmt"
	"testing"

	"github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lazyledger/optimint/config"
)

// TODO(tzdybal): move to some common place
type TestLogger struct {
	t *testing.T
}

func (t *TestLogger) Debug(msg string, keyvals ...interface{}) {
	t.t.Helper()
	t.t.Log(append([]interface{}{"DEBUG: " + msg}, keyvals...)...)
}

func (t *TestLogger) Info(msg string, keyvals ...interface{}) {
	t.t.Helper()
	t.t.Log(append([]interface{}{"INFO:  " + msg}, keyvals...)...)
}

func (t *TestLogger) Error(msg string, keyvals ...interface{}) {
	t.t.Helper()
	t.t.Log(append([]interface{}{"ERROR: " + msg}, keyvals...)...)
}

type MockLogger struct {
	debug, info, err []string
}

func (t *MockLogger) Debug(msg string, keyvals ...interface{}) {
	t.debug = append(t.debug, fmt.Sprint(append([]interface{}{msg}, keyvals...)...))
}

func (t *MockLogger) Info(msg string, keyvals ...interface{}) {
	t.info = append(t.info, fmt.Sprint(append([]interface{}{msg}, keyvals...)...))
}

func (t *MockLogger) Error(msg string, keyvals ...interface{}) {
	t.err = append(t.err, fmt.Sprint(append([]interface{}{msg}, keyvals...)...))
}

func TestClientStartup(t *testing.T) {
	privKey, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	client, err := NewClient(config.P2PConfig{}, privKey, &TestLogger{t})
	assert := assert.New(t)
	assert.NoError(err)
	assert.NotNil(client)

	err = client.Start(context.Background())
	defer client.Close()
	assert.NoError(err)

}

func TestBootstrapping(t *testing.T) {
	log.SetLogLevel("dht", "INFO")
	//log.SetDebugLogging()

	assert := assert.New(t)
	require := require.New(t)
	logger := &TestLogger{t}

	privKey1, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	privKey2, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	privKey3, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	privKey4, _, _ := crypto.GenerateEd25519Key(rand.Reader)

	cid1, err := peer.IDFromPrivateKey(privKey1)
	require.NoError(err)
	require.NotEmpty(cid1)

	cid2, err := peer.IDFromPrivateKey(privKey2)
	require.NoError(err)
	require.NotEmpty(cid2)

	// client1 has no seeds
	client1, err := NewClient(config.P2PConfig{ListenAddress: "/ip4/0.0.0.0/tcp/7676"}, privKey1, logger)
	require.NoError(err)
	require.NotNil(client1)

	// client2 will use client1 as predefined seed
	client2, err := NewClient(config.P2PConfig{
		ListenAddress: "/ip4/127.0.0.1/tcp/7677",
		Seeds:         "/ip4/127.0.0.1/tcp/7676/p2p/" + cid1.String(),
	}, privKey2, logger)
	require.NoError(err)

	// client3 will use clien1 and client2 as seeds
	client3, err := NewClient(config.P2PConfig{
		ListenAddress: "/ip4/127.0.0.1/tcp/7678",
		Seeds:         "/ip4/127.0.0.1/tcp/7676/p2p/" + cid1.String() + ",/ip4/127.0.0.1/tcp/7677/p2p/" + cid2.String(),
	}, privKey3, logger)
	require.NoError(err)

	// client4 will use clien1 as seed
	client4, err := NewClient(config.P2PConfig{
		ListenAddress: "/ip4/127.0.0.1/tcp/7679",
		Seeds:         "/ip4/127.0.0.1/tcp/7676/p2p/" + cid1.String(),
	}, privKey4, logger)
	require.NoError(err)

	err = client1.Start(context.Background())
	defer client1.Close()
	assert.NoError(err)

	err = client2.Start(context.Background())
	defer client2.Close()
	assert.NoError(err)

	err = client3.Start(context.Background())
	defer client3.Close()
	assert.NoError(err)

	err = client4.Start(context.Background())
	defer client4.Close()
	assert.NoError(err)

	// wait for clients to finish refreshing routing tables
	<-client1.dht.RefreshRoutingTable()
	<-client2.dht.RefreshRoutingTable()
	<-client3.dht.RefreshRoutingTable()
	<-client4.dht.RefreshRoutingTable()

	assert.Equal(3, len(client1.host.Network().Peers()))
	assert.Equal(3, len(client2.host.Network().Peers()))
	assert.Equal(3, len(client3.host.Network().Peers()))
	assert.Equal(3, len(client4.host.Network().Peers()))
}

func TestDiscovery(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	logger := &TestLogger{t}

	// TODO: create helper function to create "connected" network
	privKey1, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	privKey2, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	privKey3, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	privKey4, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	privKey5, _, _ := crypto.GenerateEd25519Key(rand.Reader)

	cid1, err := peer.IDFromPrivateKey(privKey1)
	require.NoError(err)
	require.NotEmpty(cid1)

	cid2, err := peer.IDFromPrivateKey(privKey2)
	require.NoError(err)
	require.NotEmpty(cid2)

	cid3, err := peer.IDFromPrivateKey(privKey3)
	require.NoError(err)
	require.NotEmpty(cid2)

	// client1 has no seeds
	client1, err := NewClient(config.P2PConfig{ListenAddress: "/ip4/0.0.0.0/tcp/7676"}, privKey1, logger)
	require.NoError(err)
	require.NotNil(client1)

	// client2 will use client1 as predefined seed
	client2, err := NewClient(config.P2PConfig{
		ListenAddress: "/ip4/127.0.0.1/tcp/7677",
		Seeds:         "/ip4/127.0.0.1/tcp/7676/p2p/" + cid1.String(),
	}, privKey2, logger)
	require.NoError(err)

	// client3 will use clien1 as predefined seed
	client3, err := NewClient(config.P2PConfig{
		ListenAddress: "/ip4/127.0.0.1/tcp/7678",
		Seeds:         "/ip4/127.0.0.1/tcp/7676/p2p/" + cid1.String(),
	}, privKey3, logger)
	require.NoError(err)

	// client4 will use clien2 as seed
	client4, err := NewClient(config.P2PConfig{
		ListenAddress: "/ip4/127.0.0.1/tcp/7679",
		Seeds:         "/ip4/127.0.0.1/tcp/7677/p2p/" + cid2.String(),
	}, privKey4, logger)
	require.NoError(err)

	// client5 will use clien2 as seed
	client5, err := NewClient(config.P2PConfig{
		ListenAddress: "/ip4/127.0.0.1/tcp/7680",
		Seeds:         "/ip4/127.0.0.1/tcp/7678/p2p/" + cid3.String(),
	}, privKey5, logger)
	require.NoError(err)

	client2.chainID = "ORU2"
	client3.chainID = "ORU2"
	client4.chainID = "ORU1"
	client5.chainID = "ORU1"

	err = client1.Start(context.Background())
	defer client1.Close()
	assert.NoError(err)

	err = client2.Start(context.Background())
	defer client2.Close()
	assert.NoError(err)

	err = client3.Start(context.Background())
	defer client3.Close()
	assert.NoError(err)

	err = client4.Start(context.Background())
	defer client4.Close()
	assert.NoError(err)

	err = client4.Start(context.Background())
	defer client4.Close()
	assert.NoError(err)

	err = client5.Start(context.Background())
	defer client5.Close()
	assert.NoError(err)

	// wait for clients to finish refreshing routing tables
	<-client1.dht.RefreshRoutingTable()
	<-client2.dht.RefreshRoutingTable()
	<-client3.dht.RefreshRoutingTable()
	<-client4.dht.RefreshRoutingTable()
	<-client5.dht.RefreshRoutingTable()

	assert.Contains(client4.host.Network().Peers(), client5.host.ID())
	assert.Contains(client5.host.Network().Peers(), client4.host.ID())
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
			logger := &MockLogger{}
			client, err := NewClient(config.P2PConfig{}, privKey, logger)
			require.NoError(err)
			require.NotNil(client)
			actual := client.getSeedAddrInfo(c.input)
			assert.NotNil(actual)
			assert.Equal(c.expected, actual)
			// ensure that errors are logged
			assert.Equal(c.nErrors, len(logger.err))
		})
	}
}
