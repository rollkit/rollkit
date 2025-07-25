package p2p

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log/v2"
	libp2p "github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/crypto"
	cdiscovery "github.com/libp2p/go-libp2p/core/discovery"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	discovery "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	discutil "github.com/libp2p/go-libp2p/p2p/discovery/util"
	routedhost "github.com/libp2p/go-libp2p/p2p/host/routed"
	"github.com/libp2p/go-libp2p/p2p/net/conngater"
	"github.com/multiformats/go-multiaddr"

	"github.com/evstack/ev-node/pkg/config"
	rollhash "github.com/evstack/ev-node/pkg/hash"
	"github.com/evstack/ev-node/pkg/p2p/key"
)

// TODO(tzdybal): refactor to configuration parameters
const (
	// reAdvertisePeriod defines a period after which P2P client re-attempt advertising namespace in DHT.
	reAdvertisePeriod = 1 * time.Hour

	// peerLimit defines limit of number of peers returned during active peer discovery.
	peerLimit = 60
)

// Client is a P2P client, implemented with libp2p.
//
// Initially, client connects to predefined seed nodes (aka bootnodes, bootstrap nodes).
// Those seed nodes serve Kademlia DHT protocol, and are agnostic to ORU chain. Using DHT
// peer routing and discovery clients find other peers within ORU network.
type Client struct {
	logger logging.EventLogger

	conf    config.P2PConfig
	chainID string
	privKey crypto.PrivKey

	host  host.Host
	dht   *dht.IpfsDHT
	disc  *discovery.RoutingDiscovery
	gater *conngater.BasicConnectionGater
	ps    *pubsub.PubSub

	metrics *Metrics
}

// NewClient creates new Client object.
//
// Basic checks on parameters are done, and default parameters are provided for unset-configuration
func NewClient(
	conf config.Config,
	nodeKey *key.NodeKey,
	ds datastore.Datastore,
	logger logging.EventLogger,
	metrics *Metrics,
) (*Client, error) {
	if conf.RootDir == "" {
		return nil, fmt.Errorf("rootDir is required")
	}

	gater, err := conngater.NewBasicConnectionGater(ds)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection gater: %w", err)
	}

	if nodeKey == nil {
		return nil, fmt.Errorf("node key is required")
	}

	return &Client{
		conf:    conf.P2P,
		gater:   gater,
		privKey: nodeKey.PrivKey,
		chainID: conf.ChainID,
		logger:  logger,
		metrics: metrics,
	}, nil
}

func NewClientWithHost(
	conf config.Config,
	nodeKey *key.NodeKey,
	ds datastore.Datastore,
	logger logging.EventLogger,
	metrics *Metrics,
	h host.Host, // injected host (mocknet or custom)
) (*Client, error) {
	c, err := NewClient(conf, nodeKey, ds, logger, metrics)
	if err != nil {
		return nil, err
	}

	// Reject hosts whose identity does not match the supplied node key
	expectedID, _ := peer.IDFromPrivateKey(nodeKey.PrivKey)
	if h.ID() != expectedID {
		return nil, fmt.Errorf(
			"injected host ID %s does not match node key ID %s",
			h.ID(),
			expectedID,
		)
	}

	c.host = h
	return c, nil
}

// Start establish Client's P2P connectivity.
//
// Following steps are taken:
// 1. Setup libp2p host, start listening for incoming connections.
// 2. Setup gossibsub.
// 3. Setup DHT, establish connection to seed nodes and initialize peer discovery.
// 4. Use active peer discovery to look for peers from same ORU network.
func (c *Client) Start(ctx context.Context) error {
	c.logger.Debug("starting P2P client")

	if c.host != nil {
		return c.startWithHost(ctx, c.host)
	}

	h, err := c.listen()
	if err != nil {
		return err
	}
	return c.startWithHost(ctx, h)
}

