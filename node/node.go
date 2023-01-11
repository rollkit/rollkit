package node

import (
	"context"

	"github.com/libp2p/go-libp2p/core/crypto"

	abciclient "github.com/tendermint/tendermint/abci/client"
	"github.com/tendermint/tendermint/libs/log"
	rpcclient "github.com/tendermint/tendermint/rpc/client"
	tmtypes "github.com/tendermint/tendermint/types"

	"github.com/celestiaorg/rollmint/config"
	"github.com/celestiaorg/rollmint/node/full"
)

type Node interface {
	Start() error
	GetClient() rpcclient.Client
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
		return full.NewFullNode(
			ctx,
			conf,
			p2pKey,
			signingKey,
			appClient,
			genesis,
			logger,
		)
	} else {
		panic("Light node not implemented")
	}
}
