package server

import (
	"context"
	"io"
	"net"
	"os"

	cmtcfg "github.com/cometbft/cometbft/config"
	"github.com/cometbft/cometbft/p2p"
	pvm "github.com/cometbft/cometbft/privval"
	"github.com/cometbft/cometbft/proxy"
	cmttypes "github.com/cometbft/cometbft/types"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/server"
	"github.com/cosmos/cosmos-sdk/server/api"
	serverconfig "github.com/cosmos/cosmos-sdk/server/config"
	servergrpc "github.com/cosmos/cosmos-sdk/server/grpc"
	servercmtlog "github.com/cosmos/cosmos-sdk/server/log"
	sdktypes "github.com/cosmos/cosmos-sdk/server/types"
	"github.com/cosmos/cosmos-sdk/telemetry"
	"github.com/cosmos/cosmos-sdk/version"
	genutiltypes "github.com/cosmos/cosmos-sdk/x/genutil/types"
	"github.com/hashicorp/go-metrics"
	"github.com/rollkit/rollkit/config"
	"github.com/rollkit/rollkit/node"
	"github.com/rollkit/rollkit/rpc"
	"github.com/rollkit/rollkit/types"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	flagTraceStore = "trace-store"
	flagTransport  = "transport"
	flagGRPCOnly   = "grpc-only"
)

func StartHandler[T sdktypes.Application](svrCtx *server.Context, clientCtx client.Context, appCreator sdktypes.AppCreator, inProcess bool, opts server.StartCmdOptions) error {
	svrCfg, err := getAndValidateConfig(svrCtx)
	if err != nil {
		return err
	}

	app, appCleanupFn, err := startApp[T](svrCtx, appCreator, opts)
	if err != nil {
		return err
	}
	defer appCleanupFn()

	metrics, err := startTelemetry(svrCfg)
	if err != nil {
		return err
	}

	emitServerInfoMetrics()

	return startInProcess[T](svrCtx, svrCfg, clientCtx, app, metrics, opts)
}

func startApp[T sdktypes.Application](svrCtx *server.Context, appCreator sdktypes.AppCreator, opts server.StartCmdOptions) (app sdktypes.Application, cleanupFn func(), err error) {
	traceWriter, traceCleanupFn, err := setupTraceWriter(svrCtx)
	if err != nil {
		return app, traceCleanupFn, err
	}

	home := svrCtx.Config.RootDir
	db, err := opts.DBOpener(home, server.GetAppDBBackend(svrCtx.Viper))
	if err != nil {
		return app, traceCleanupFn, err
	}

	app = appCreator(svrCtx.Logger, db, traceWriter, svrCtx.Viper)

	cleanupFn = func() {
		traceCleanupFn()
		if localErr := app.Close(); localErr != nil {
			svrCtx.Logger.Error(localErr.Error())
		}
	}
	return app, cleanupFn, nil
}

func startInProcess[T sdktypes.Application](svrCtx *server.Context, svrCfg serverconfig.Config, clientCtx client.Context, app sdktypes.Application,
	metrics *telemetry.Metrics, opts server.StartCmdOptions,
) error {
	cmtCfg := svrCtx.Config
	gRPCOnly := svrCtx.Viper.GetBool(flagGRPCOnly)

	g, ctx := getCtx(svrCtx, true)

	if gRPCOnly {
		// TODO: Generalize logic so that gRPC only is really in startStandAlone
		svrCtx.Logger.Info("starting node in gRPC only mode; CometBFT is disabled")
		svrCfg.GRPC.Enable = true
	} else {
		svrCtx.Logger.Info("starting node with ABCI CometBFT in-process")
		server, cleanupFn, err := startNode(ctx, cmtCfg, app, svrCtx)
		if err != nil {
			return err
		}
		defer cleanupFn()

		// Add the tx service to the gRPC router. We only need to register this
		// service if API or gRPC is enabled, and avoid doing so in the general
		// case, because it spawns a new local CometBFT RPC client.
		if svrCfg.API.Enable || svrCfg.GRPC.Enable {
			// Re-assign for making the client available below do not use := to avoid
			// shadowing the clientCtx variable.
			clientCtx = clientCtx.WithClient(server.Client())

			app.RegisterTxService(clientCtx)
			app.RegisterTendermintService(clientCtx)
			app.RegisterNodeService(clientCtx, svrCfg)
		}
	}

	grpcSrv, clientCtx, err := startGrpcServer(ctx, g, svrCfg.GRPC, clientCtx, svrCtx, app)
	if err != nil {
		return err
	}

	err = startAPIServer(ctx, g, svrCfg, clientCtx, svrCtx, app, cmtCfg.RootDir, grpcSrv, metrics)
	if err != nil {
		return err
	}

	if opts.PostSetup != nil {
		if err := opts.PostSetup(svrCtx, clientCtx, ctx, g); err != nil {
			return err
		}
	}

	// wait for signal capture and gracefully return
	// we are guaranteed to be waiting for the "ListenForQuitSignals" goroutine.
	return g.Wait()
}

