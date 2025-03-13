package node

import (
	"context"

	"cosmossdk.io/log"
	cmtypes "github.com/cometbft/cometbft/types"
	ds "github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p/core/crypto"

	"github.com/rollkit/rollkit/config"
	coreexecutor "github.com/rollkit/rollkit/core/execution"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	"github.com/rollkit/rollkit/p2p"
)

// Node is the interface for a rollup node
type Node interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	IsRunning() bool
}

// NewNode returns a new Full or Light Node based on the config
// This is the entry point for composing a node, when compiling a node, you need to provide an executor
// Example executors can be found: TODO: add link
func NewNode(
	ctx context.Context,
	conf config.NodeConfig,
	exec coreexecutor.Executor,
	sequencer coresequencer.Sequencer,
	p2pClient *p2p.Client,
	signingKey crypto.PrivKey,
	genesis *cmtypes.GenesisDoc,
	database ds.Batching,
	metricsProvider MetricsProvider,
	logger log.Logger,
) (Node, error) {
	if conf.Light {
		return newLightNode(
			ctx,
			conf,
			p2pClient,
			genesis,
			database,
			logger,
		)
	}

	return newFullNode(
		ctx,
		conf,
		p2pClient,
		signingKey,
		genesis,
		exec,
		sequencer,
		database,
		metricsProvider,
		logger,
	)
}
