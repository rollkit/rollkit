package node

import (
	"context"
	"fmt"
	abci "github.com/lazyledger/lazyledger-core/abci/types"
	"github.com/lazyledger/lazyledger-core/libs/log"
	"github.com/lazyledger/lazyledger-core/libs/service"
	tmnode "github.com/lazyledger/lazyledger-core/node"
	"github.com/lazyledger/lazyledger-core/proxy"
	"github.com/lazyledger/lazyledger-core/types"
)

var _ tmnode.NodeInterface = &Node{}

type Node struct {
	service.BaseService
	eventBus *types.EventBus
	proxyApp proxy.AppConns
}

func NewNode(
	clientCreator proxy.ClientCreator,
	logger log.Logger,
) (*Node, error) {
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
	n.Logger.Info("optimint: app is running: ", "running", n.proxyApp.IsRunning())
	response, err := n.proxyApp.Consensus().BeginBlockSync(context.Background(), abci.RequestBeginBlock{
	})
	if response != nil {
		n.Logger.Info("response: ", "resp", response.String())
	}
	return err
}

//func (n *Node) Start() error {
//	panic("implement me")
//}

//func (n *Node) Stop() error {
//	panic("implement me")
//}

//func (n *Node) Reset() error {
//	panic("implement me")
//}

//func (n *Node) IsRunning() bool {
//	panic("implement me")
//}

//func (n *Node) Quit() <-chan struct{} {
//	panic("implement me")
//}

//func (n *Node) String() string {
//	panic("implement me")
//}

func (n *Node) OnStop() {
	panic("implement me")
}


func (n *Node) OnReset() error {
	panic("implement me")
}


func (n *Node) SetLogger(logger log.Logger) {
	n.Logger = logger
}

func (n *Node) ConfigureRPC() error {
	panic("implement me")
}

func (n *Node) GetLogger() log.Logger {
	return n.Logger
}

func (n *Node) EventBus() *types.EventBus {
	panic("implement me")
}
