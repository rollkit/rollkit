package node

import (
	"fmt"

	"github.com/lazyledger/lazyledger-core/libs/log"
	"github.com/lazyledger/lazyledger-core/libs/service"
	"github.com/lazyledger/lazyledger-core/proxy"
	"github.com/lazyledger/lazyledger-core/types"
)

type Node struct {
	service.BaseService
	eventBus *types.EventBus
	proxyApp proxy.AppConns
}

func NewNode(clientCreator proxy.ClientCreator, logger log.Logger) (*Node, error) {
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

	node := &Node{
		proxyApp: proxyApp,
		eventBus: eventBus,
	}
	node.BaseService = *service.NewBaseService(logger, "Node", node)

	return node, nil
}

func (n *Node) OnStart() error {
	panic("not implemented!")
}

func (n *Node) OnStop() {
	panic("not implemented!")
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