func (c *Client) startWithHost(ctx context.Context, h host.Host) error {
	c.host = h
	for _, a := range c.host.Addrs() {
		c.logger.Info("listening on address ", fmt.Sprintf("%s/p2p/%s", a, c.host.ID()))
	}

	c.logger.Debug("blocking blacklisted peers blacklist ", c.conf.BlockedPeers)
	if err := c.setupBlockedPeers(c.parseAddrInfoList(c.conf.BlockedPeers)); err != nil {
		return err
	}

	c.logger.Debug("allowing whitelisted peers whitelist ", c.conf.AllowedPeers)
	if err := c.setupAllowedPeers(c.parseAddrInfoList(c.conf.AllowedPeers)); err != nil {
		return err
	}

	c.logger.Debug("setting up gossiping")
	if err := c.setupGossiping(ctx); err != nil {
		return err
	}

	c.logger.Debug("setting up DHT")
	if err := c.setupDHT(ctx); err != nil {
		return err
	}

	c.logger.Debug("setting up active peer discovery")
	if err := c.peerDiscovery(ctx); err != nil {
		return err
	}

	return nil
}

// Close gently stops Client.
func (c *Client) Close() error {
	var dhtErr, hostErr error
	if c.dht != nil {
		dhtErr = c.dht.Close()
	}
	if c.host != nil {
		hostErr = c.host.Close()
	}
	return errors.Join(dhtErr, hostErr)
}

// Addrs returns listen addresses of Client.
func (c *Client) Addrs() []multiaddr.Multiaddr {
	return c.host.Addrs()
}

// Host returns the libp2p node in a peer-to-peer network
func (c *Client) Host() host.Host {
	return c.host
}

// PubSub returns the libp2p node pubsub for adding future subscriptions
func (c *Client) PubSub() *pubsub.PubSub {
	return c.ps
}

// ConnectionGater returns the client's connection gater
func (c *Client) ConnectionGater() *conngater.BasicConnectionGater {
	return c.gater
}

// Info returns client ID, ListenAddr, and Network info
func (c *Client) Info() (string, string, string, error) {
	rawKey, err := c.privKey.GetPublic().Raw()
	if err != nil {
		return "", "", "", err
	}
	return hex.EncodeToString(rollhash.SumTruncated(rawKey)), c.conf.ListenAddress, c.chainID, nil
}

// PeerIDs returns list of peer IDs of connected peers excluding self and inactive
func (c *Client) PeerIDs() []peer.ID {
	peerIDs := make([]peer.ID, 0)
	for _, conn := range c.host.Network().Conns() {
		if conn.RemotePeer() != c.host.ID() {
			peerIDs = append(peerIDs, conn.RemotePeer())
		}
	}
	return peerIDs
}

// Peers returns list of peers connected to Client.
func (c *Client) Peers() []PeerConnection {
	conns := c.host.Network().Conns()
	res := make([]PeerConnection, 0, len(conns))
	for _, conn := range conns {
		pc := PeerConnection{
			NodeInfo: NodeInfo{
				ListenAddr: c.conf.ListenAddress,
				Network:    c.chainID,
				NodeID:     conn.RemotePeer().String(),
			},
			IsOutbound: conn.Stat().Direction == network.DirOutbound,
			RemoteIP:   conn.RemoteMultiaddr().String(),
		}
		res = append(res, pc)
	}
	return res
}

func (c *Client) listen() (host.Host, error) {
	maddr, err := multiaddr.NewMultiaddr(c.conf.ListenAddress)
	if err != nil {
		return nil, err
	}

	return libp2p.New(libp2p.ListenAddrs(maddr), libp2p.Identity(c.privKey), libp2p.ConnectionGater(c.gater))
}

func (c *Client) setupDHT(ctx context.Context) error {
	peers := c.parseAddrInfoList(c.conf.Peers)
	if len(peers) == 0 {
		c.logger.Info("no peers - only listening for connections")
	}

	for _, sa := range peers {
		c.logger.Debug("peer", "addr", sa)
	}

	var err error
	c.dht, err = dht.New(ctx, c.host, dht.Mode(dht.ModeServer), dht.BootstrapPeers(peers...))
	if err != nil {
		return fmt.Errorf("failed to create DHT: %w", err)
	}

	err = c.dht.Bootstrap(ctx)
	if err != nil {
		return fmt.Errorf("failed to bootstrap DHT: %w", err)
	}

	c.host = routedhost.Wrap(c.host, c.dht)

	return nil
}

