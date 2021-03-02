package p2p

import (
	"context"
	"errors"
	"strings"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"

	"github.com/lazyledger/optimint/config"
	"github.com/lazyledger/optimint/log"
	"github.com/multiformats/go-multiaddr"
)

// Client is a P2P client, implemented with libp2p
type Client struct {
	conf    config.P2PConfig
	privKey crypto.PrivKey

	host host.Host

	logger log.Logger
}

// NewClient creates new Client object
//
// Basic checks on parameters are done, and default parameters are provided for unset-configuration
func NewClient(conf config.P2PConfig, privKey crypto.PrivKey, logger log.Logger) (*Client, error) {
	if privKey == nil {
		// TODO(tzdybal): extract sentinel error
		return nil, errors.New("private key not provided")
	}
	if conf.ListenAddress == "" {
		// TODO(tzdybal): extract const
		conf.ListenAddress = "0.0.0.0:7676"
	}
	return &Client{
		conf:    conf,
		privKey: privKey,
		logger:  logger,
	}, nil
}

func (c *Client) Start() error {
	c.logger.Debug("Starting P2P client")
	err := c.listen()
	if err != nil {
		return err
	}

	// start bootstrapping connections

	return nil
}

func (c *Client) listen() error {
	maddr, err := GetMultiAddr(c.conf.ListenAddress)
	if err != nil {
		return err
	}

	//TODO(tzdybal): think about per-client context
	host, err := libp2p.New(context.Background(), libp2p.ListenAddrs(maddr), libp2p.Identity(c.privKey))
	if err != nil {
		return err
	}
	for _, a := range host.Addrs() {
		c.logger.Info("listening on", "address", a)
	}

	c.host = host
	return nil
}

func GetMultiAddr(addr string) (multiaddr.Multiaddr, error) {
	parts := strings.Split(addr, ":")
	if len(parts) != 2 {
		// TODO(tzdybal): extract sentinel error
		return nil, errors.New("invalid address format, expected <IPv4>:<PORT>")
	}
	return multiaddr.NewMultiaddr("/ip4/" + parts[0] + "/tcp/" + parts[1])
}
