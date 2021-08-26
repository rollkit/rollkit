package conv

import (
	"strings"
	"testing"

	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"

	"github.com/celestiaorg/optimint/config"
)

func TestTranslateAddresses(t *testing.T) {
	t.Parallel()

	invalidCosmos := "foobar"
	validCosmos := "127.0.0.1:1234"
	validOptimint := "/ip4/127.0.0.1/tcp/1234"

	cases := []struct {
		name        string
		input       config.NodeConfig
		expected    config.NodeConfig
		expectedErr string
	}{
		{"empty", config.NodeConfig{}, config.NodeConfig{}, ""},
		{
			"valid listen address",
			config.NodeConfig{P2P: config.P2PConfig{ListenAddress: validCosmos}},
			config.NodeConfig{P2P: config.P2PConfig{ListenAddress: validOptimint}},
			"",
		},
		{
			"valid seed address",
			config.NodeConfig{P2P: config.P2PConfig{Seeds: validCosmos + "," + validCosmos}},
			config.NodeConfig{P2P: config.P2PConfig{Seeds: validOptimint + "," + validOptimint}},
			"",
		},
		{
			"invalid listen address",
			config.NodeConfig{P2P: config.P2PConfig{ListenAddress: invalidCosmos}},
			config.NodeConfig{},
			errInvalidAddress.Error(),
		},
		{
			"invalid seed address",
			config.NodeConfig{P2P: config.P2PConfig{Seeds: validCosmos + "," + invalidCosmos}},
			config.NodeConfig{},
			errInvalidAddress.Error(),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert := assert.New(t)
			// c.input is changed in place
			err := TranslateAddresses(&c.input)
			if c.expectedErr != "" {
				assert.Error(err)
				assert.True(strings.HasPrefix(err.Error(), c.expectedErr), "invalid error message")
			} else {
				assert.NoError(err)
				assert.Equal(c.expected, c.input)
			}
		})
	}
}

func TestGetMultiaddr(t *testing.T) {
	t.Parallel()

	valid := mustGetMultiaddr(t, "/ip4/127.0.0.1/tcp/1234")
	withID := mustGetMultiaddr(t, "/ip4/127.0.0.1/tcp/1234/p2p/k2k4r8oqamigqdo6o7hsbfwd45y70oyynp98usk7zmyfrzpqxh1pohl7")
	udpWithID := mustGetMultiaddr(t, "/ip4/127.0.0.1/udp/1234/p2p/k2k4r8oqamigqdo6o7hsbfwd45y70oyynp98usk7zmyfrzpqxh1pohl7")

	cases := []struct {
		name        string
		input       string
		expected    multiaddr.Multiaddr
		expectedErr string
	}{
		{"empty", "", nil, errInvalidAddress.Error()},
		{"no port", "127.0.0.1:", nil, "failed to parse multiaddr"},
		{"ip only", "127.0.0.1", nil, errInvalidAddress.Error()},
		{"with invalid id", "deadbeef@127.0.0.1:1234", nil, "failed to parse multiaddr"},
		{"valid", "127.0.0.1:1234", valid, ""},
		{"valid with id", "k2k4r8oqamigqdo6o7hsbfwd45y70oyynp98usk7zmyfrzpqxh1pohl7@127.0.0.1:1234", withID, ""},
		{"valid with id and proto", "udp://k2k4r8oqamigqdo6o7hsbfwd45y70oyynp98usk7zmyfrzpqxh1pohl7@127.0.0.1:1234", udpWithID, ""},
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
