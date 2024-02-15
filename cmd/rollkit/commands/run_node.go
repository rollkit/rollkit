package commands

import (
	"context"
	"fmt"
	"os"
	"time"

	cmCfg "github.com/cometbft/cometbft/config"
	tmflags "github.com/cometbft/cometbft/libs/cli/flags"
	log "github.com/cometbft/cometbft/libs/log"
	tmos "github.com/cometbft/cometbft/libs/os"
	tmnode "github.com/cometbft/cometbft/node"
	tmp2p "github.com/cometbft/cometbft/p2p"
	privval "github.com/cometbft/cometbft/privval"
	"github.com/cometbft/cometbft/proxy"
	"github.com/spf13/cobra"

	rollconf "github.com/rollkit/rollkit/config"
	rollnode "github.com/rollkit/rollkit/node"
	rollrpc "github.com/rollkit/rollkit/rpc"
	rolltypes "github.com/rollkit/rollkit/types"
)

var (
	genesisHash    []byte
	aggregator     bool          = false
	lazyAggregator bool          = false
	daLayer        string        = "mock"
	daConfig       string        = ""
	blockTime      time.Duration = (30 * time.Second)
	daBlockTime    time.Duration = (15 * time.Second)
	daStartHeight  uint64        = 1
	daNamespace    string        = "0000000000000000"
	fraudProofs    bool          = false
	light          bool          = false
	trustedHash    string        = ""
)

// addNodeFlags exposes some common configuration options on the command-line
// These are exposed for convenience of commands embedding a rollkit node
func addNodeFlags(cmd *cobra.Command) {
	config := defaultConfig
	// node flags
	cmd.Flags().BytesHexVar(
		&genesisHash,
		"genesis_hash",
		[]byte{},
		"optional SHA-256 hash of the genesis file")
	// abci flags
	cmd.Flags().String(
		"proxy_app",
		config.ProxyApp,
		"proxy app address, or one of: 'kvstore',"+
			" 'persistent_kvstore', 'counter', 'e2e' or 'noop' for local testing.")
	cmd.Flags().String("transport", config.ABCI, "specify abci transport (socket | grpc)")

	// rpc flags
	cmd.Flags().String("rpc.laddr", config.RPC.ListenAddress, "RPC listen address. Port required")

	// p2p flags
	cmd.Flags().String(
		"p2p.laddr",
		config.P2P.ListenAddress,
		"node listen address.")

	// db flags
	// Would be cool if rollkit supported different DB backends
	// cmd.Flags().String(
	// 	"db_backend",
	// 	config.DBBackend,
	// 	"database backend: goleveldb | cleveldb | boltdb | rocksdb | badgerdb")
	cmd.Flags().String(
		"db_dir",
		config.DBPath,
		"database directory")

	// Rollkit commands
	cmd.Flags().BoolVar(&aggregator, rollconf.FlagAggregator, aggregator, "run node in aggregator mode")
	cmd.Flags().BoolVar(&lazyAggregator, rollconf.FlagLazyAggregator, lazyAggregator, "wait for transactions, don't build empty blocks")
	cmd.Flags().StringVar(&daLayer, rollconf.FlagDALayer, daLayer, "Data Availability Layer Client name (mock or grpc")
	cmd.Flags().StringVar(&daConfig, rollconf.FlagDAConfig, daConfig, "Data Availability Layer Client config")
	cmd.Flags().DurationVar(&blockTime, rollconf.FlagBlockTime, blockTime, "block time (for aggregator mode)")
	cmd.Flags().DurationVar(&daBlockTime, rollconf.FlagDABlockTime, daBlockTime, "DA chain block time (for syncing)")
	cmd.Flags().Uint64Var(&daStartHeight, rollconf.FlagDAStartHeight, daStartHeight, "starting DA block height (for syncing)")
	cmd.Flags().StringVar(&daNamespace, rollconf.FlagDANamespace, daNamespace, "namespace identifies (8 bytes in hex)")
	cmd.Flags().BoolVar(&fraudProofs, rollconf.FlagFraudProofs, fraudProofs, "enable fraud proofs (experimental & insecure)")
	cmd.Flags().BoolVar(&light, rollconf.FlagLight, light, "run light client")
	cmd.Flags().StringVar(&trustedHash, rollconf.FlagTrustedHash, trustedHash, "initial trusted hash to start the header exchange service")
}

