package p2p

import (
	"context"
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
		return nil, ErrNoPrivKey
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
	// TODO(tzdybal): consider requiring listen address in multiaddress format
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

func (c *Client) bootstrap() error {
	seeds := strings.Split(c.conf.Seeds, ",")
	for _, s := range seeds {
		maddr, err := GetMultiAddr(s)
		if err != nil {
			c.logger.Error("error while parsing seed node", "address", s, "error", err)
			continue
		}
		c.logger.Debug("seed", "addr", maddr.String())
	}

	return nil
}

func GetMultiAddr(addr string) (multiaddr.Multiaddr, error) {
	var err error
	var p2pId multiaddr.Multiaddr
	if at := strings.IndexRune(addr, '@'); at != -1 {
		p2pId, err = multiaddr.NewMultiaddr("/p2p/" + addr[:at])
		if err != nil {
			return nil, err
		}
		addr = addr[at+1:]
	}
	parts := strings.Split(addr, ":")
	if len(parts) != 2 {
		return nil, ErrInvalidAddress
	}
	maddr, err := multiaddr.NewMultiaddr("/ip4/" + parts[0] + "/tcp/" + parts[1])
	if err != nil {
		return nil, err
	}
	if p2pId != nil {
		maddr = maddr.Encapsulate(p2pId)
	}
	return maddr, nil
}
