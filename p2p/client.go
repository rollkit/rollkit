package p2p

import (
	"context"
	"fmt"
	"strings"

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

// Client is a P2P client, implemented with libp2p.
//
// Initially, client connects to predefined seed nodes (aka bootnodes, bootstrap nodes).
// Those seed nodes serve Kademlia DHT protocol, and are agnostic to ORU chain. Using DHT
// peer routing and discovery clients find other peers within ORU network.
type Client struct {
	conf    config.P2PConfig
	privKey crypto.PrivKey

	ctx  context.Context
	host host.Host
	dht  *dht.IpfsDHT
	disc *discovery.RoutingDiscovery

	logger log.Logger
}

// NewClient creates new Client object.
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
	c.logger.Debug("starting P2P client")
	err := c.listen()
	if err != nil {
		return err
	}

	c.logger.Debug("setting up DHT")
	err = c.setupDHT()
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) Close() error {
	dhtErr := c.dht.Close()
	if dhtErr != nil {
		c.logger.Error("failed to close DHT", "error", dhtErr)
	}
	err := c.host.Close()
	if err != nil {
		c.logger.Error("failed to close P2P host", "error", err)
		return err
	}
	return dhtErr
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
	)
	if err != nil {
		return err
	}

	for _, a := range c.host.Addrs() {
		c.logger.Info("listening on", "address", fmt.Sprintf("%s/p2p/%s", a, c.host.ID()))
	}

	return nil
}

func (c *Client) setupDHT() error {
	seedNodes := c.getSeedAddrInfo(c.conf.Seeds)
	if len(seedNodes) == 0 {
		c.logger.Info("no seed nodes - only listening for connections")
	}

	for _, sa := range seedNodes {
		c.logger.Debug("seed node", "addr", sa)
	}

	var err error
	c.dht, err = dht.New(c.ctx, c.host, dht.Mode(dht.ModeServer), dht.BootstrapPeers(seedNodes...))
	if err != nil {
		return fmt.Errorf("failed to create DHT: %w", err)
	}

	err = c.dht.Bootstrap(c.ctx)
	if err != nil {
		return fmt.Errorf("failed to bootstrap DHT: %w", err)
	}
	return nil
}

func (c *Client) getSeedAddrInfo(seedStr string) []peer.AddrInfo {
	if len(seedStr) == 0 {
		return []peer.AddrInfo{}
	}
	seeds := strings.Split(seedStr, ",")
	addrs := make([]peer.AddrInfo, 0, len(seeds))
	for _, s := range seeds {
		maddr, err := multiaddr.NewMultiaddr(s)
		if err != nil {
			c.logger.Error("error while parsing seed node", "address", s, "error", err)
			continue
		}
		c.logger.Debug("seed", "addr", maddr.String())
		addrInfo, err := peer.AddrInfoFromP2pAddr(maddr)
		if err != nil {
			c.logger.Error("error while creating address info", "error", err)
			continue
		}
		addrs = append(addrs, *addrInfo)
	}
	return addrs
}
