package node

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"cosmossdk.io/log"
	ds "github.com/ipfs/go-datastore"

	"github.com/rollkit/rollkit/pkg/config"
	"github.com/rollkit/rollkit/pkg/genesis"
	"github.com/rollkit/rollkit/pkg/p2p"
	rpcserver "github.com/rollkit/rollkit/pkg/rpc/server"
	"github.com/rollkit/rollkit/pkg/service"
	"github.com/rollkit/rollkit/pkg/store"
	"github.com/rollkit/rollkit/pkg/sync"
)

var _ Node = &LightNode{}

// LightNode is a chain node that only needs the header service
type LightNode struct {
	service.BaseService

	P2P *p2p.Client

	hSyncService *sync.HeaderSyncService
	Store        store.Store
	rpcServer    *http.Server
	nodeConfig   config.Config
}

func newLightNode(
	conf config.Config,
	genesis genesis.Genesis,
	p2pClient *p2p.Client,
	database ds.Batching,
	logger log.Logger,
) (ln *LightNode, err error) {
	headerSyncService, err := sync.NewHeaderSyncService(database, conf, genesis, p2pClient, logger.With("module", "HeaderSyncService"))
	if err != nil {
		return nil, fmt.Errorf("error while initializing HeaderSyncService: %w", err)
	}

	store := store.NewAsyncPruningStore(database, conf.Node.Pruning)

	node := &LightNode{
		P2P:          p2pClient,
		hSyncService: headerSyncService,
		Store:        store,
		nodeConfig:   conf,
	}

	node.BaseService = *service.NewBaseService(logger, "LightNode", node)

	return node, nil
}

// OnStart starts the P2P and HeaderSync services
func (ln *LightNode) OnStart(ctx context.Context) error {
	// Start RPC server
	handler, err := rpcserver.NewServiceHandler(ln.Store, ln.P2P)
	if err != nil {
		return fmt.Errorf("error creating RPC handler: %w", err)
	}

	ln.rpcServer = &http.Server{
		Addr:         ln.nodeConfig.RPC.Address,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		if err := ln.rpcServer.ListenAndServe(); err != http.ErrServerClosed {
			ln.Logger.Error("RPC server error", "err", err)
		}
	}()

	ln.Logger.Info("Started RPC server", "addr", ln.nodeConfig.RPC.Address)

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

	// Use a timeout context to ensure shutdown doesn't hang
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := ln.P2P.Close()
	err = errors.Join(err, ln.hSyncService.Stop(shutdownCtx))

	if ln.rpcServer != nil {
		err = errors.Join(err, ln.rpcServer.Shutdown(shutdownCtx))
	}

	err = errors.Join(err, ln.Store.Close())
	ln.Logger.Error("errors while stopping node:", "errors", err)
}

// IsRunning returns true if the node is running.
func (ln *LightNode) IsRunning() bool {
	return ln.P2P != nil && ln.hSyncService != nil
}
