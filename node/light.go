package node

import (
	"github.com/tendermint/tendermint/libs/service"
)

var _ Node = &LightNode{}

type LightNode struct {
	service.BaseService
}

func newLightNode() (Node, error) {
	return &LightNode{}, nil
}

func (n *LightNode) IsRunning() bool {
	panic("Not implemented")
}
