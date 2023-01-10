package node

import (
	rpcclient "github.com/tendermint/tendermint/rpc/client"
)

type Node interface {
	Start() error
	GetClient() rpcclient.Client
}
