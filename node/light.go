package node

import (
	"github.com/tendermint/tendermint/libs/service"
)

var _ Node = &LightNode{}

type LightNode struct {
	service.BaseService
}

func (n *LightNode) IsRunning() bool {
	panic("Not implemented")
}
