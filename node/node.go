package node

import (
	"context"

	"cosmossdk.io/log"
	ds "github.com/ipfs/go-datastore"

	coreda "github.com/rollkit/rollkit/core/da"
	coreexecutor "github.com/rollkit/rollkit/core/execution"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	"github.com/rollkit/rollkit/pkg/config"
	"github.com/rollkit/rollkit/pkg/genesis"
	"github.com/rollkit/rollkit/pkg/p2p"
	"github.com/rollkit/rollkit/pkg/p2p/key"
	"github.com/rollkit/rollkit/pkg/service"
	"github.com/rollkit/rollkit/pkg/signer"
	"github.com/rollkit/rollkit/types"
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
	da coreda.DA,
	signer signer.Signer,
	nodeKey key.NodeKey,
	p2pClient *p2p.Client,
	genesis genesis.Genesis,
	database ds.Batching,
	metricsProvider MetricsProvider,
	logger log.Logger,
	validatorHasher types.ValidatorHasher,
	signaturePayloadProvider types.SignaturePayloadProvider,
	headerHasher types.HeaderHasher,
	commitHashProvider types.CommitHashProvider,
) (Node, error) {
	if conf.Node.Light {
		return newLightNode(conf, genesis, p2pClient, nodeKey, database, logger)
	}

	return newFullNode(
		ctx,
		conf,
		p2pClient,
		signer,
		nodeKey,
		genesis,
		database,
		exec,
		sequencer,
		da,
		metricsProvider,
		logger,
		validatorHasher,
		headerHasher,
		commitHashProvider,
		signaturePayloadProvider,
	)
}
