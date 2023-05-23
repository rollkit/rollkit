package commands

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	tmCfg "github.com/tendermint/tendermint/config"
	tmflags "github.com/tendermint/tendermint/libs/cli/flags"
	log "github.com/tendermint/tendermint/libs/log"
	tmos "github.com/tendermint/tendermint/libs/os"
	tmnode "github.com/tendermint/tendermint/node"
	tmp2p "github.com/tendermint/tendermint/p2p"
	privval "github.com/tendermint/tendermint/privval"
	"github.com/tendermint/tendermint/proxy"

	rollconf "github.com/rollkit/rollkit/config"
	rollconv "github.com/rollkit/rollkit/conv"
	rollnode "github.com/rollkit/rollkit/node"
	rollrpc "github.com/rollkit/rollkit/rpc"
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
	namespaceID    string        = "0000000000000000"
	fraudProofs    bool          = false
	light          bool          = false
	trustedHash    string        = ""
)

// AddNodeFlags exposes some common configuration options on the command-line
// These are exposed for convenience of commands embedding a rollkit node
func AddNodeFlags(cmd *cobra.Command) {
	// node flags
	cmd.Flags().BytesHexVar(
		&genesisHash,
		"genesis_hash",
		[]byte{},
		"optional SHA-256 hash of the genesis file")
	// abci flags
	cmd.Flags().String(
		"proxy_app",
		tendermintConfig.ProxyApp,
		"proxy app address, or one of: 'kvstore',"+
			" 'persistent_kvstore', 'counter', 'e2e' or 'noop' for local testing.")
	cmd.Flags().String("transport", tendermintConfig.ABCI, "specify abci transport (socket | grpc)")

	// rpc flags
	cmd.Flags().String("rpc.laddr", tendermintConfig.RPC.ListenAddress, "RPC listen address. Port required")

	// p2p flags
	cmd.Flags().String(
		"p2p.laddr",
		tendermintConfig.P2P.ListenAddress,
		"node listen address.")

	// db flags
	// Would be cool if rollkit supported different DB backends
	// cmd.Flags().String(
	// 	"db_backend",
	// 	config.DBBackend,
	// 	"database backend: goleveldb | cleveldb | boltdb | rocksdb | badgerdb")
	cmd.Flags().String(
		"db_dir",
		tendermintConfig.DBPath,
		"database directory")

	// Rollkit commands
	cmd.Flags().BoolVar(&aggregator, rollconf.FlagAggregator, aggregator, "run node in aggregator mode")
	cmd.Flags().BoolVar(&lazyAggregator, rollconf.FlagLazyAggregator, lazyAggregator, "wait for transactions, don't build empty blocks")
	cmd.Flags().StringVar(&daLayer, rollconf.FlagDALayer, daLayer, "Data Availability Layer Client name (mock or grpc")
	cmd.Flags().StringVar(&daConfig, rollconf.FlagDAConfig, daConfig, "Data Availability Layer Client config")
	cmd.Flags().DurationVar(&blockTime, rollconf.FlagBlockTime, blockTime, "block time (for aggregator mode)")
	cmd.Flags().DurationVar(&daBlockTime, rollconf.FlagDABlockTime, daBlockTime, "DA chain block time (for syncing)")
	cmd.Flags().Uint64Var(&daStartHeight, rollconf.FlagDAStartHeight, daStartHeight, "starting DA block height (for syncing)")
	cmd.Flags().StringVar(&namespaceID, rollconf.FlagNamespaceID, namespaceID, "namespace identifies (8 bytes in hex)")
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

			genDocProvider := tmnode.DefaultGenesisDocProviderFunc(tendermintConfig)
			genDoc, err := genDocProvider()
			if err != nil {
				return err
			}
			nodeKey, err := tmp2p.LoadOrGenNodeKey(tendermintConfig.NodeKeyFile())
			if err != nil {
				return err
			}
			pval := privval.LoadOrGenFilePV(tendermintConfig.PrivValidatorKeyFile(), tendermintConfig.PrivValidatorStateFile())
			p2pKey, err := rollconv.GetNodeKey(nodeKey)
			if err != nil {
				return err
			}
			signingKey, err := rollconv.GetNodeKey(&tmp2p.NodeKey{PrivKey: pval.Key.PrivKey})
			if err != nil {
				return err
			}

			// create logger
			logger := log.NewTMLogger(log.NewSyncWriter(os.Stdout))
			logger, err = tmflags.ParseLogLevel(tendermintConfig.LogLevel, logger, tmCfg.DefaultLogLevel)
			if err != nil {
				return fmt.Errorf("failed to parse log level: %w", err)
			}

			// default to socket connections for remote clients
			if len(tendermintConfig.ABCI) == 0 {
				tendermintConfig.ABCI = "socket"
			}

			bytes, err := hex.DecodeString(namespaceID)
			if err != nil {
				return err
			}

			rollkitConfig := rollconf.NodeConfig{
				Aggregator: aggregator,
				BlockManagerConfig: rollconf.BlockManagerConfig{
					BlockTime:     blockTime,
					FraudProofs:   fraudProofs,
					DAStartHeight: daStartHeight,
					DABlockTime:   daBlockTime,
				},
				DALayer:  daLayer,
				DAConfig: daConfig,
				Light:    light,
				HeaderConfig: rollconf.HeaderConfig{
					TrustedHash: trustedHash,
				},
				LazyAggregator: lazyAggregator,
			}
			copy(rollkitConfig.NamespaceID[:], bytes)

			rollconv.GetNodeConfig(&rollkitConfig, tendermintConfig)
			if err := rollconv.TranslateAddresses(&rollkitConfig); err != nil {
				return err
			}

			rollnode, err := rollnode.NewNode(
				context.Background(),
				rollkitConfig,
				p2pKey,
				signingKey,
				proxy.DefaultClientCreator(tendermintConfig.ProxyApp, tendermintConfig.ABCI, rollkitConfig.DBPath),
				genDoc,
				logger,
			)

			if err != nil {
				return fmt.Errorf("failed to create new rollkit node: %w", err)
			}

			server := rollrpc.NewServer(rollnode, tendermintConfig.RPC, logger)
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

	AddNodeFlags(cmd)
	return cmd
}
