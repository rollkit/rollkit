package p2p

import (
	"crypto/rand"
	"strings"
	"testing"

	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"

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
	client, err := NewClient(config.P2PConfig{
		Seeds: "127.0.0.1:7677",
	}, privKey, &TestLogger{t})
	assert := assert.New(t)
	assert.NoError(err)
	assert.NotNil(client)

	err = client.Start()
	assert.NoError(err)
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
