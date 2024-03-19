package commands

import (
	"context"
	"fmt"
	"math/rand"
	"net/url"
	"os"

	cmtcmd "github.com/cometbft/cometbft/cmd/cometbft/commands"
	cometconf "github.com/cometbft/cometbft/config"
	cometcli "github.com/cometbft/cometbft/libs/cli"
	cometflags "github.com/cometbft/cometbft/libs/cli/flags"
	cometlog "github.com/cometbft/cometbft/libs/log"
	cometos "github.com/cometbft/cometbft/libs/os"
	cometnode "github.com/cometbft/cometbft/node"
	cometp2p "github.com/cometbft/cometbft/p2p"
	cometprivval "github.com/cometbft/cometbft/privval"
	cometproxy "github.com/cometbft/cometbft/proxy"
	comettypes "github.com/cometbft/cometbft/types"
	comettime "github.com/cometbft/cometbft/types/time"

	proxy "github.com/rollkit/go-da/proxy/jsonrpc"
	goDATest "github.com/rollkit/go-da/test"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	rollconf "github.com/rollkit/rollkit/config"
	rollnode "github.com/rollkit/rollkit/node"
	rollrpc "github.com/rollkit/rollkit/rpc"
	rolltypes "github.com/rollkit/rollkit/types"
)

var (
	// initialize the config with the cometBFT defaults
	config = cometconf.DefaultConfig()

	// initialize the rollkit configuration
	rollkitConfig = rollconf.DefaultNodeConfig

	// initialize the logger with the cometBFT defaults
	logger = cometlog.NewTMLogger(cometlog.NewSyncWriter(os.Stdout))
)

// NewRunNodeCmd returns the command that allows the CLI to start a node.
func NewRunNodeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "start",
		Aliases: []string{"node", "run"},
		Short:   "Run the rollkit node",
		// PersistentPreRunE is used to parse the config and initial the config files
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Parse the config
			err := parseConfig(cmd)
			if err != nil {
				return err
			}

			// Update log format if the flag is set
			if config.LogFormat == cometconf.LogFormatJSON {
				logger = cometlog.NewTMJSONLogger(cometlog.NewSyncWriter(os.Stdout))
			}

			// Parse the log level
			logger, err = cometflags.ParseLogLevel(config.LogLevel, logger, cometconf.DefaultLogLevel)
			if err != nil {
				return err
			}

			// Add tracing to the logger if the flag is set
			if viper.GetBool(cometcli.TraceFlag) {
				logger = cometlog.NewTracingLogger(logger)
			}

			logger = logger.With("module", "main")

			// Initialize the config files
			return initFiles()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			genDocProvider := cometnode.DefaultGenesisDocProviderFunc(config)
			genDoc, err := genDocProvider()
			if err != nil {
				return err
			}
			nodeKey, err := cometp2p.LoadOrGenNodeKey(config.NodeKeyFile())
			if err != nil {
				return err
			}
			pval := cometprivval.LoadOrGenFilePV(config.PrivValidatorKeyFile(), config.PrivValidatorStateFile())
			p2pKey, err := rolltypes.GetNodeKey(nodeKey)
			if err != nil {
				return err
			}
			signingKey, err := rolltypes.GetNodeKey(&cometp2p.NodeKey{PrivKey: pval.Key.PrivKey})
			if err != nil {
				return err
			}

			// default to socket connections for remote clients
			if len(config.ABCI) == 0 {
				config.ABCI = "socket"
			}

			// get the node configuration
			rollconf.GetNodeConfig(&rollkitConfig, config)
			if err := rollconf.TranslateAddresses(&rollkitConfig); err != nil {
				return err
			}

			// initialize the metrics
			metrics := rollnode.DefaultMetricsProvider(cometconf.DefaultInstrumentationConfig())

			// use mock jsonrpc da server by default
			if !cmd.Flags().Lookup("rollkit.da_address").Changed {
				srv, err := startMockDAServJSONRPC(cmd.Context())
				if err != nil {
					return fmt.Errorf("failed to launch mock da server: %w", err)
				}
				// nolint:errcheck,gosec
				defer func() { srv.Stop(cmd.Context()) }()
			}

			// create the rollkit node
			rollnode, err := rollnode.NewNode(
				context.Background(),
				rollkitConfig,
				p2pKey,
				signingKey,
				cometproxy.DefaultClientCreator(config.ProxyApp, config.ABCI, rollkitConfig.DBPath),
				genDoc,
				metrics,
				logger,
			)
			if err != nil {
				return fmt.Errorf("failed to create new rollkit node: %w", err)
			}

			// Launch the RPC server
			server := rollrpc.NewServer(rollnode, config.RPC, logger)
			err = server.Start()
			if err != nil {
				return fmt.Errorf("failed to launch RPC server: %w", err)
			}

			// Start the node
			if err := rollnode.Start(); err != nil {
				return fmt.Errorf("failed to start node: %w", err)
			}

			// TODO: Do rollkit nodes not have information about them? CometBFT has node.switch.NodeInfo()
			logger.Info("Started node")

			// Stop upon receiving SIGTERM or CTRL-C.
			cometos.TrapSignal(logger, func() {
				if rollnode.IsRunning() {
					if err := rollnode.Stop(); err != nil {
						logger.Error("unable to stop the node", "error", err)
					}
				}
			})

			// Check if we are running in CI mode
			inCI, err := cmd.Flags().GetBool("ci")
			if err != nil {
				return err
			}
			if !inCI {
				// Block forever
				select {}
			}

			return nil
		},
	}

	addNodeFlags(cmd)

	// use noop proxy app by default
	if !cmd.Flags().Lookup("proxy_app").Changed {
		config.ProxyApp = "noop"
	}

	// use aggregator by default
	if !cmd.Flags().Lookup("rollkit.aggregator").Changed {
		rollkitConfig.Aggregator = true
	}
	return cmd
}

