package node

import (
	"context"

	"github.com/libp2p/go-libp2p/core/crypto"

	abciclient "github.com/cometbft/cometbft/abci/client"
	"github.com/cometbft/cometbft/libs/log"
	rpcclient "github.com/cometbft/cometbft/rpc/client"
	tmtypes "github.com/cometbft/cometbft/types"

	"github.com/rollkit/rollkit/config"
)

type Node interface {
	Start() error
	GetClient() rpcclient.Client
	Stop() error
	IsRunning() bool
}

func NewNode(
	ctx context.Context,
	conf config.NodeConfig,
	p2pKey crypto.PrivKey,
	signingKey crypto.PrivKey,
	appClient abciclient.Client,
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
