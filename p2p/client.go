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

	"github.com/lazyledger/optimint/config"
	"github.com/lazyledger/optimint/log"
	"github.com/multiformats/go-multiaddr"
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
		conf.ListenAddress = "0.0.0.0:7676"
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

	c.dht, err = dht.New(c.ctx, c.host, dht.Mode(dht.ModeServer))
	if err != nil {
		return err
	}

	err = c.dht.Bootstrap(c.ctx)
	if err != nil {
		return fmt.Errorf("failed to bootstrap DHT routing table: %w", err)
	}

	c.disc = discovery.NewRoutingDiscovery(c.dht)
	// TODO(tzdybal) - find cosmos-sdk config for number of peers limit
	// TODO(tzdybal) - extract ns as const or config
	discovery.Advertise(c.ctx, c.disc, "ORU")

	peerCh, err := c.disc.FindPeers(c.ctx, "ORU")
	if err != nil {
		return err
	}
	for peer := range peerCh {
		if peer.ID == c.host.ID() {
			continue
		}
		fmt.Println("tzdybal:", peer)
	}

	return nil
}

func (c *Client) listen() error {
	// TODO(tzdybal): consider requiring listen address in multiaddress format
	maddr, err := GetMultiAddr(c.conf.ListenAddress)
	if err != nil {
		return err
	}

	host, err := libp2p.New(c.ctx,
		libp2p.ListenAddrs(maddr),
		libp2p.Identity(c.privKey),
	)
	if err != nil {
		return err
	}
	c.host = host

	for _, a := range host.Addrs() {
		c.logger.Info("listening on", "address", a, "ID", c.host.ID())
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
		maddr, err := GetMultiAddr(s)
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
	seeds := strings.Split(c.conf.Seeds, ",")
	addrs := make([]peer.AddrInfo, len(seeds))
	for _, s := range seeds {
		maddr, err := GetMultiAddr(s)
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
