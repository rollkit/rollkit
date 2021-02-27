package node

import (
	"fmt"

	"github.com/lazyledger/lazyledger-core/libs/log"
	"github.com/lazyledger/lazyledger-core/libs/service"
	"github.com/lazyledger/lazyledger-core/p2p"
	"github.com/lazyledger/lazyledger-core/proxy"
	"github.com/lazyledger/lazyledger-core/types"
	"github.com/libp2p/go-libp2p-core/crypto"
)

type Node struct {
	service.BaseService
	eventBus *types.EventBus
	proxyApp proxy.AppConns

	privKey crypto.PrivKey
}

func NewNode(nodeKey p2p.NodeKey, clientCreator proxy.ClientCreator, logger log.Logger) (*Node, error) {
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
	err := node.loadPrivateKey(nodeKey)
	if err != nil {
		return nil, err
	}

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

func (n *Node) loadPrivateKey(nodeKey p2p.NodeKey) error {
	switch nodeKey.PrivKey.Type() {
	case "ed25519":
		privKey, err := crypto.UnmarshalEd25519PrivateKey(nodeKey.PrivKey.Bytes())
		if err != nil {
			return fmt.Errorf("node private key unmarshaling error: %w", err)
		}
		n.privKey = privKey
	default:
		return fmt.Errorf("unsupported type of node private key: %s", nodeKey.PrivKey.Type())
	}
	return nil
}
