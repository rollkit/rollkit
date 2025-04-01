package node

import (
	"context"

	"cosmossdk.io/log"

	coreda "github.com/rollkit/rollkit/core/da"
	coreexecutor "github.com/rollkit/rollkit/core/execution"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	"github.com/rollkit/rollkit/pkg/config"
	"github.com/rollkit/rollkit/pkg/genesis"
	"github.com/rollkit/rollkit/pkg/p2p/key"
	"github.com/rollkit/rollkit/pkg/service"
	"github.com/rollkit/rollkit/pkg/signer"
)

// Node is the interface for a rollup node
type Node interface {
	service.Service

	IsRunning() bool
}

// NewNode returns a new Full or Light Node based on the config
// This is the entry point for composing a node, when compiling a node, you need to provide an executor
// Example executors can be found in rollups/
func NewNode(
	ctx context.Context,
	conf config.Config,
	exec coreexecutor.Executor,
	sequencer coresequencer.Sequencer,
	dac coreda.Client,
	signer signer.Signer,
	nodeKey key.NodeKey,
	genesis genesis.Genesis,
	metricsProvider MetricsProvider,
	logger log.Logger,
) (Node, error) {
	if conf.Node.Light {
		return newLightNode(conf, genesis, metricsProvider, nodeKey, logger)
	}

	return newFullNode(
		ctx,
		conf,
		signer,
		nodeKey,
		genesis,
		exec,
		sequencer,
		dac,
		metricsProvider,
		logger,
	)
}