// NewRunNodeCmd returns the command that allows the CLI to start a node.
func NewRunNodeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "start",
		Aliases: []string{"node", "run"},
		Short:   "Run the rollkit node",
		RunE: func(cmd *cobra.Command, args []string) error {
			defaultCometConfig := cmCfg.DefaultConfig()
			genDocProvider := tmnode.DefaultGenesisDocProviderFunc(defaultCometConfig)
			genDoc, err := genDocProvider()
			if err != nil {
				return err
			}
			nodeKey, err := tmp2p.LoadOrGenNodeKey(defaultCometConfig.NodeKeyFile())
			if err != nil {
				return err
			}
			pval := privval.LoadOrGenFilePV(defaultCometConfig.PrivValidatorKeyFile(), defaultCometConfig.PrivValidatorStateFile())
			p2pKey, err := rolltypes.GetNodeKey(nodeKey)
			if err != nil {
				return err
			}
			signingKey, err := rolltypes.GetNodeKey(&tmp2p.NodeKey{PrivKey: pval.Key.PrivKey})
			if err != nil {
				return err
			}

			// create logger
			logger := log.NewTMLogger(log.NewSyncWriter(os.Stdout))
			logger, err = tmflags.ParseLogLevel(defaultCometConfig.LogLevel, logger, cmCfg.DefaultLogLevel)
			if err != nil {
				return fmt.Errorf("failed to parse log level: %w", err)
			}

			// default to socket connections for remote clients
			if len(defaultCometConfig.ABCI) == 0 {
				defaultCometConfig.ABCI = "socket"
			}

			rollkitConfig := rollconf.NodeConfig{
				Aggregator: aggregator,
				BlockManagerConfig: rollconf.BlockManagerConfig{
					BlockTime:     blockTime,
					FraudProofs:   fraudProofs,
					DAStartHeight: daStartHeight,
					DABlockTime:   daBlockTime,
				},
				DALayer:     daLayer,
				DAConfig:    daConfig,
				DANamespace: daNamespace,
				Light:       light,
				HeaderConfig: rollconf.HeaderConfig{
					TrustedHash: trustedHash,
				},
				LazyAggregator: lazyAggregator,
			}

			rollconf.GetNodeConfig(&rollkitConfig, defaultCometConfig)
			if err := rollconf.TranslateAddresses(&rollkitConfig); err != nil {
				return err
			}

			metrics := rollnode.DefaultMetricsProvider(cmCfg.DefaultInstrumentationConfig())
			rollnode, err := rollnode.NewNode(
				context.Background(),
				rollkitConfig,
				p2pKey,
				signingKey,
				proxy.DefaultClientCreator(defaultCometConfig.ProxyApp, defaultCometConfig.ABCI, rollkitConfig.DBPath),
				genDoc,
				metrics,
				logger,
			)

			if err != nil {
				return fmt.Errorf("failed to create new rollkit node: %w", err)
			}

			server := rollrpc.NewServer(rollnode, defaultCometConfig.RPC, logger)
			err = server.Start()
			if err != nil {
				return err
			}

			if err := rollnode.Start(); err != nil {
				return fmt.Errorf("failed to start node: %w", err)
			}

			// Do rollkit nodes not have information about them? tendermint has node.switch.NodeInfo()
			logger.Info("Started node")

			// Stop upon receiving SIGTERM or CTRL-C.
			tmos.TrapSignal(logger, func() {
				if rollnode.IsRunning() {
					if err := rollnode.Stop(); err != nil {
						logger.Error("unable to stop the node", "error", err)
					}
				}
			})
			// Run forever.
			select {}
		},
	}

	addNodeFlags(cmd)
	return cmd
}
