package node

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/pprof"
	"time"

	"cosmossdk.io/log"
	ds "github.com/ipfs/go-datastore"
	ktds "github.com/ipfs/go-datastore/keytransform"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/rollkit/rollkit/block"
	coreda "github.com/rollkit/rollkit/core/da"
	coreexecutor "github.com/rollkit/rollkit/core/execution"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	"github.com/rollkit/rollkit/pkg/config"
	genesispkg "github.com/rollkit/rollkit/pkg/genesis"
	"github.com/rollkit/rollkit/pkg/p2p"
	rpcserver "github.com/rollkit/rollkit/pkg/rpc/server"
	"github.com/rollkit/rollkit/pkg/service"
	"github.com/rollkit/rollkit/pkg/signer"
	"github.com/rollkit/rollkit/pkg/store"
	"github.com/rollkit/rollkit/pkg/sync"
)

// prefixes used in KV store to separate rollkit data from execution environment data (if the same data base is reused)
var RollkitPrefix = "0"

const (
	// genesisChunkSize is the maximum size, in bytes, of each
	// chunk in the genesis structure for the chunked API
	genesisChunkSize = 16 * 1024 * 1024 // 16 MiB
)

var _ Node = &FullNode{}

// FullNode represents a client node in Rollkit network.
// It connects all the components and orchestrates their work.
type FullNode struct {
	service.BaseService

	genesis genesispkg.Genesis
	// cache of chunked genesis data.
	genChunks []string

	nodeConfig config.Config

	da coreda.DA

	p2pClient    *p2p.Client
	hSyncService *sync.HeaderSyncService
	dSyncService *sync.DataSyncService
	Store        store.Store
	blockManager *block.Manager
	reaper       *block.Reaper

	prometheusSrv *http.Server
	pprofSrv      *http.Server
	rpcServer     *http.Server
}

// newFullNode creates a new Rollkit full node.
func newFullNode(
	ctx context.Context,
	nodeConfig config.Config,
	p2pClient *p2p.Client,
	signer signer.Signer,
	genesis genesispkg.Genesis,
	database ds.Batching,
	exec coreexecutor.Executor,
	sequencer coresequencer.Sequencer,
	da coreda.DA,
	metricsProvider MetricsProvider,
	logger log.Logger,
) (fn *FullNode, err error) {
	seqMetrics, _ := metricsProvider(genesis.ChainID)

	mainKV := newPrefixKV(database, RollkitPrefix)
	headerSyncService, err := initHeaderSyncService(mainKV, nodeConfig, genesis, p2pClient, logger)
	if err != nil {
		return nil, err
	}

	dataSyncService, err := initDataSyncService(mainKV, nodeConfig, genesis, p2pClient, logger)
	if err != nil {
		return nil, err
	}

	store := store.New(mainKV)

	blockManager, err := initBlockManager(
		ctx,
		signer,
		exec,
		nodeConfig,
		genesis,
		store,
		sequencer,
		da,
		logger,
		headerSyncService,
		dataSyncService,
		seqMetrics,
		nodeConfig.DA.GasPrice,
		nodeConfig.DA.GasMultiplier,
	)
	if err != nil {
		return nil, err
	}

	reaper := block.NewReaper(
		ctx,
		exec,
		sequencer,
		genesis.ChainID,
		nodeConfig.Node.BlockTime.Duration,
		logger.With("module", "Reaper"),
		mainKV,
	)

	// Connect the reaper to the manager for transaction notifications
	reaper.SetManager(blockManager)

	node := &FullNode{
		genesis:      genesis,
		nodeConfig:   nodeConfig,
		p2pClient:    p2pClient,
		blockManager: blockManager,
		reaper:       reaper,
		da:           da,
		Store:        store,
		hSyncService: headerSyncService,
		dSyncService: dataSyncService,
	}

	node.BaseService = *service.NewBaseService(logger, "Node", node)

	return node, nil
}

func initHeaderSyncService(
	mainKV ds.Batching,
	nodeConfig config.Config,
	genesis genesispkg.Genesis,
	p2pClient *p2p.Client,
	logger log.Logger,
) (*sync.HeaderSyncService, error) {
	headerSyncService, err := sync.NewHeaderSyncService(mainKV, nodeConfig, genesis, p2pClient, logger.With("module", "HeaderSyncService"))
	if err != nil {
		return nil, fmt.Errorf("error while initializing HeaderSyncService: %w", err)
	}
	return headerSyncService, nil
}

