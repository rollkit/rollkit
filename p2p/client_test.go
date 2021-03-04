package p2p

import (
	"context"
	"crypto/rand"
	"strings"
	"testing"

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
	assert := assert.New(t)
	require := require.New(t)
	logger := &TestLogger{t}

	privKey1, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	privKey2, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	privKey3, _, _ := crypto.GenerateEd25519Key(rand.Reader)

	cid1, err := peer.IDFromPrivateKey(privKey1)
	require.NoError(err)
	require.NotEmpty(cid1)

	cid2, err := peer.IDFromPrivateKey(privKey2)
	require.NoError(err)
	require.NotEmpty(cid2)

	// client1 has no seeds
	client1, err := NewClient(context.Background(), config.P2PConfig{ListenAddress: "127.0.0.1:7676"}, privKey1, logger)
	require.NoError(err)
	require.NotNil(client1)

	// client2 will use client1 as predefined seed
	client2, err := NewClient(context.Background(), config.P2PConfig{
		ListenAddress: "127.0.0.1:7677",
		Seeds:         cid1.Pretty() + "@127.0.0.1:7676",
	}, privKey2, logger)
	require.NoError(err)

	// client3 will use clien1 and client2 as seeds
	client3, err := NewClient(context.Background(), config.P2PConfig{
		ListenAddress: "127.0.0.1:7678",
		Seeds:         cid1.Pretty() + "@127.0.0.1:7676" + "," + cid2.Pretty() + "@127.0.0.1:7677",
	}, privKey3, logger)
	require.NoError(err)

	err = client1.Start()
	assert.NoError(err)

	err = client2.Start()
	assert.NoError(err)

	err = client3.Start()
	assert.NoError(err)

	assert.Equal(2, len(client1.host.Network().Peers()))
	assert.Equal(2, len(client2.host.Network().Peers()))
	assert.Equal(2, len(client3.host.Network().Peers()))
}

func TestGetMultiaddr(t *testing.T) {
	t.Parallel()

	valid := mustGetMultiaddr(t, "/ip4/127.0.0.1/tcp/1234")
	withId := mustGetMultiaddr(t, "/ip4/127.0.0.1/tcp/1234/p2p/k2k4r8oqamigqdo6o7hsbfwd45y70oyynp98usk7zmyfrzpqxh1pohl7")

	cases := []struct {
		name        string
		input       string
		expected    multiaddr.Multiaddr
		expectedErr string
	}{
		{"empty", "", nil, ErrInvalidAddress.Error()},
		{"no port", "127.0.0.1:", nil, "failed to parse multiaddr"},
		{"ip only", "127.0.0.1", nil, ErrInvalidAddress.Error()},
		{"with invalid id", "deadbeef@127.0.0.1:1234", nil, "failed to parse multiaddr"},
		{"valid", "127.0.0.1:1234", valid, ""},
		{"valid with id", "k2k4r8oqamigqdo6o7hsbfwd45y70oyynp98usk7zmyfrzpqxh1pohl7@127.0.0.1:1234", withId, ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert := assert.New(t)
			actual, err := GetMultiAddr(c.input)
			if c.expectedErr != "" {
				assert.Error(err)
				assert.Nil(actual)
				assert.True(strings.HasPrefix(err.Error(), c.expectedErr), "invalid error message")
			} else {
				assert.NoError(err)
				assert.Equal(c.expected, actual)
			}
		})
	}
}

func mustGetMultiaddr(t *testing.T, addr string) multiaddr.Multiaddr {
	t.Helper()
	maddr, err := multiaddr.NewMultiaddr(addr)
	if err != nil {
		t.Fatal(err)
	}
	return maddr
}
