package commands

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/url"
	"os"
	"syscall"
	"time"

	"cosmossdk.io/log"
	cmtcmd "github.com/cometbft/cometbft/cmd/cometbft/commands"
	cometcli "github.com/cometbft/cometbft/libs/cli"
	cometos "github.com/cometbft/cometbft/libs/os"
	cometp2p "github.com/cometbft/cometbft/p2p"
	cometprivval "github.com/cometbft/cometbft/privval"
	comettypes "github.com/cometbft/cometbft/types"
	comettime "github.com/cometbft/cometbft/types/time"
	"github.com/mitchellh/mapstructure"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"google.golang.org/grpc"

	"github.com/rollkit/go-da"
	proxy "github.com/rollkit/go-da/proxy/jsonrpc"
	goDATest "github.com/rollkit/go-da/test"
	execGRPC "github.com/rollkit/go-execution/proxy/grpc"
	execTest "github.com/rollkit/go-execution/test"
	execTypes "github.com/rollkit/go-execution/types"
	pb "github.com/rollkit/go-execution/types/pb/execution"
	seqGRPC "github.com/rollkit/go-sequencing/proxy/grpc"
	seqTest "github.com/rollkit/go-sequencing/test"

	rollconf "github.com/rollkit/rollkit/config"
	rollnode "github.com/rollkit/rollkit/node"
	rolltypes "github.com/rollkit/rollkit/types"
)

var (
	// initialize the config with the rollkit defaults
	config = rollconf.DefaultConfig()

	// initialize the rollkit node configuration
	nodeConfig = rollconf.DefaultNodeConfig

	// initialize the logger with the cometBFT defaults
	logger = log.NewLogger(os.Stdout)

	errDAServerAlreadyRunning  = errors.New("DA server already running")
	errSequencerAlreadyRunning = errors.New("sequencer already running")
	errExecutorAlreadyRunning  = errors.New("executor already running")
)

