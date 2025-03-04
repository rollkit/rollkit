package node

import (
	"context"

	"cosmossdk.io/log"
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
	genesis *config.GenesisDoc,
	metricsProvider MetricsProvider,
	logger log.Logger,
) (Node, error) {
	if conf.Light {
		return newLightNode(
			conf,
			p2pKey,
			genesis,
			metricsProvider,
			logger,
		)
	}

	return newFullNode(
		conf,
		p2pKey,
		signingKey,
		genesis,
		metricsProvider,
		logger,
	)
}