func startGrpcServer(
	ctx context.Context,
	g *errgroup.Group,
	config serverconfig.GRPCConfig,
	clientCtx client.Context,
	svrCtx *server.Context,
	app sdktypes.Application,
) (*grpc.Server, client.Context, error) {
	if !config.Enable {
		// return grpcServer as nil if gRPC is disabled
		return nil, clientCtx, nil
	}
	_, _, err := net.SplitHostPort(config.Address)
	if err != nil {
		return nil, clientCtx, err
	}

	maxSendMsgSize := config.MaxSendMsgSize
	if maxSendMsgSize == 0 {
		maxSendMsgSize = serverconfig.DefaultGRPCMaxSendMsgSize
	}

	maxRecvMsgSize := config.MaxRecvMsgSize
	if maxRecvMsgSize == 0 {
		maxRecvMsgSize = serverconfig.DefaultGRPCMaxRecvMsgSize
	}

	// if gRPC is enabled, configure gRPC client for gRPC gateway
	grpcClient, err := grpc.Dial(
		config.Address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.ForceCodec(codec.NewProtoCodec(clientCtx.InterfaceRegistry).GRPCCodec()),
			grpc.MaxCallRecvMsgSize(maxRecvMsgSize),
			grpc.MaxCallSendMsgSize(maxSendMsgSize),
		),
	)
	if err != nil {
		return nil, clientCtx, err
	}

	clientCtx = clientCtx.WithGRPCClient(grpcClient)
	svrCtx.Logger.Debug("gRPC client assigned to client context", "target", config.Address)

	grpcSrv, err := servergrpc.NewGRPCServer(clientCtx, app, config)
	if err != nil {
		return nil, clientCtx, err
	}

	// Start the gRPC server in a goroutine. Note, the provided ctx will ensure
	// that the server is gracefully shut down.
	g.Go(func() error {
		return servergrpc.StartGRPCServer(ctx, svrCtx.Logger.With("module", "grpc-server"), config, grpcSrv)
	})
	return grpcSrv, clientCtx, nil
}

func startAPIServer(
	ctx context.Context,
	g *errgroup.Group,
	svrCfg serverconfig.Config,
	clientCtx client.Context,
	svrCtx *server.Context,
	app sdktypes.Application,
	home string,
	grpcSrv *grpc.Server,
	metrics *telemetry.Metrics,
) error {
	if !svrCfg.API.Enable {
		return nil
	}

	clientCtx = clientCtx.WithHomeDir(home)

	apiSrv := api.New(clientCtx, svrCtx.Logger.With("module", "api-server"), grpcSrv)
	app.RegisterAPIRoutes(apiSrv, svrCfg.API)

	if svrCfg.Telemetry.Enabled {
		apiSrv.SetTelemetry(metrics)
	}

	g.Go(func() error {
		return apiSrv.Start(ctx, svrCfg)
	})
	return nil
}

func startTelemetry(cfg serverconfig.Config) (*telemetry.Metrics, error) {
	if !cfg.Telemetry.Enabled {
		return nil, nil
	}

	return telemetry.New(cfg.Telemetry)
}