// NewRunNodeCmd returns the command that allows the CLI to start a node.
func NewRunNodeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "start",
		Aliases: []string{"node", "run"},
		Short:   "Run the rollkit node",
		// PersistentPreRunE is used to parse the config and initial the config files
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			err := parseConfig(cmd)
			if err != nil {
				return err
			}

			// use aggregator by default if the flag is not specified explicitly
			if !cmd.Flags().Lookup("rollkit.aggregator").Changed {
				nodeConfig.Aggregator = true
			}

			logger = logger.With("module", "main")

			// Initialize the config files
			return initFiles()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			genDoc, err := rollconf.LoadGenesisDoc(config.GenesisFile())
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
			rollconf.GetNodeConfig(&nodeConfig)
			if err := rollconf.TranslateAddresses(&nodeConfig); err != nil {
				return err
			}

			// initialize the metrics
			metrics := rollnode.DefaultMetricsProvider(rollconf.DefaultInstrumentationConfig())

			// Try and launch a mock JSON RPC DA server if there is no DA server running.
			// Only start mock DA server if the user did not provide --rollkit.da_address
			var daSrv *proxy.Server = nil
			if !cmd.Flags().Lookup("rollkit.da_address").Changed {
				daSrv, err = tryStartMockDAServJSONRPC(cmd.Context(), nodeConfig.DAAddress, proxy.NewServer)
				if err != nil && !errors.Is(err, errDAServerAlreadyRunning) {
					return fmt.Errorf("failed to launch mock da server: %w", err)
				}
				// nolint:errcheck,gosec
				defer func() {
					if daSrv != nil {
						daSrv.Stop(cmd.Context())
					}
				}()
			}

			// Determine which rollupID to use. If the flag has been set we want to use that value and ensure that the chainID in the genesis doc matches.
			// if cmd.Flags().Lookup(rollconf.FlagSequencerRollupID).Changed {
			// 	genDoc.ChainID = nodeConfig.SequencerRollupID
			// }

			sequencerRollupID := genDoc.GetChainID()
			// Try and launch a mock gRPC sequencer if there is no sequencer running.
			// Only start mock Sequencer if the user did not provide --rollkit.sequencer_address
			var seqSrv *grpc.Server = nil
			if !cmd.Flags().Lookup(rollconf.FlagSequencerAddress).Changed {
				seqSrv, err = tryStartMockSequencerServerGRPC(nodeConfig.SequencerAddress, sequencerRollupID)
				if err != nil && !errors.Is(err, errSequencerAlreadyRunning) {
					return fmt.Errorf("failed to launch mock sequencing server: %w", err)
				}
				// nolint:errcheck,gosec
				defer func() {
					if seqSrv != nil {
						seqSrv.Stop()
					}
				}()
			}

			// Try and launch a mock gRPC executor if there is no executor running.
			// Only start mock Executor if the user did not provide --rollkit.executor_address
			var execSrv *grpc.Server = nil
			if !cmd.Flags().Lookup("rollkit.executor_address").Changed {
				execSrv, err = tryStartMockExecutorServerGRPC(nodeConfig.ExecutorAddress)
				if err != nil && !errors.Is(err, errExecutorAlreadyRunning) {
					return fmt.Errorf("failed to launch mock executor server: %w", err)
				}
				// nolint:errcheck,gosec
				defer func() {
					if execSrv != nil {
						execSrv.Stop()
					}
				}()
			}

			logger.Info("Executor address", "address", nodeConfig.ExecutorAddress)

			// use noop proxy app by default
			if !cmd.Flags().Lookup("proxy_app").Changed {
				config.ProxyApp = "noop"
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// create the rollkit node
			rollnode, err := rollnode.NewNode(
				ctx,
				nodeConfig,
				p2pKey,
				signingKey,
				genDoc,
				metrics,
				logger,
			)
			if err != nil {
				return fmt.Errorf("failed to create new rollkit node: %w", err)
			}

			// Start the node
			if err := rollnode.Start(ctx); err != nil {
				return fmt.Errorf("failed to start node: %w", err)
			}

			// TODO: Do rollkit nodes not have information about them? CometBFT has node.switch.NodeInfo()
			logger.Info("Started node")

			// Stop upon receiving SIGTERM or CTRL-C.
			cometos.TrapSignal(logger, func() {
				if rollnode.IsRunning() {
					if err := rollnode.Stop(ctx); err != nil {
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
				// Block forever to force user to stop node
				select {}
			}

			// CI mode. Wait for 5s and then verify the node is running before calling stop node.
			time.Sleep(5 * time.Second)
			if !rollnode.IsRunning() {
				return fmt.Errorf("node is not running")
			}

			return rollnode.Stop(ctx)
		},
	}

	addNodeFlags(cmd)

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

// tryStartMockDAServJSONRPC will try and start a mock JSONRPC server
func tryStartMockDAServJSONRPC(
	ctx context.Context,
	daAddress string,
	newServer func(string, string, da.DA) *proxy.Server,
) (*proxy.Server, error) {
	addr, err := url.Parse(daAddress)
	if err != nil {
		return nil, err
	}

	srv := newServer(addr.Hostname(), addr.Port(), goDATest.NewDummyDA())
	if err := srv.Start(ctx); err != nil {
		if errors.Is(err, syscall.EADDRINUSE) {
			logger.Info("DA server is already running", "address", daAddress)
			return nil, errDAServerAlreadyRunning
		}
		return nil, err
	}

	logger.Info("Starting mock DA server", "address", daAddress)

	return srv, nil
}

// tryStartMockSequencerServerGRPC will try and start a mock gRPC server with the given listenAddress.
func tryStartMockSequencerServerGRPC(listenAddress string, rollupId string) (*grpc.Server, error) {
	dummySeq := seqTest.NewDummySequencer([]byte(rollupId))
	server := seqGRPC.NewServer(dummySeq, dummySeq, dummySeq)
	lis, err := net.Listen("tcp", listenAddress)
	if err != nil {
		if errors.Is(err, syscall.EADDRINUSE) || errors.Is(err, syscall.EADDRNOTAVAIL) {
			logger.Info(errSequencerAlreadyRunning.Error(), "address", listenAddress)
			logger.Info("make sure your rollupID matches your sequencer", "rollupID", rollupId)
			return nil, errSequencerAlreadyRunning
		}
		return nil, err
	}
	go func() {
		_ = server.Serve(lis)
	}()
	logger.Info("Starting mock sequencer", "address", listenAddress, "rollupID", rollupId)
	return server, nil
}

// tryStartMockExecutorServerGRPC will try and start a mock gRPC executor server
func tryStartMockExecutorServerGRPC(listenAddress string) (*grpc.Server, error) {
	dummyExec := execTest.NewDummyExecutor()

	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		i := 0
		for range ticker.C {
			dummyExec.InjectTx(execTypes.Tx{byte(3*i + 1), byte(3*i + 2), byte(3*i + 3)})
			i++
		}
	}()

	execServer := execGRPC.NewServer(dummyExec, nil)
	server := grpc.NewServer()
	pb.RegisterExecutionServiceServer(server, execServer)
	lis, err := net.Listen("tcp", listenAddress)
	if err != nil {
		if errors.Is(err, syscall.EADDRINUSE) {
			logger.Info("Executor server already running", "address", listenAddress)
			return nil, errExecutorAlreadyRunning
		}
		return nil, err
	}
	go func() {
		_ = server.Serve(lis)
	}()
	logger.Info("Starting mock executor", "address", listenAddress)
	return server, nil
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
	rollconf.EnsureRoot(config.RootDir)

	// Validate the config
	if err := config.ValidateBasic(); err != nil {
		return fmt.Errorf("error in config file: %w", err)
	}

	// Parse the flags
	if err := parseFlags(cmd); err != nil {
		return err
	}

	return nil
}

func parseFlags(cmd *cobra.Command) error {
	v := viper.GetViper()
	if err := v.BindPFlags(cmd.Flags()); err != nil {
		return err
	}

	// unmarshal viper into config
	err := v.Unmarshal(&config, func(c *mapstructure.DecoderConfig) {
		c.TagName = "mapstructure"
		c.DecodeHook = mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
		)
	})
	if err != nil {
		return fmt.Errorf("unable to decode command flags into config: %w", err)
	}

	// special handling for the p2p external address, due to inconsistencies in mapstructure and flag name
	if cmd.Flags().Lookup("p2p.external-address").Changed {
		config.P2P.ExternalAddress = viper.GetString("p2p.external-address")
	}

	// handle rollkit node configuration
	if err := nodeConfig.GetViperConfig(v); err != nil {
		return fmt.Errorf("unable to decode command flags into nodeConfig: %w", err)
	}

	return nil
}
