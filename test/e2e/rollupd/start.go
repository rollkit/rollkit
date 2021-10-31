package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"time"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/server"
	"github.com/cosmos/cosmos-sdk/server/api"
	"github.com/cosmos/cosmos-sdk/server/config"
	servergrpc "github.com/cosmos/cosmos-sdk/server/grpc"
	"github.com/cosmos/cosmos-sdk/server/types"
	"github.com/cosmos/cosmos-sdk/snapshots"
	"github.com/cosmos/cosmos-sdk/store"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cast"
	"github.com/spf13/cobra"
	"github.com/tendermint/spm/cosmoscmd"
	"github.com/tendermint/tendermint/node"
	"github.com/tendermint/tendermint/p2p"
	"github.com/tendermint/tendermint/proxy"
	dbm "github.com/tendermint/tm-db"
	"google.golang.org/grpc"

	opticonf "github.com/celestiaorg/optimint/config"
	opticonv "github.com/celestiaorg/optimint/conv"
	optinode "github.com/celestiaorg/optimint/node"
	"github.com/celestiaorg/optimint/rpcclient"
	"github.com/tendermint/tendermint/libs/log"
)

// StartCmd runs the service passed in, either stand-alone or in-process with
// Tendermint.
func StartCmd(appCreator types.AppCreator, defaultNodeHome string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start-optimint",
		Short: "Run the full node",
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			serverCtx := server.GetServerContextFromCmd(cmd)

			// Bind flags to the Context's Viper so the app construction can set
			// options accordingly.
			serverCtx.Viper.BindPFlags(cmd.Flags())

			_, err := server.GetPruningOptionsFromFlags(serverCtx.Viper)
			return err
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			serverCtx := server.GetServerContextFromCmd(cmd)
			clientCtx, err := client.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}

			serverCtx.Logger.Info("starting ABCI with Tendermint")

			// amino is needed here for backwards compatibility of REST routes
			err = startInProcess(serverCtx, clientCtx, appCreator)
			errCode, ok := err.(server.ErrorCode)
			if !ok {
				return err
			}

			serverCtx.Logger.Debug(fmt.Sprintf("received quit signal: %d", errCode.Code))
			return nil
		},
	}

	cmd.Flags().String(flags.FlagHome, defaultNodeHome, "The application home directory")
	return cmd
}

// legacyAminoCdc is used for the legacy REST API
func startInProcess(ctx *server.Context, clientCtx client.Context, appCreator types.AppCreator) error {
	cfg := ctx.Config
	home := cfg.RootDir
	var cpuProfileCleanup func()

	config := config.GetConfig(ctx.Viper)

	if err := config.ValidateBasic(); err != nil {
		ctx.Logger.Error("WARNING: The minimum-gas-prices config in app.toml is set to the empty string. " +
			"This defaults to 0 in the current version, but will error in the next version " +
			"(SDK v0.45). Please explicitly put the desired minimum-gas-prices in your app.toml.")
	}

	db, err := openDB(home)
	if err != nil {
		return err
	}

	app := appCreator(ctx.Logger, db, nil, ctx.Viper)

	nodeKey, err := p2p.LoadOrGenNodeKey(cfg.NodeKeyFile())
	if err != nil {
		return err
	}

	genDocProvider := node.DefaultGenesisDocProviderFunc(cfg)
	// node key in optimint format
	oNodeKey, err := opticonv.GetNodeKey(nodeKey)
	if err != nil {
		return err
	}
	genesis, err := genDocProvider()
	if err != nil {
		return err
	}
	nodeConfig := opticonf.NodeConfig{}
	// nodeConfig.GetViperConfig(ctx.Viper)
	opticonv.GetNodeConfig(&nodeConfig, cfg)
	err = opticonv.TranslateAddresses(&nodeConfig)
	if err != nil {
		return err
	}
	// todo(evan): get rid of this and just use a normal config
	fmt.Println("===== FORCING MOCK DA LAYER =====")
	nodeConfig.DALayer = "mock"
	nodeConfig.Aggregator = true
	tmNode, err := optinode.NewNode(
		context.Background(),
		nodeConfig,
		oNodeKey,
		proxy.NewLocalClientCreator(app),
		genesis,
		ctx.Logger,
	)
	if err != nil {
		return err
	}

	ctx.Logger.Debug("initialization: tmNode created")
	if err := tmNode.Start(); err != nil {
		return err
	}
	ctx.Logger.Debug("initialization: tmNode started")

	// Add the tx service to the gRPC router. We only need to register this
	// service if API or gRPC is enabled, and avoid doing so in the general
	// case, because it spawns a new local tendermint RPC client.
	if config.API.Enable || config.GRPC.Enable {
		clientCtx = clientCtx.WithClient(rpcclient.NewLocal(tmNode))

		app.RegisterTxService(clientCtx)
		app.RegisterTendermintService(clientCtx)
	}

	var apiSrv *api.Server
	if config.API.Enable {
		genDoc, err := genDocProvider()
		if err != nil {
			return err
		}

		clientCtx := clientCtx.
			WithHomeDir(home).
			WithChainID(genDoc.ChainID)

		apiSrv = api.New(clientCtx, ctx.Logger.With("module", "api-server"))
		app.RegisterAPIRoutes(apiSrv, config.API)
		errCh := make(chan error)

		go func() {
			if err := apiSrv.Start(config); err != nil {
				errCh <- err
			}
		}()

		select {
		case err := <-errCh:
			return err
		case <-time.After(types.ServerStartTime): // assume server started successfully
		}
	}

	var (
		grpcSrv    *grpc.Server
		grpcWebSrv *http.Server
	)
	if config.GRPC.Enable {
		grpcSrv, err = servergrpc.StartGRPCServer(clientCtx, app, config.GRPC.Address)
		if err != nil {
			return err
		}
		if config.GRPCWeb.Enable {
			grpcWebSrv, err = servergrpc.StartGRPCWeb(grpcSrv, config)
			if err != nil {
				ctx.Logger.Error("failed to start grpc-web http server: ", err)
				return err
			}
		}
	}

	defer func() {
		if tmNode.IsRunning() {
			_ = tmNode.Stop()
		}

		if cpuProfileCleanup != nil {
			cpuProfileCleanup()
		}

		if apiSrv != nil {
			_ = apiSrv.Close()
		}

		if grpcSrv != nil {
			grpcSrv.Stop()
			if grpcWebSrv != nil {
				grpcWebSrv.Close()
			}
		}

		ctx.Logger.Info("exiting...")
	}()

	// Wait for SIGINT or SIGTERM signal
	return server.WaitForQuitSignals()
}