// addNodeFlags exposes some common configuration options on the command-line
// These are exposed for convenience of commands embedding a rollkit node
func addNodeFlags(cmd *cobra.Command) {
	// Add cometBFT flags
	cmtcmd.AddNodeFlags(cmd)

	cmd.Flags().String("transport", config.ABCI, "specify abci transport (socket | grpc)")
	cmd.Flags().Bool("ci", false, "run node for ci testing")

	// Add Rollkit flags
	rollconf.AddFlags(cmd)
}

// startMockDAServJSONRPC starts a mock JSONRPC server
func startMockDAServJSONRPC(ctx context.Context) (*proxy.Server, error) {
	addr, _ := url.Parse(rollkitConfig.DAAddress)
	srv := proxy.NewServer(addr.Hostname(), addr.Port(), goDATest.NewDummyDA())
	err := srv.Start(ctx)
	if err != nil {
		return nil, err
	}
	return srv, nil
}

// TODO (Ferret-san): modify so that it initiates files with rollkit configurations by default
// note that such a change would also require changing the cosmos-sdk
func initFiles() error {
	// Generate the private validator config files
	cometprivvalKeyFile := config.PrivValidatorKeyFile()
	cometprivvalStateFile := config.PrivValidatorStateFile()
	var pv *cometprivval.FilePV
	if cometos.FileExists(cometprivvalKeyFile) {
		pv = cometprivval.LoadFilePV(cometprivvalKeyFile, cometprivvalStateFile)
		logger.Info("Found private validator", "keyFile", cometprivvalKeyFile,
			"stateFile", cometprivvalStateFile)
	} else {
		pv = cometprivval.GenFilePV(cometprivvalKeyFile, cometprivvalStateFile)
		pv.Save()
		logger.Info("Generated private validator", "keyFile", cometprivvalKeyFile,
			"stateFile", cometprivvalStateFile)
	}

	// Generate the node key config files
	nodeKeyFile := config.NodeKeyFile()
	if cometos.FileExists(nodeKeyFile) {
		logger.Info("Found node key", "path", nodeKeyFile)
	} else {
		if _, err := cometp2p.LoadOrGenNodeKey(nodeKeyFile); err != nil {
			return err
		}
		logger.Info("Generated node key", "path", nodeKeyFile)
	}

	// Generate the genesis file
	genFile := config.GenesisFile()
	if cometos.FileExists(genFile) {
		logger.Info("Found genesis file", "path", genFile)
	} else {
		genDoc := comettypes.GenesisDoc{
			ChainID:         fmt.Sprintf("test-rollup-%08x", rand.Uint32()), //nolint:gosec
			GenesisTime:     comettime.Now(),
			ConsensusParams: comettypes.DefaultConsensusParams(),
		}
		pubKey, err := pv.GetPubKey()
		if err != nil {
			return fmt.Errorf("can't get pubkey: %w", err)
		}
		genDoc.Validators = []comettypes.GenesisValidator{{
			Address: pubKey.Address(),
			PubKey:  pubKey,
			Power:   1000,
			Name:    "Rollkit Sequencer",
		}}

		if err := genDoc.SaveAs(genFile); err != nil {
			return err
		}
		logger.Info("Generated genesis file", "path", genFile)
	}

	return nil
}

// parseConfig retrieves the default environment configuration, sets up the
// Rollkit root and ensures that the root exists
func parseConfig(cmd *cobra.Command) error {
	// Set the root directory for the config to the home directory
	home := os.Getenv("RKHOME")
	if home == "" {
		var err error
		home, err = cmd.Flags().GetString(cometcli.HomeFlag)
		if err != nil {
			return err
		}
	}
	config.RootDir = home

	// Validate the root directory
	cometconf.EnsureRoot(config.RootDir)

	// Validate the config
	if err := config.ValidateBasic(); err != nil {
		return fmt.Errorf("error in config file: %w", err)
	}
	return nil
}