func initDataSyncService(
	mainKV ds.Batching,
	nodeConfig config.Config,
	genesis genesispkg.Genesis,
	p2pClient *p2p.Client,
	logger log.Logger,
) (*sync.DataSyncService, error) {
	dataSyncService, err := sync.NewDataSyncService(mainKV, nodeConfig, genesis, p2pClient, logger.With("module", "DataSyncService"))
	if err != nil {
		return nil, fmt.Errorf("error while initializing DataSyncService: %w", err)
	}
	return dataSyncService, nil
}

// initBlockManager initializes the block manager.
// It requires:
// - signingKey: the private key of the validator
// - nodeConfig: the node configuration
// - genesis: the genesis document
// - store: the store
// - seqClient: the sequencing client
// - da: the DA
func initBlockManager(
	ctx context.Context,
	signer signer.Signer,
	exec coreexecutor.Executor,
	nodeConfig config.Config,
	genesis genesispkg.Genesis,
	store store.Store,
	sequencer coresequencer.Sequencer,
	da coreda.DA,
	logger log.Logger,
	headerSyncService *sync.HeaderSyncService,
	dataSyncService *sync.DataSyncService,
	seqMetrics *block.Metrics,
	gasPrice float64,
	gasMultiplier float64,
) (*block.Manager, error) {
	logger.Debug("Proposer address", "address", genesis.ProposerAddress)

	blockManager, err := block.NewManager(
		ctx,
		signer,
		nodeConfig,
		genesis,
		store,
		exec,
		sequencer,
		da,
		logger.With("module", "BlockManager"),
		headerSyncService.Store(),
		dataSyncService.Store(),
		seqMetrics,
		gasPrice,
		gasMultiplier,
	)
	if err != nil {
		return nil, fmt.Errorf("error while initializing BlockManager: %w", err)
	}
	return blockManager, nil
}

// initGenesisChunks creates a chunked format of the genesis document to make it easier to
// iterate through larger genesis structures.
func (n *FullNode) initGenesisChunks() error {
	if n.genChunks != nil {
		return nil
	}

	data, err := json.Marshal(n.genesis)
	if err != nil {
		return err
	}

	for i := 0; i < len(data); i += genesisChunkSize {
		end := min(i+genesisChunkSize, len(data))

		n.genChunks = append(n.genChunks, base64.StdEncoding.EncodeToString(data[i:end]))
	}

	return nil
}

