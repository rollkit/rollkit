package node

import (
	"context"

	"github.com/libp2p/go-libp2p/core/crypto"

	"github.com/tendermint/tendermint/libs/log"
	proxy "github.com/tendermint/tendermint/proxy"
	rpcclient "github.com/tendermint/tendermint/rpc/client"
	tmtypes "github.com/tendermint/tendermint/types"

	"github.com/rollkit/rollkit/config"
)

type Node interface {
	Start() error
	GetClient() rpcclient.Client
	Stop() error
	IsRunning() bool
}

// Add Defaults?

func NewNode(
	ctx context.Context,
	conf config.NodeConfig,
	p2pKey crypto.PrivKey,
	signingKey crypto.PrivKey,
	appClient proxy.ClientCreator,
	genesis *tmtypes.GenesisDoc,
	logger log.Logger,
) (Node, error) {
	if !conf.Light {
		return newFullNode(
			ctx,
			conf,
			p2pKey,
			signingKey,
			appClient,
			genesis,
			logger,
		)
	} else {
		return newLightNode(
			ctx,
			conf,
			p2pKey,
			appClient,
			genesis,
			logger,
		)
	}
}
