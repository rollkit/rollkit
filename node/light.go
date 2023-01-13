package node

import (
	"github.com/tendermint/tendermint/libs/service"
	rpcclient "github.com/tendermint/tendermint/rpc/client"
)

var _ Node = &LightNode{}

type LightNode struct {
	service.BaseService
}

func (n *LightNode) GetClient() rpcclient.Client {
	return NewLightClient(n)
}

func newLightNode() (Node, error) {
	return &LightNode{}, nil
}
