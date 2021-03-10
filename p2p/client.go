package p2p

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	discovery "github.com/libp2p/go-libp2p-discovery"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/multiformats/go-multiaddr"

	"github.com/lazyledger/optimint/config"
	"github.com/lazyledger/optimint/log"
)

// Client is a P2P client, implemented with libp2p
type Client struct {
	conf    config.P2PConfig
	privKey crypto.PrivKey

	ctx  context.Context
	host host.Host
	dht  *dht.IpfsDHT
	disc *discovery.RoutingDiscovery

	logger log.Logger
}

// NewClient creates new Client object
//
// Basic checks on parameters are done, and default parameters are provided for unset-configuration
func NewClient(ctx context.Context, conf config.P2PConfig, privKey crypto.PrivKey, logger log.Logger) (*Client, error) {
	if privKey == nil {
		return nil, ErrNoPrivKey
	}
	if conf.ListenAddress == "" {
		// TODO(tzdybal): extract const
		conf.ListenAddress = config.DefaultListenAddress
	}
	return &Client{
		ctx:     ctx,
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
	err = c.bootstrap()
	if err != nil {
		return err
	}

	/*
		c.dht, err = dht.New(c.ctx, c.host, dht.Mode(dht.ModeServer))
		if err != nil {
			return err
		}

		err = c.dht.Bootstrap(c.ctx)
		if err != nil {
			return fmt.Errorf("failed to bootstrap DHT routing table: %w", err)
		}
	*/

	return nil
}

func (c *Client) listen() error {
	var err error
	maddr, err := multiaddr.NewMultiaddr(c.conf.ListenAddress)
	if err != nil {
		return err
	}

	c.host, err = libp2p.New(c.ctx,
		libp2p.ListenAddrs(maddr),
		libp2p.Identity(c.privKey),
		/*libp2p.Routing(func(h host.Host) (routing.PeerRouting, error) {
			c.dht, err = dht.New(c.ctx, h, dht.Mode(dht.ModeServer))
			return c.dht, err
		}),*/
	)
	if err != nil {
		return err
	}

	for _, a := range c.host.Addrs() {
		c.logger.Info("listening on", "address", fmt.Sprintf("%s/p2p/%s", a, c.host.ID()))
	}

	return nil
}

func (c *Client) bootstrap() error {
	if len(strings.TrimSpace(c.conf.Seeds)) == 0 {
		c.logger.Info("no seed nodes - only listening for connections")
		return nil
	}
	seeds := strings.Split(c.conf.Seeds, ",")
	for _, s := range seeds {
		maddr, err := multiaddr.NewMultiaddr(s)
		if err != nil {
			c.logger.Error("error while parsing seed node", "address", s, "error", err)
			continue
		}
		c.logger.Debug("seed", "addr", maddr.String())
		// TODO(tzdybal): configuration param for connection timeout
		ctx, cancel := context.WithTimeout(c.ctx, 3*time.Second)
		defer cancel()
		addrInfo, err := peer.AddrInfoFromP2pAddr(maddr)
		if err != nil {
			c.logger.Error("error while creating address info", "error", err)
			continue
		}
		err = c.host.Connect(ctx, *addrInfo)
		if err != nil {
			c.logger.Error("error while connecting to seed node", "error", err)
			continue
		}
		c.logger.Debug("connected to seed node", "address", s)
	}

	return nil
}

func (c *Client) getSeedAddrInfo() []peer.AddrInfo {
	if len(c.conf.Seeds) == 0 {
		return []peer.AddrInfo{}
	}
	seeds := strings.Split(c.conf.Seeds, ",")
	addrs := make([]peer.AddrInfo, len(seeds))
	for _, s := range seeds {
		maddr, err := multiaddr.NewMultiaddr(s)
		if err != nil {
			c.logger.Error("error while parsing seed node", "address", s, "error", err)
			continue
		}
		c.logger.Debug("seed", "addr", maddr.String())
		// TODO(tzdybal): configuration param for connection timeout
		addrInfo, err := peer.AddrInfoFromP2pAddr(maddr)
		if err != nil {
			c.logger.Error("error while creating address info", "error", err)
			continue
		}
		addrs = append(addrs, *addrInfo)
	}
	return addrs
}
