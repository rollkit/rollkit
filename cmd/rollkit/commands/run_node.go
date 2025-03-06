package commands

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/mitchellh/mapstructure"

	"cosmossdk.io/log"
	cmtcmd "github.com/cometbft/cometbft/cmd/cometbft/commands"
	cometcli "github.com/cometbft/cometbft/libs/cli"
	cometos "github.com/cometbft/cometbft/libs/os"
	cometp2p "github.com/cometbft/cometbft/p2p"
	cometprivval "github.com/cometbft/cometbft/privval"
	comettypes "github.com/cometbft/cometbft/types"
	comettime "github.com/cometbft/cometbft/types/time"
	"github.com/rollkit/go-da"
	proxy "github.com/rollkit/go-da/proxy/jsonrpc"
	goDATest "github.com/rollkit/go-da/test"
	seqGRPC "github.com/rollkit/go-sequencing/proxy/grpc"
	seqTest "github.com/rollkit/go-sequencing/test"
	rollconf "github.com/rollkit/rollkit/config"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	"github.com/rollkit/rollkit/node"
	testExecutor "github.com/rollkit/rollkit/test/executors/kv"
	rolltypes "github.com/rollkit/rollkit/types"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
)

var (
	// initialize the rollkit node configuration
	nodeConfig = rollconf.DefaultNodeConfig

	// initialize the logger with the cometBFT defaults
	logger = log.NewLogger(os.Stdout)

	errDAServerAlreadyRunning  = errors.New("DA server already running")
	errSequencerAlreadyRunning = errors.New("sequencer already running")
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

			// Update log format if the flag is set
			// if config.LogFormat == cometconf.LogFormatJSON {
			// 	logger = cometlog.NewTMJSONLogger(cometlog.NewSyncWriter(os.Stdout))
			// }

			// // Parse the log level
			// logger, err = cometflags.ParseLogLevel(config.LogLevel, logger, cometconf.DefaultLogLevel)
			// if err != nil {
			// 	return err
			// }

			// // Add tracing to the logger if the flag is set
			// if viper.GetBool(cometcli.TraceFlag) {
			// 	logger = cometlog.NewTracingLogger(logger)
			// }

			logger = logger.With("module", "main")

			// Initialize the config files
			return initFiles()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			genDocProvider := RollkitGenesisDocProviderFunc(nodeConfig)
			genDoc, err := genDocProvider()
			if err != nil {
				return err
			}
			nodeKeyFile := filepath.Join(nodeConfig.RootDir, "config", "node_key.json")
			nodeKey, err := cometp2p.LoadOrGenNodeKey(nodeKeyFile)
			if err != nil {
				return err
			}
			privValidatorKeyFile := filepath.Join(nodeConfig.RootDir, "config", "priv_validator_key.json")
			privValidatorStateFile := filepath.Join(nodeConfig.RootDir, "data", "priv_validator_state.json")
			pval := cometprivval.LoadOrGenFilePV(privValidatorKeyFile, privValidatorStateFile)
			p2pKey, err := rolltypes.GetNodeKey(nodeKey)
			if err != nil {
				return err
			}
			signingKey, err := rolltypes.GetNodeKey(&cometp2p.NodeKey{PrivKey: pval.Key.PrivKey})
			if err != nil {
				return err
			}

			// initialize the metrics
			metrics := node.DefaultMetricsProvider(rollconf.DefaultInstrumentationConfig())

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
			if cmd.Flags().Lookup(rollconf.FlagSequencerRollupID).Changed {
				genDoc.ChainID = nodeConfig.SequencerRollupID
			}
			sequencerRollupID := genDoc.ChainID
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

			logger.Info("Executor address", "address", nodeConfig.ExecutorAddress)

			// Create a cancellable context for the node
			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel() // Ensure context is cancelled when command exits

			kvExecutor := createDirectKVExecutor()
			dummySequencer := coresequencer.NewDummySequencer()

			// create the rollkit node
			rollnode, err := node.NewNode(
				ctx,
				nodeConfig,
				// THIS IS FOR TESTING ONLY
				kvExecutor,
				dummySequencer,
				p2pKey,
				signingKey,
				genDoc,
				metrics,
				logger,
			)
			if err != nil {
				return fmt.Errorf("failed to create new rollkit node: %w", err)
			}

			// Create error channel and signal channel
			errCh := make(chan error, 1)
			shutdownCh := make(chan struct{})

			// Start the node in a goroutine
			go func() {
				defer func() {
					if r := recover(); r != nil {
						err := fmt.Errorf("node panicked: %v", r)
						logger.Error("Recovered from panic in node", "panic", r)
						select {
						case errCh <- err:
						default:
							logger.Error("Error channel full", "error", err)
						}
					}
				}()

				err := rollnode.Run(ctx)
				select {
				case errCh <- err:
				default:
					logger.Error("Error channel full", "error", err)
				}
			}()

			// Wait a moment to check for immediate startup errors
			time.Sleep(100 * time.Millisecond)

			// Check if the node stopped immediately
			select {
			case err := <-errCh:
				return fmt.Errorf("failed to start node: %w", err)
			default:
				// This is expected - node is running
				logger.Info("Started node")
			}

			// Stop upon receiving SIGTERM or CTRL-C.
			go func() {
				cometos.TrapSignal(logger, func() {
					logger.Info("Received shutdown signal")
					cancel() // Cancel context to stop the node
					close(shutdownCh)
				})
			}()

			// Check if we are running in CI mode
			inCI, err := cmd.Flags().GetBool("ci")
			if err != nil {
				return err
			}

			if !inCI {
				// Block until either the node exits with an error or a shutdown signal is received
				select {
				case err := <-errCh:
					return fmt.Errorf("node exited with error: %w", err)
				case <-shutdownCh:
					// Wait for the node to clean up
					select {
					case <-time.After(5 * time.Second):
						logger.Info("Node shutdown timed out")
					case err := <-errCh:
						if err != nil && !errors.Is(err, context.Canceled) {
							logger.Error("Error during shutdown", "error", err)
						}
					}
					return nil
				}
			}

			// CI mode. Wait for 1s and then verify the node is running before cancelling context
			time.Sleep(1 * time.Second)

			// Check if the node is still running
			select {
			case err := <-errCh:
				return fmt.Errorf("node stopped unexpectedly in CI mode: %w", err)
			default:
				// Node is still running, which is what we want
				logger.Info("Node running successfully in CI mode, shutting down")
			}

			// Cancel the context to stop the node
			cancel()

			// Wait for the node to exit with a timeout
			select {
			case <-time.After(5 * time.Second):
				return fmt.Errorf("node shutdown timed out in CI mode")
			case err := <-errCh:
				if err != nil && !errors.Is(err, context.Canceled) {
					return fmt.Errorf("error during node shutdown in CI mode: %w", err)
				}
				return nil
			}
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

	cmd.Flags().Bool("ci", false, "run node for ci testing")

	// This is for testing only
	cmd.Flags().String("kv-executor-http", ":40042", "address for the KV executor HTTP server (empty to disable)")

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

// createDirectKVExecutor creates a KVExecutor for testing
func createDirectKVExecutor() *testExecutor.KVExecutor {
	kvExecutor := testExecutor.NewKVExecutor()

	// Pre-populate with some test transactions
	for i := 0; i < 5; i++ {
		tx := []byte(fmt.Sprintf("test%d=value%d", i, i))
		kvExecutor.InjectTx(tx)
	}

	// Start HTTP server for transaction submission if address is specified
	httpAddr := viper.GetString("kv-executor-http")
	if httpAddr != "" {
		httpServer := testExecutor.NewHTTPServer(kvExecutor, httpAddr)
		go func() {
			fmt.Printf("Starting KV Executor HTTP server on %s\n", httpAddr)
			if err := httpServer.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				fmt.Printf("KV Executor HTTP server error: %v\n", err)
			}
		}()
	}

	return kvExecutor
}

