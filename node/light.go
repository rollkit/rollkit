package node

import (
	"context"
	"errors"
	"fmt"

	"cosmossdk.io/log"
	cmtypes "github.com/cometbft/cometbft/types"
	ds "github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p/core/crypto"

	"github.com/rollkit/rollkit/block"
	"github.com/rollkit/rollkit/config"
	"github.com/rollkit/rollkit/p2p"
	"github.com/rollkit/rollkit/pkg/service"
)

var _ Node = &LightNode{}

// LightNode is a rollup node that only needs the header service
type LightNode struct {
	service.BaseService

	P2P *p2p.Client

	hSyncService *block.HeaderSyncService
}

func newLightNode(
	ctx context.Context,
	conf config.NodeConfig,
	p2pKey crypto.PrivKey,
	genesis *cmtypes.GenesisDoc,
	database ds.Batching,
	metricsProvider MetricsProvider,
	logger log.Logger,
) (ln *LightNode, err error) {

	_, p2pMetrics := metricsProvider(genesis.ChainID)

	client, err := p2p.NewClient(conf.P2P, p2pKey, genesis.ChainID, database, logger.With("module", "p2p"), p2pMetrics)
	if err != nil {
		return nil, err
	}

	headerSyncService, err := block.NewHeaderSyncService(database, conf, genesis, client, logger.With("module", "HeaderSyncService"))
	if err != nil {
		return nil, fmt.Errorf("error while initializing HeaderSyncService: %w", err)
	}

	node := &LightNode{
		P2P:          client,
		hSyncService: headerSyncService,
	}

	node.BaseService = *service.NewBaseService(logger, "LightNode", node)

	return node, nil
}

// OnStart starts the P2P and HeaderSync services
func (ln *LightNode) OnStart(ctx context.Context) error {
	if err := ln.P2P.Start(ctx); err != nil {
		return err
	}

	if err := ln.hSyncService.Start(ctx); err != nil {
		return fmt.Errorf("error while starting header sync service: %w", err)
	}

	return nil
}

// OnStop stops the light node
func (ln *LightNode) OnStop(ctx context.Context) {
	ln.Logger.Info("halting light node...")
	err := ln.P2P.Close()
	err = errors.Join(err, ln.hSyncService.Stop(ctx))
	ln.Logger.Error("errors while stopping node:", "errors", err)
}
