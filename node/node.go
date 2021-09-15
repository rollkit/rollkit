package node

import (
	"context"
	"fmt"

	"github.com/libp2p/go-libp2p-core/crypto"
	abci "github.com/tendermint/tendermint/abci/types"
	llcfg "github.com/tendermint/tendermint/config"
	"github.com/tendermint/tendermint/libs/clist"
	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/libs/service"
	corep2p "github.com/tendermint/tendermint/p2p"
	"github.com/tendermint/tendermint/proxy"
	lltypes "github.com/tendermint/tendermint/types"
	"go.uber.org/multierr"

	"github.com/celestiaorg/optimint/config"
	"github.com/celestiaorg/optimint/da"
	"github.com/celestiaorg/optimint/da/registry"
	"github.com/celestiaorg/optimint/mempool"
	"github.com/celestiaorg/optimint/p2p"
	"github.com/celestiaorg/optimint/store"
)

// prefixes used in KV store to separate main node data from DALC data
var (
	mainPrefix = []byte{0}
	dalcPrefix = []byte{1}
)

// Node represents a client node in Optimint network.
// It connects all the components and orchestrates their work.
type Node struct {
	service.BaseService
	eventBus *lltypes.EventBus
	proxyApp proxy.AppConns

	genesis *lltypes.GenesisDoc

	conf config.NodeConfig
	P2P  *p2p.Client

	// TODO(tzdybal): consider extracting "mempool reactor"
	Mempool      mempool.Mempool
	mempoolIDs   *mempoolIDs
	incomingTxCh chan *p2p.GossipMessage

	Store      store.Store
	aggregator *aggregator
	dalc       da.DataAvailabilityLayerClient

	// keep context here only because of API compatibility
	// - it's used in `OnStart` (defined in service.Service interface)
	ctx context.Context
}

// NewNode creates new Optimint node.
func NewNode(ctx context.Context, conf config.NodeConfig, nodeKey crypto.PrivKey, clientCreator proxy.ClientCreator, genesis *lltypes.GenesisDoc, logger log.Logger) (*Node, error) {
	proxyApp := proxy.NewAppConns(clientCreator)
	proxyApp.SetLogger(logger.With("module", "proxy"))
	if err := proxyApp.Start(); err != nil {
		return nil, fmt.Errorf("error starting proxy app connections: %w", err)
	}

	eventBus := lltypes.NewEventBus()
	eventBus.SetLogger(logger.With("module", "events"))
	if err := eventBus.Start(); err != nil {
		return nil, err
	}

	client, err := p2p.NewClient(conf.P2P, nodeKey, genesis.ChainID, logger.With("module", "p2p"))
	if err != nil {
		return nil, err
	}

	// TODO(tzdybal): change after implementing https://github.com/celestiaorg/optimint/issues/67
	baseKV := store.NewInMemoryKVStore()
	mainKV := store.NewPrefixKV(baseKV, mainPrefix)
	dalcKV := store.NewPrefixKV(baseKV, dalcPrefix)

	store := store.New(mainKV)

	dalc := registry.GetClient(conf.DALayer)
	if dalc == nil {
		return nil, fmt.Errorf("couldn't get data availability client named '%s'", conf.DALayer)
	}
	err = dalc.Init(conf.DAConfig, dalcKV, logger.With("module", "da_client"))
	if err != nil {
		return nil, fmt.Errorf("data availability layer client initialization error: %w", err)
	}

	mp := mempool.NewCListMempool(llcfg.DefaultMempoolConfig(), proxyApp.Mempool(), 0)

	var aggregator *aggregator = nil
	if conf.Aggregator {
		aggregator, err = newAggregator(nodeKey, conf.AggregatorConfig, genesis, store, mp, proxyApp.Consensus(), dalc, logger.With("module", "aggregator"))
		if err != nil {
			return nil, fmt.Errorf("aggregator initialization error: %w", err)
		}
	}

	node := &Node{
		proxyApp:     proxyApp,
		eventBus:     eventBus,
		genesis:      genesis,
		conf:         conf,
		P2P:          client,
		aggregator:   aggregator,
		dalc:         dalc,
		Mempool:      mp,
		mempoolIDs:   newMempoolIDs(),
		incomingTxCh: make(chan *p2p.GossipMessage),
		Store:        store,
		ctx:          ctx,
	}
	node.BaseService = *service.NewBaseService(logger, "Node", node)

	return node, nil
}

func (n *Node) mempoolReadLoop(ctx context.Context) {
	for {
		select {
		case tx := <-n.incomingTxCh:
			n.Logger.Debug("tx received", "from", tx.From, "bytes", len(tx.Data))
			err := n.Mempool.CheckTx(tx.Data, func(resp *abci.Response) {}, mempool.TxInfo{
				SenderID:    n.mempoolIDs.GetForPeer(tx.From),
				SenderP2PID: corep2p.ID(tx.From),
			})
			if err != nil {
				n.Logger.Error("failed to execute CheckTx", "error", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (n *Node) mempoolPublishLoop(ctx context.Context) {
	rawMempool := n.Mempool.(*mempool.CListMempool)
	var next *clist.CElement

	for {
		// wait for transactions
		if next == nil {
			select {
			case <-rawMempool.TxsWaitChan():
				if next = rawMempool.TxsFront(); next != nil {
					continue
				}
			case <-ctx.Done():
				return
			}
		}

		// send transactions
		for {
			memTx := next.Value.(*mempool.MempoolTx)
			tx := memTx.Tx

			err := n.P2P.GossipTx(ctx, tx)
			if err != nil {
				n.Logger.Error("failed to gossip transaction", "error", err)
				continue
			}

			nx := next.Next()
			if nx == nil {
				break
			}
			next = nx
		}

		select {
		case <-next.NextWaitChan():
			next = next.Next()
		case <-ctx.Done():
			return
		}
	}
}

// OnStart is a part of Service interface.
func (n *Node) OnStart() error {
	n.Logger.Info("starting P2P client")
	err := n.P2P.Start(n.ctx)
	if err != nil {
		return fmt.Errorf("error while starting P2P client: %w", err)
	}
	err = n.dalc.Start()
	if err != nil {
		return fmt.Errorf("error while starting data availability layer client: %w", err)
	}
	go n.mempoolReadLoop(n.ctx)
	go n.mempoolPublishLoop(n.ctx)
	if n.conf.Aggregator {
		go n.aggregator.aggregationLoop(n.ctx)
	}
	n.P2P.SetTxHandler(func(tx *p2p.GossipMessage) {
		n.incomingTxCh <- tx
	})

	return nil
}

// OnStop is a part of Service interface.
func (n *Node) OnStop() {
	err := n.dalc.Stop()
	err = multierr.Append(err, n.P2P.Close())
	n.Logger.Error("errors while stopping node:", "errors", err)
}

// OnReset is a part of Service interface.
func (n *Node) OnReset() error {
	panic("OnReset - not implemented!")
}

// SetLogger sets the logger used by node.
func (n *Node) SetLogger(logger log.Logger) {
	n.Logger = logger
}

// GetLogger returns logger.
func (n *Node) GetLogger() log.Logger {
	return n.Logger
}

// EventBus gives access to Node's event bus.
func (n *Node) EventBus() *lltypes.EventBus {
	return n.eventBus
}

// ProxyApp returns ABCI proxy connections to communicate with application.
func (n *Node) ProxyApp() proxy.AppConns {
	return n.proxyApp
}
