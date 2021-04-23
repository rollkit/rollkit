package node

import (
	"context"
	"fmt"

	abci "github.com/lazyledger/lazyledger-core/abci/types"
	llcfg "github.com/lazyledger/lazyledger-core/config"
	"github.com/lazyledger/lazyledger-core/libs/clist"
	"github.com/lazyledger/lazyledger-core/libs/log"
	"github.com/lazyledger/lazyledger-core/libs/service"
	corep2p "github.com/lazyledger/lazyledger-core/p2p"
	"github.com/lazyledger/lazyledger-core/proxy"
	"github.com/lazyledger/lazyledger-core/types"
	"github.com/libp2p/go-libp2p-core/crypto"

	"github.com/lazyledger/optimint/config"
	"github.com/lazyledger/optimint/mempool"
	"github.com/lazyledger/optimint/p2p"
	"github.com/lazyledger/optimint/store"
)

type Node struct {
	service.BaseService
	eventBus *types.EventBus
	proxyApp proxy.AppConns

	genesis *types.GenesisDoc

	conf config.NodeConfig
	P2P  *p2p.Client

	// TODO(tzdybal): consider extracting "mempool reactor"
	Mempool      mempool.Mempool
	mempoolIDs   *mempoolIDs
	incomingTxCh chan *p2p.Tx

	BlockStore store.BlockStore

	// keep context here only because of API compatibility
	// - it's used in `OnStart` (defined in service.Service interface)
	ctx context.Context
}

func NewNode(ctx context.Context, conf config.NodeConfig, nodeKey crypto.PrivKey, clientCreator proxy.ClientCreator, genesis *types.GenesisDoc, logger log.Logger) (*Node, error) {
	proxyApp := proxy.NewAppConns(clientCreator)
	proxyApp.SetLogger(logger.With("module", "proxy"))
	if err := proxyApp.Start(); err != nil {
		return nil, fmt.Errorf("error starting proxy app connections: %w", err)
	}

	eventBus := types.NewEventBus()
	eventBus.SetLogger(logger.With("module", "events"))
	if err := eventBus.Start(); err != nil {
		return nil, err
	}

	client, err := p2p.NewClient(conf.P2P, nodeKey, genesis.ChainID, logger.With("module", "p2p"))
	if err != nil {
		return nil, err
	}

	mp := mempool.NewCListMempool(llcfg.DefaultMempoolConfig(), proxyApp.Mempool(), 0)

	node := &Node{
		proxyApp:     proxyApp,
		eventBus:     eventBus,
		genesis:      genesis,
		conf:         conf,
		P2P:          client,
		Mempool:      mp,
		mempoolIDs:   newMempoolIDs(),
		incomingTxCh: make(chan *p2p.Tx),
		BlockStore:   store.NewBlockStore(),
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
				Context:     ctx,
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
		n.Logger.Debug("loop begin")
		if next == nil {
			n.Logger.Debug("waiting for mempool")
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
			n.Logger.Debug("Gossiping...")
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

		n.Logger.Debug("waiting for next...")
		select {
		case <-next.NextWaitChan():
			next = next.Next()
		case <-ctx.Done():
			return
		}
	}
}

func (n *Node) OnStart() error {
	n.Logger.Info("starting P2P client")
	err := n.P2P.Start(n.ctx)
	if err != nil {
		return fmt.Errorf("error while starting P2P client: %w", err)
	}
	go n.mempoolReadLoop(n.ctx)
	go n.mempoolPublishLoop(n.ctx)
	n.P2P.SetTxHandler(func(tx *p2p.Tx) {
		n.incomingTxCh <- tx
	})

	return nil
}

func (n *Node) GetGenesis() *types.GenesisDoc {
	return n.genesis
}

func (n *Node) OnStop() {
	n.P2P.Close()
}

func (n *Node) OnReset() error {
	panic("OnReset - not implemented!")
}

func (n *Node) SetLogger(logger log.Logger) {
	n.Logger = logger
}

func (n *Node) GetLogger() log.Logger {
	return n.Logger
}

func (n *Node) EventBus() *types.EventBus {
	return n.eventBus
}

func (n *Node) ProxyApp() proxy.AppConns {
	return n.proxyApp
}