func (c *Client) peerDiscovery(ctx context.Context) error {
	err := c.setupPeerDiscovery(ctx)
	if err != nil {
		return err
	}

	err = c.advertise(ctx)
	if err != nil {
		return err
	}

	err = c.findPeers(ctx)
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) setupPeerDiscovery(ctx context.Context) error {
	// wait for DHT
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.dht.RefreshRoutingTable():
	}
	c.disc = discovery.NewRoutingDiscovery(c.dht)
	return nil
}

func (c *Client) setupBlockedPeers(peers []peer.AddrInfo) error {
	for _, p := range peers {
		if err := c.gater.BlockPeer(p.ID); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) setupAllowedPeers(peers []peer.AddrInfo) error {
	for _, p := range peers {
		if err := c.gater.UnblockPeer(p.ID); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) advertise(ctx context.Context) error {
	discutil.Advertise(ctx, c.disc, c.getNamespace(), cdiscovery.TTL(reAdvertisePeriod))
	return nil
}

func (c *Client) findPeers(ctx context.Context) error {
	peerCh, err := c.disc.FindPeers(ctx, c.getNamespace(), cdiscovery.Limit(peerLimit))
	if err != nil {
		return err
	}

	for peer := range peerCh {
		go c.tryConnect(ctx, peer)
	}

	return nil
}

// tryConnect attempts to connect to a peer and logs error if necessary
func (c *Client) tryConnect(ctx context.Context, peer peer.AddrInfo) {
	if peer.ID == c.host.ID() {
		return
	}

	err := c.host.Connect(ctx, peer)
	if err != nil && ctx.Err() == nil {
		c.logger.Error("failed to connect to peer", "peer", peer, "error", err)
	}
}

func (c *Client) setupGossiping(ctx context.Context) error {
	var err error
	c.ps, err = pubsub.NewGossipSub(ctx, c.host)
	if err != nil {
		return err
	}
	return nil
}

// parseAddrInfoList parses a comma separated string of multiaddrs into a list of peer.AddrInfo structs
func (c *Client) parseAddrInfoList(addrInfoStr string) []peer.AddrInfo {
	if len(addrInfoStr) == 0 {
		return []peer.AddrInfo{}
	}
	peers := strings.Split(addrInfoStr, ",")
	addrs := make([]peer.AddrInfo, 0, len(peers))
	for _, p := range peers {
		maddr, err := multiaddr.NewMultiaddr(p)
		if err != nil {
			c.logger.Error("failed to parse peer, address: ", p, "error: ", err)
			continue
		}
		addrInfo, err := peer.AddrInfoFromP2pAddr(maddr)
		if err != nil {
			c.logger.Error("failed to create addr info for peer, address: ", maddr, "error: ", err)
			continue
		}
		addrs = append(addrs, *addrInfo)
	}
	return addrs
}

// getNamespace returns unique string identifying ORU network.
//
// It is used to advertise/find peers in libp2p DHT.
// For now, chainID is used.
func (c *Client) getNamespace() string {
	return c.chainID
}

func (c *Client) GetPeers() ([]peer.AddrInfo, error) {
	peerCh, err := c.disc.FindPeers(context.Background(), c.getNamespace(), cdiscovery.Limit(peerLimit))
	if err != nil {
		return nil, err
	}

	var peers []peer.AddrInfo
	for p := range peerCh {
		if p.ID == c.host.ID() {
			continue
		}
		peers = append(peers, p)
	}
	return peers, nil
}

func (c *Client) GetNetworkInfo() (NetworkInfo, error) {
	var addrs []string
	for _, a := range c.host.Addrs() {
		addr := fmt.Sprintf("%s/p2p/%s", a, c.host.ID())
		addrs = append(addrs, addr)
	}

	return NetworkInfo{
		ID:             c.host.ID().String(),
		ListenAddress:  addrs,
		ConnectedPeers: c.PeerIDs(),
	}, nil
}