func openDB(rootDir string) (dbm.DB, error) {
	dataDir := filepath.Join(rootDir, "data")
	return sdk.NewLevelDB("application", dataDir)
}

// appCreator is an app creator
type appCreator struct {
	encodingConfig cosmoscmd.EncodingConfig
	buildApp       cosmoscmd.AppBuilder
}

// newApp creates a new Cosmos SDK app
func (a appCreator) newApp(
	logger log.Logger,
	db dbm.DB,
	traceStore io.Writer,
	appOpts types.AppOptions,
) types.Application {
	var cache sdk.MultiStorePersistentCache

	if cast.ToBool(appOpts.Get(server.FlagInterBlockCache)) {
		cache = store.NewCommitKVStoreCacheManager()
	}

	skipUpgradeHeights := make(map[int64]bool)
	for _, h := range cast.ToIntSlice(appOpts.Get(server.FlagUnsafeSkipUpgrades)) {
		skipUpgradeHeights[int64(h)] = true
	}

	pruningOpts, err := server.GetPruningOptionsFromFlags(appOpts)
	if err != nil {
		panic(err)
	}

	snapshotDir := filepath.Join(cast.ToString(appOpts.Get(flags.FlagHome)), "data", "snapshots")
	snapshotDB, err := sdk.NewLevelDB("metadata", snapshotDir)
	if err != nil {
		panic(err)
	}
	snapshotStore, err := snapshots.NewStore(snapshotDB, snapshotDir)
	if err != nil {
		panic(err)
	}

	return a.buildApp(
		logger,
		db,
		traceStore,
		true,
		skipUpgradeHeights,
		cast.ToString(appOpts.Get(flags.FlagHome)),
		cast.ToUint(appOpts.Get(server.FlagInvCheckPeriod)),
		a.encodingConfig,
		appOpts,
		baseapp.SetPruning(pruningOpts),
		baseapp.SetMinGasPrices(cast.ToString(appOpts.Get(server.FlagMinGasPrices))),
		baseapp.SetMinRetainBlocks(cast.ToUint64(appOpts.Get(server.FlagMinRetainBlocks))),
		baseapp.SetHaltHeight(cast.ToUint64(appOpts.Get(server.FlagHaltHeight))),
		baseapp.SetHaltTime(cast.ToUint64(appOpts.Get(server.FlagHaltTime))),
		baseapp.SetInterBlockCache(cache),
		baseapp.SetTrace(cast.ToBool(appOpts.Get(server.FlagTrace))),
		baseapp.SetIndexEvents(cast.ToStringSlice(appOpts.Get(server.FlagIndexEvents))),
		baseapp.SetSnapshotStore(snapshotStore),
		baseapp.SetSnapshotInterval(cast.ToUint64(appOpts.Get(server.FlagStateSyncSnapshotInterval))),
		baseapp.SetSnapshotKeepRecent(cast.ToUint32(appOpts.Get(server.FlagStateSyncSnapshotKeepRecent))),
	)
}

// appExport creates a new simapp (optionally at a given height)
func (a appCreator) appExport(
	logger log.Logger,
	db dbm.DB,
	traceStore io.Writer,
	height int64,
	forZeroHeight bool,
	jailAllowedAddrs []string,
	appOpts types.AppOptions,
) (types.ExportedApp, error) {

	var exportableApp cosmoscmd.ExportableApp

	homePath, ok := appOpts.Get(flags.FlagHome).(string)
	if !ok || homePath == "" {
		return types.ExportedApp{}, errors.New("application home not set")
	}

	exportableApp = a.buildApp(
		logger,
		db,
		traceStore,
		height == -1, // -1: no height provided
		map[int64]bool{},
		homePath,
		uint(1),
		a.encodingConfig,
		appOpts,
	)

	if height != -1 {
		if err := exportableApp.LoadHeight(height); err != nil {
			return types.ExportedApp{}, err
		}
	}

	return exportableApp.ExportAppStateAndValidators(forZeroHeight, jailAllowedAddrs)
}
