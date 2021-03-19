package node

import (
	"context"
	"fmt"

	"github.com/lazyledger/lazyledger-core/libs/log"
	"github.com/lazyledger/lazyledger-core/libs/service"
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

	conf   config.NodeConfig
	client *p2p.Client

	// keep context here only because of API compatibility
	// - it's used in `OnStart` (defined in service.Service interface)
	ctx context.Context
}

func NewNode(ctx context.Context, conf config.NodeConfig, nodeKey crypto.PrivKey, clientCreator proxy.ClientCreator, logger log.Logger) (*Node, error) {
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

	client, err := p2p.NewClient(conf.P2P, nodeKey, logger.With("module", "p2p"))
	if err != nil {
		return nil, err
	}

	node := &Node{
		proxyApp: proxyApp,
		eventBus: eventBus,
		conf:     conf,
		client:   client,
		ctx:      ctx,
	}
	node.BaseService = *service.NewBaseService(logger, "Node", node)

	return node, nil
}

func (n *Node) OnStart() error {
	n.Logger.Info("starting P2P client")
	err := n.client.Start(n.ctx)
	if err != nil {
		return fmt.Errorf("error while starting P2P client: %w", err)
	}

	return nil
}

func (n *Node) OnStop() {
	n.client.Close()
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
