package node

import (
	"context"
	"fmt"

	abci "github.com/lazyledger/lazyledger-core/abci/types"
	llcfg "github.com/lazyledger/lazyledger-core/config"
	"github.com/lazyledger/lazyledger-core/libs/log"
	"github.com/lazyledger/lazyledger-core/libs/service"
	"github.com/lazyledger/lazyledger-core/mempool"
	corep2p "github.com/lazyledger/lazyledger-core/p2p"
	"github.com/lazyledger/lazyledger-core/proxy"
	"github.com/lazyledger/lazyledger-core/types"
	"github.com/libp2p/go-libp2p-core/crypto"

	"github.com/lazyledger/optimint/config"
	"github.com/lazyledger/optimint/p2p"
)

type Node struct {
	service.BaseService
	eventBus *types.EventBus
	proxyApp proxy.AppConns

	genesis *types.GenesisDoc

	conf config.NodeConfig
	P2P  *p2p.Client

	Mempool       mempool.Mempool
	mempoolIDs    *mempoolIDs
	incommingTxCh chan *p2p.Tx

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
		proxyApp:      proxyApp,
		eventBus:      eventBus,
		genesis:       genesis,
		conf:          conf,
		P2P:           client,
		Mempool:       mp,
		mempoolIDs:    newMempoolIDs(),
		incommingTxCh: make(chan *p2p.Tx),
		ctx:           ctx,
	}
	node.BaseService = *service.NewBaseService(logger, "Node", node)

	return node, nil
}

func (n *Node) mempoolLoop(ctx context.Context) {
	for {
		select {
		case tx := <-n.incommingTxCh:
			n.Mempool.CheckTx(tx.Data, func(resp *abci.Response) {}, mempool.TxInfo{
				SenderID:    n.mempoolIDs.GetForPeer(tx.From),
				SenderP2PID: corep2p.ID(tx.From),
				Context:     ctx,
			})
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
	go n.mempoolLoop(n.ctx)
	n.P2P.SetTxHandler(func(tx *p2p.Tx) {
		n.incommingTxCh <- tx
	})

	return nil
}

func (n *Node) OnStop() {
	n.P2P.Close()
}

func (n *Node) OnReset() error {
	panic("not implemented!")
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