// TODO (Ferret-san): modify so that it initiates files with rollkit configurations by default
// note that such a change would also require changing the cosmos-sdk
func initFiles() error {
	// Generate the private validator config files
	cometprivvalKeyFile := filepath.Join(nodeConfig.RootDir, "config", "priv_validator_key.json")
	cometprivvalStateFile := filepath.Join(nodeConfig.RootDir, "data", "priv_validator_state.json")
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
	nodeKeyFile := filepath.Join(nodeConfig.RootDir, "config", "node_key.json")
	if cometos.FileExists(nodeKeyFile) {
		logger.Info("Found node key", "path", nodeKeyFile)
	} else {
		if _, err := cometp2p.LoadOrGenNodeKey(nodeKeyFile); err != nil {
			return err
		}
		logger.Info("Generated node key", "path", nodeKeyFile)
	}

	// Generate the genesis file
	genFile := filepath.Join(nodeConfig.RootDir, "config", "genesis.json")
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
	nodeConfig.RootDir = home

	// Validate the root directory
	rollconf.EnsureRoot(nodeConfig.RootDir)

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
	err := v.Unmarshal(&nodeConfig, func(c *mapstructure.DecoderConfig) {
		c.TagName = "mapstructure"
		c.DecodeHook = mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
		)
	})
	if err != nil {
		return fmt.Errorf("unable to decode command flags into config: %w", err)
	}

	// handle rollkit node configuration
	if err := nodeConfig.GetViperConfig(v); err != nil {
		return fmt.Errorf("unable to decode command flags into nodeConfig: %w", err)
	}

	return nil
}

// RollkitGenesisDocProviderFunc returns a function that loads the GenesisDoc from the filesystem
// using nodeConfig instead of config.
func RollkitGenesisDocProviderFunc(nodeConfig rollconf.NodeConfig) func() (*comettypes.GenesisDoc, error) {
	return func() (*comettypes.GenesisDoc, error) {
		// Construct the genesis file path using rootify
		genFile := filepath.Join(nodeConfig.RootDir, "config", "genesis.json")
		return comettypes.GenesisDocFromFile(genFile)
	}
}
