package node

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/rollkit/rollkit/pkg/log"
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

	running bool
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

	store := store.New(database)

	node := &LightNode{
		P2P:          p2pClient,
		hSyncService: headerSyncService,
		Store:        store,
		nodeConfig:   conf,
	}

	node.BaseService = *service.NewBaseService(logger, "LightNode", node)

	return node, nil
}

// IsRunning returns true if the node is running.
func (ln *LightNode) IsRunning() bool {
	return ln.running
}

// Run implements the Service interface.
// It starts all subservices and manages the node's lifecycle.
func (ln *LightNode) Run(parentCtx context.Context) error {
	ctx, cancelNode := context.WithCancel(parentCtx)
	defer func() {
		ln.running = false
		cancelNode()
	}()

	ln.running = true
	// Start RPC server
	handler, err := rpcserver.NewServiceHandler(ln.Store, ln.P2P, ln.Logger)
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
		ln.Logger.Info("started RPC server", "addr", ln.nodeConfig.RPC.Address)
		if err := ln.rpcServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			ln.Logger.Error("RPC server error", "err", err)
		}
	}()

	if err := ln.P2P.Start(ctx); err != nil {
		return fmt.Errorf("error while starting P2P client: %w", err)
	}

	if err := ln.hSyncService.Start(ctx); err != nil {
		return fmt.Errorf("error while starting header sync service: %w", err)
	}

	<-parentCtx.Done()
	ln.Logger.Info("context canceled, stopping node")
	cancelNode()

	ln.Logger.Info("halting light node and its sub services...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var multiErr error

	// Stop Header Sync Service
	err = ln.hSyncService.Stop(shutdownCtx)
	if err != nil {
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			multiErr = errors.Join(multiErr, fmt.Errorf("stopping header sync service: %w", err))
		} else {
			ln.Logger.Debug("header sync service stop context ended", "reason", err)
		}
	}

	// Shutdown RPC Server
	if ln.rpcServer != nil {
		err = ln.rpcServer.Shutdown(shutdownCtx)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			multiErr = errors.Join(multiErr, fmt.Errorf("shutting down RPC server: %w", err))
		} else {
			ln.Logger.Debug("RPC server shutdown context ended", "reason", err)
		}
	}

	// Stop P2P Client
	err = ln.P2P.Close()
	if err != nil {
		multiErr = errors.Join(multiErr, fmt.Errorf("closing P2P client: %w", err))
	}

	if err = ln.Store.Close(); err != nil {
		multiErr = errors.Join(multiErr, fmt.Errorf("closing store: %w", err))
	} else {
		ln.Logger.Debug("store closed")
	}

	if multiErr != nil {
		if unwrapper, ok := multiErr.(interface{ Unwrap() []error }); ok {
			for _, err := range unwrapper.Unwrap() {
				ln.Logger.Error("error during shutdown", "error", err)
			}
		} else {
			ln.Logger.Error("error during shutdown", "error", multiErr)
		}
		ln.Logger.Error("error during shutdown", "error", err)
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	return multiErr
}
