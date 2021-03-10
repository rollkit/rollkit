package p2p

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lazyledger/optimint/config"
)

// TODO(tzdybal): move to some common place
type TestLogger struct {
	t *testing.T
}

func (t *TestLogger) Debug(msg string, keyvals ...interface{}) {
	t.t.Log(append([]interface{}{"DEBUG: " + msg}, keyvals...)...)
}

func (t *TestLogger) Info(msg string, keyvals ...interface{}) {
	t.t.Log(append([]interface{}{"INFO:  " + msg}, keyvals...)...)
}

func (t *TestLogger) Error(msg string, keyvals ...interface{}) {
	t.t.Log(append([]interface{}{"ERROR: " + msg}, keyvals...)...)
}

func TestClientStartup(t *testing.T) {
	privKey, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	client, err := NewClient(context.Background(), config.P2PConfig{}, privKey, &TestLogger{t})
	assert := assert.New(t)
	assert.NoError(err)
	assert.NotNil(client)

	err = client.Start()
	assert.NoError(err)
}

func TestBootstrapping(t *testing.T) {
	//log.SetLogLevel("dht", "DEBUG")
	log.SetDebugLogging()

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
	client1, err := NewClient(context.Background(), config.P2PConfig{ListenAddress: "/ip4/0.0.0.0/tcp/7676"}, privKey1, logger)
	require.NoError(err)
	require.NotNil(client1)

	// client2 will use client1 as predefined seed
	client2, err := NewClient(context.Background(), config.P2PConfig{
		ListenAddress: "/ip4/127.0.0.1/tcp/7677",
		Seeds:         "/ip4/127.0.0.1/tcp/7676/p2p/" + cid1.String(),
	}, privKey2, logger)
	require.NoError(err)

	// client3 will use clien1 and client2 as seeds
	client3, err := NewClient(context.Background(), config.P2PConfig{
		ListenAddress: "/ip4/127.0.0.1/tcp/7678",
		Seeds:         "/ip4/127.0.0.1/tcp/7676/p2p/" + cid1.String() + ",/ip4/127.0.0.1/tcp/7677/p2p/" + cid2.String(),
	}, privKey3, logger)
	require.NoError(err)

	// client4 will use clien1 as seed
	client4, err := NewClient(context.Background(), config.P2PConfig{
		ListenAddress: "/ip4/127.0.0.1/tcp/7679",
		Seeds:         "/ip4/127.0.0.1/tcp/7676/p2p/" + cid1.String(),
	}, privKey4, logger)
	require.NoError(err)

	err = client1.Start()
	assert.NoError(err)
	time.Sleep(1 * time.Second)

	err = client2.Start()
	assert.NoError(err)

	err = client3.Start()
	assert.NoError(err)

	err = client4.Start()
	assert.NoError(err)

	assert.Equal(3, len(client1.host.Network().Peers()))
	assert.Equal(3, len(client2.host.Network().Peers()))
	assert.Equal(3, len(client3.host.Network().Peers()))
	assert.Equal(3, len(client4.host.Network().Peers()))
}