func (n *FullNode) headerPublishLoop(ctx context.Context) {
	for {
		select {
		case signedHeader := <-n.blockManager.HeaderCh:
			err := n.hSyncService.WriteToStoreAndBroadcast(ctx, signedHeader)
			if err != nil {
				// failed to init or start headerstore
				n.Logger.Error(err.Error())
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func (n *FullNode) dataPublishLoop(ctx context.Context) {
	for {
		select {
		case data := <-n.blockManager.DataCh:
			err := n.dSyncService.WriteToStoreAndBroadcast(ctx, data)
			if err != nil {
				// failed to init or start blockstore
				n.Logger.Error(err.Error())
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// startInstrumentationServer starts HTTP servers for instrumentation (Prometheus metrics and pprof).
// Returns the primary server (Prometheus if enabled, otherwise pprof) and optionally a secondary server.
func (n *FullNode) startInstrumentationServer() (*http.Server, *http.Server) {
	var prometheusServer, pprofServer *http.Server

	// Check if Prometheus is enabled
	if n.nodeConfig.Instrumentation.IsPrometheusEnabled() {
		prometheusMux := http.NewServeMux()

		// Register Prometheus metrics handler
		prometheusMux.Handle("/metrics", promhttp.InstrumentMetricHandler(
			prometheus.DefaultRegisterer, promhttp.HandlerFor(
				prometheus.DefaultGatherer,
				promhttp.HandlerOpts{MaxRequestsInFlight: n.nodeConfig.Instrumentation.MaxOpenConnections},
			),
		))

		prometheusServer = &http.Server{
			Addr:              n.nodeConfig.Instrumentation.PrometheusListenAddr,
			Handler:           prometheusMux,
			ReadHeaderTimeout: readHeaderTimeout,
		}

		go func() {
			if err := prometheusServer.ListenAndServe(); err != http.ErrServerClosed {
				// Error starting or closing listener:
				n.Logger.Error("Prometheus HTTP server ListenAndServe", "err", err)
			}
		}()

		n.Logger.Info("Started Prometheus HTTP server", "addr", n.nodeConfig.Instrumentation.PrometheusListenAddr)
	}

	// Check if pprof is enabled
	if n.nodeConfig.Instrumentation.IsPprofEnabled() {
		pprofMux := http.NewServeMux()

		// Register pprof handlers
		pprofMux.HandleFunc("/debug/pprof/", pprof.Index)
		pprofMux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		pprofMux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		pprofMux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		pprofMux.HandleFunc("/debug/pprof/trace", pprof.Trace)
		// Register other pprof handlers
		pprofMux.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
		pprofMux.Handle("/debug/pprof/heap", pprof.Handler("heap"))
		pprofMux.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))
		pprofMux.Handle("/debug/pprof/block", pprof.Handler("block"))
		pprofMux.Handle("/debug/pprof/mutex", pprof.Handler("mutex"))
		pprofMux.Handle("/debug/pprof/allocs", pprof.Handler("allocs"))

		pprofServer = &http.Server{
			Addr:              n.nodeConfig.Instrumentation.GetPprofListenAddr(),
			Handler:           pprofMux,
			ReadHeaderTimeout: readHeaderTimeout,
		}

		go func() {
			if err := pprofServer.ListenAndServe(); err != http.ErrServerClosed {
				// Error starting or closing listener:
				n.Logger.Error("pprof HTTP server ListenAndServe", "err", err)
			}
		}()

		n.Logger.Info("Started pprof HTTP server", "addr", n.nodeConfig.Instrumentation.GetPprofListenAddr())
	}

	// Return the primary server (for backward compatibility) and the secondary server
	if prometheusServer != nil {
		return prometheusServer, pprofServer
	}
	return pprofServer, nil
}

// Run implements the Service interface.
// It starts all subservices and manages the node's lifecycle.
func (n *FullNode) Run(parentCtx context.Context) error {
	ctx, cancelNode := context.WithCancel(parentCtx)
	defer cancelNode() // safety net

	// begin prometheus metrics gathering if it is enabled
	if n.nodeConfig.Instrumentation != nil &&
		(n.nodeConfig.Instrumentation.IsPrometheusEnabled() || n.nodeConfig.Instrumentation.IsPprofEnabled()) {
		n.prometheusSrv, n.pprofSrv = n.startInstrumentationServer()
	}

	// Start RPC server
	handler, err := rpcserver.NewServiceHandler(n.Store, n.p2pClient)
	if err != nil {
		return fmt.Errorf("error creating RPC handler: %w", err)
	}

	n.rpcServer = &http.Server{
		Addr:         n.nodeConfig.RPC.Address,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		err := n.rpcServer.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			n.Logger.Error("RPC server error", "err", err)
		}
		n.Logger.Info("started RPC server", "addr", n.nodeConfig.RPC.Address)
	}()

	n.Logger.Info("starting P2P client")
	err = n.p2pClient.Start(ctx)
	if err != nil {
		return fmt.Errorf("error while starting P2P client: %w", err)
	}

	if err = n.hSyncService.Start(ctx); err != nil {
		return fmt.Errorf("error while starting header sync service: %w", err)
	}

	if err = n.dSyncService.Start(ctx); err != nil {
		return fmt.Errorf("error while starting data sync service: %w", err)
	}

	// only the first error is propagated
	// any error is an issue, so blocking is not a problem
	errCh := make(chan error, 1)

	if n.nodeConfig.Node.Aggregator {
		n.Logger.Info("working in aggregator mode", "block time", n.nodeConfig.Node.BlockTime)
		go n.blockManager.AggregationLoop(ctx, errCh)
		go n.reaper.Start(ctx)
		go n.blockManager.HeaderSubmissionLoop(ctx)
		go n.blockManager.BatchSubmissionLoop(ctx)
		go n.headerPublishLoop(ctx)
		go n.dataPublishLoop(ctx)
		go n.blockManager.DAIncluderLoop(ctx, errCh)
	} else {
		go n.blockManager.RetrieveLoop(ctx)
		go n.blockManager.HeaderStoreRetrieveLoop(ctx)
		go n.blockManager.DataStoreRetrieveLoop(ctx)
		go n.blockManager.SyncLoop(ctx, errCh)
		go n.blockManager.DAIncluderLoop(ctx, errCh)
	}

	select {
	case err := <-errCh:
		if err != nil {
			n.Logger.Error("unrecoverable error in one of the go routines...", "error", err)
			cancelNode() // propagate shutdown to all child goroutines
		}
	case <-parentCtx.Done():
		// Block until parent context is canceled
		n.Logger.Info("context canceled, stopping node")
		cancelNode() // propagate shutdown to all child goroutines
	}

	// Perform cleanup
	n.Logger.Info("halting full node and its sub services...")

	// Use a timeout context to ensure shutdown doesn't hang
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var multiErr error // Use a multierror variable

	// Stop P2P Client
	err = n.p2pClient.Close()
	if err != nil {
		multiErr = errors.Join(multiErr, fmt.Errorf("closing P2P client: %w", err))
	}

	// Stop Header Sync Service
	err = n.hSyncService.Stop(shutdownCtx)
	if err != nil {
		// Log context canceled errors at a lower level if desired, or handle specific non-cancel errors
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			n.Logger.Error("error stopping header sync service", "error", err)
			multiErr = errors.Join(multiErr, fmt.Errorf("stopping header sync service: %w", err))
		} else {
			n.Logger.Debug("header sync service stop context ended", "reason", err) // Log cancellation as debug
		}
	}

	// Stop Data Sync Service
	err = n.dSyncService.Stop(shutdownCtx)
	if err != nil {
		// Log context canceled errors at a lower level if desired, or handle specific non-cancel errors
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			n.Logger.Error("error stopping data sync service", "error", err)
			multiErr = errors.Join(multiErr, fmt.Errorf("stopping data sync service: %w", err))
		} else {
			n.Logger.Debug("data sync service stop context ended", "reason", err) // Log cancellation as debug
		}
	}

	// Shutdown Prometheus Server
	if n.prometheusSrv != nil {
		err = n.prometheusSrv.Shutdown(shutdownCtx)
		// http.ErrServerClosed is expected on graceful shutdown
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			multiErr = errors.Join(multiErr, fmt.Errorf("shutting down Prometheus server: %w", err))
		}
	}

	// Shutdown Pprof Server
	if n.pprofSrv != nil {
		err = n.pprofSrv.Shutdown(shutdownCtx)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			multiErr = errors.Join(multiErr, fmt.Errorf("shutting down pprof server: %w", err))
		}
	}

	// Shutdown RPC Server
	if n.rpcServer != nil {
		err = n.rpcServer.Shutdown(shutdownCtx)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			multiErr = errors.Join(multiErr, fmt.Errorf("shutting down RPC server: %w", err))
		}
	}

	// Ensure Store.Close is called last to maximize chance of data flushing
	if err = n.Store.Close(); err != nil {
		multiErr = errors.Join(multiErr, fmt.Errorf("closing store: %w", err))
	}

	// Save caches if needed
	if err := n.blockManager.SaveCache(); err != nil {
		multiErr = errors.Join(multiErr, fmt.Errorf("saving caches: %w", err))
	}

	// Log final status
	if multiErr != nil {
		for _, err := range multiErr.(interface{ Unwrap() []error }).Unwrap() {
			n.Logger.Error("error during shutdown", "error", err)
		}
	} else {
		n.Logger.Info("full node halted successfully")
	}

	// Return the original context error if it exists (e.g., context cancelled)
	// or the combined shutdown error if the context cancellation was clean.
	if ctx.Err() != nil {
		return ctx.Err()
	}

	return multiErr // Return shutdown errors if context was okay
}

// GetGenesis returns entire genesis doc.
func (n *FullNode) GetGenesis() genesispkg.Genesis {
	return n.genesis
}

// GetGenesisChunks returns chunked version of genesis.
func (n *FullNode) GetGenesisChunks() ([]string, error) {
	err := n.initGenesisChunks()
	if err != nil {
		return nil, err
	}
	return n.genChunks, nil
}

// IsRunning returns true if the node is running.
func (n *FullNode) IsRunning() bool {
	return n.blockManager != nil
}

// SetLogger sets the logger used by node.
func (n *FullNode) SetLogger(logger log.Logger) {
	n.Logger = logger
}

// GetLogger returns logger.
func (n *FullNode) GetLogger() log.Logger {
	return n.Logger
}

func newPrefixKV(kvStore ds.Batching, prefix string) ds.Batching {
	return ktds.Wrap(kvStore, ktds.PrefixTransform{Prefix: ds.NewKey(prefix)})
}