func startNode(
	ctx context.Context,
	cfg *cmtcfg.Config,
	app sdktypes.Application,
	svrCtx *server.Context,
) (serv *rpc.Server, cleanupFn func(), err error) {
	nodeKey, err := p2p.LoadOrGenNodeKey(cfg.NodeKeyFile())
	if err != nil {
		return nil, cleanupFn, err
	}

	svrCtx.Logger.Info("starting node with Rollkit in-process")

	pval := pvm.LoadOrGenFilePV(cfg.PrivValidatorKeyFile(), cfg.PrivValidatorStateFile())

	//keys in Rollkit format
	p2pKey, err := types.GetNodeKey(nodeKey)
	if err != nil {
		return nil, cleanupFn, err
	}

	signingKey, err := types.GetNodeKey(&p2p.NodeKey{PrivKey: pval.Key.PrivKey})
	if err != nil {
		return nil, cleanupFn, err
	}

	nodeConfig := config.NodeConfig{}
	err = nodeConfig.GetViperConfig(svrCtx.Viper)
	if err != nil {
		return nil, cleanupFn, err
	}
	config.GetNodeConfig(&nodeConfig, cfg)
	err = config.TranslateAddresses(&nodeConfig)
	if err != nil {
		return nil, cleanupFn, err
	}

	genDoc, err := getGenDocProvider(cfg)()
	if err != nil {
		return nil, cleanupFn, err
	}

	cmtApp := server.NewCometABCIWrapper(app)
	node, err := node.NewNode(
		ctx,
		nodeConfig,
		p2pKey,
		signingKey,
		proxy.NewLocalClientCreator(cmtApp),
		genDoc,
		node.DefaultMetricsProvider(cfg.Instrumentation),
		servercmtlog.CometLoggerWrapper{Logger: svrCtx.Logger},
	)
	if err != nil {
		return nil, cleanupFn, err
	}

	serv = rpc.NewServer(node, cfg.RPC, servercmtlog.CometLoggerWrapper{Logger: svrCtx.Logger})
	err = serv.Start()
	if err != nil {
		return nil, cleanupFn, err
	}

	if err := node.Start(); err != nil {
		return nil, cleanupFn, err
	}

	cleanupFn = func() {
		if node != nil && node.IsRunning() {
			_ = node.Stop()
		}
	}

	return serv, cleanupFn, nil
}

// getGenDocProvider returns a function which returns the genesis doc from the genesis file.
func getGenDocProvider(cfg *cmtcfg.Config) func() (*cmttypes.GenesisDoc, error) {
	return func() (*cmttypes.GenesisDoc, error) {
		appGenesis, err := genutiltypes.AppGenesisFromFile(cfg.GenesisFile())
		if err != nil {
			return nil, err
		}

		return appGenesis.ToGenesisDoc()
	}
}

func getAndValidateConfig(svrCtx *server.Context) (serverconfig.Config, error) {
	config, err := serverconfig.GetConfig(svrCtx.Viper)
	if err != nil {
		return config, err
	}

	if err := config.ValidateBasic(); err != nil {
		return config, err
	}
	return config, nil
}

func getCtx(svrCtx *server.Context, block bool) (*errgroup.Group, context.Context) {
	ctx, cancelFn := context.WithCancel(context.Background())
	g, ctx := errgroup.WithContext(ctx)
	// listen for quit signals so the calling parent process can gracefully exit
	server.ListenForQuitSignals(g, block, cancelFn, svrCtx.Logger)
	return g, ctx
}

func openTraceWriter(traceWriterFile string) (w io.WriteCloser, err error) {
	if traceWriterFile == "" {
		return
	}
	return os.OpenFile(
		traceWriterFile,
		os.O_WRONLY|os.O_APPEND|os.O_CREATE,
		0o666,
	)
}

func setupTraceWriter(svrCtx *server.Context) (traceWriter io.WriteCloser, cleanup func(), err error) {
	// clean up the traceWriter when the server is shutting down
	cleanup = func() {}

	traceWriterFile := svrCtx.Viper.GetString(flagTraceStore)
	traceWriter, err = openTraceWriter(traceWriterFile)
	if err != nil {
		return traceWriter, cleanup, err
	}

	// if flagTraceStore is not used then traceWriter is nil
	if traceWriter != nil {
		cleanup = func() {
			if err = traceWriter.Close(); err != nil {
				svrCtx.Logger.Error("failed to close trace writer", "err", err)
			}
		}
	}

	return traceWriter, cleanup, nil
}

// emitServerInfoMetrics emits server info related metrics using application telemetry.
func emitServerInfoMetrics() {
	var ls []metrics.Label

	versionInfo := version.NewInfo()
	if len(versionInfo.GoVersion) > 0 {
		ls = append(ls, telemetry.NewLabel("go", versionInfo.GoVersion))
	}
	if len(versionInfo.CosmosSdkVersion) > 0 {
		ls = append(ls, telemetry.NewLabel("version", versionInfo.CosmosSdkVersion))
	}

	if len(ls) == 0 {
		return
	}

	telemetry.SetGaugeWithLabels([]string{"server", "info"}, 1, ls)
}
