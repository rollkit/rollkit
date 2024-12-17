package node

import (
	"context"

	"github.com/libp2p/go-libp2p/core/crypto"

	"github.com/cometbft/cometbft/libs/log"
	proxy "github.com/cometbft/cometbft/proxy"
	cmtypes "github.com/cometbft/cometbft/types"

	"github.com/rollkit/rollkit/config"
)

// Node is the interface for a rollup node
type Node interface {
	Start() error
	Stop() error
	IsRunning() bool
	Cancel()
}

// NewNode returns a new Full or Light Node based on the config
func NewNode(
	ctx context.Context,
	conf config.NodeConfig,
	p2pKey crypto.PrivKey,
	signingKey crypto.PrivKey,
	appClient proxy.ClientCreator,
	genesis *cmtypes.GenesisDoc,
	metricsProvider MetricsProvider,
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
			metricsProvider,
			logger,
		)
	}
	return newLightNode(
		ctx,
		conf,
		p2pKey,
		appClient,
		genesis,
		metricsProvider,
		logger,
	)
}
