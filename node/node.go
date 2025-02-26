package node

import (
	"context"

	"cosmossdk.io/log"
	cmtypes "github.com/cometbft/cometbft/types"
	"github.com/libp2p/go-libp2p/core/crypto"

	"github.com/rollkit/rollkit/config"
)

// Node is the interface for a rollup node
type Node interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	IsRunning() bool
}

// NewNode returns a new Full or Light Node based on the config
func NewNode(
	ctx context.Context,
	conf config.NodeConfig,
	p2pKey crypto.PrivKey,
	signingKey crypto.PrivKey,
	genesis *cmtypes.GenesisDoc,
	metricsProvider MetricsProvider,
	logger log.Logger,
) (Node, error) {
	if conf.Light {
		return newLightNode(
			ctx,
			conf,
			p2pKey,
			genesis,
			metricsProvider,
			logger,
		)
	}

	return newFullNode(
		ctx,
		conf,
		p2pKey,
		signingKey,
		genesis,
		metricsProvider,
		logger,
	)
}
