package commands

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"cosmossdk.io/log"
	cometos "github.com/cometbft/cometbft/libs/os"
	cometp2p "github.com/cometbft/cometbft/p2p"
	cometprivval "github.com/cometbft/cometbft/privval"
	comettypes "github.com/cometbft/cometbft/types"
	comettime "github.com/cometbft/cometbft/types/time"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"google.golang.org/grpc"

	seqGRPC "github.com/rollkit/go-sequencing/proxy/grpc"
	seqTest "github.com/rollkit/go-sequencing/test"

	rollconf "github.com/rollkit/rollkit/config"
	coreda "github.com/rollkit/rollkit/core/da"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	"github.com/rollkit/rollkit/da"
	"github.com/rollkit/rollkit/node"
	testExecutor "github.com/rollkit/rollkit/test/executors/kv"
	rolltypes "github.com/rollkit/rollkit/types"
)

var (
	// initialize the rollkit node configuration
	nodeConfig = rollconf.DefaultNodeConfig

	// initialize the logger with the cometBFT defaults
	logger = log.NewLogger(os.Stdout)

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

			// Determine which rollupID to use. If the flag has been set we want to use that value and ensure that the chainID in the genesis doc matches.
			if cmd.Flags().Lookup(rollconf.FlagSequencerRollupID).Changed {
				genDoc.ChainID = nodeConfig.Node.SequencerRollupID
			}
			sequencerRollupID := genDoc.ChainID
			// Try and launch a mock gRPC sequencer if there is no sequencer running.
			// Only start mock Sequencer if the user did not provide --rollkit.sequencer_address
			var seqSrv *grpc.Server = nil
			if !cmd.Flags().Lookup(rollconf.FlagSequencerAddress).Changed {
				seqSrv, err = tryStartMockSequencerServerGRPC(nodeConfig.Node.SequencerAddress, sequencerRollupID)
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

			logger.Info("Executor address", "address", nodeConfig.Node.ExecutorAddress)

			// Create a cancellable context for the node
			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel() // Ensure context is cancelled when command exits

			kvExecutor := createDirectKVExecutor(ctx)
			dummySequencer := coresequencer.NewDummySequencer()
			dummyDA := coreda.NewDummyDA(100_000)
			dummyDALC := da.NewDAClient(dummyDA, nodeConfig.DA.GasPrice, nodeConfig.DA.GasMultiplier, []byte(nodeConfig.DA.Namespace), []byte(nodeConfig.DA.SubmitOptions), logger)
			// create the rollkit node
			rollnode, err := node.NewNode(
				ctx,
				nodeConfig,
				// THIS IS FOR TESTING ONLY
				kvExecutor,
				dummySequencer,
				dummyDALC,
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
	cmd.Flags().Bool("ci", false, "run node for ci testing")

	// This is for testing only
	cmd.Flags().String("kv-executor-http", ":40042", "address for the KV executor HTTP server (empty to disable)")

	// Add Rollkit flags
	rollconf.AddFlags(cmd)
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
func createDirectKVExecutor(ctx context.Context) *testExecutor.KVExecutor {
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
		logger.Info("Creating KV Executor HTTP server", "address", httpAddr)
		go func() {
			logger.Info("Starting KV Executor HTTP server", "address", httpAddr)
			if err := httpServer.Start(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Error("KV Executor HTTP server error", "error", err)
			}
			logger.Info("KV Executor HTTP server stopped", "address", httpAddr)
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
	// Load configuration with the correct order of precedence:
	// DefaultNodeConfig -> Toml -> Flags
	var err error
	nodeConfig, err = rollconf.LoadNodeConfig(cmd)
	if err != nil {
		return err
	}

	// Validate the root directory
	rollconf.EnsureRoot(nodeConfig.RootDir)

	return nil
}

// RollkitGenesisDocProviderFunc returns a function that loads the GenesisDoc from the filesystem
// using nodeConfig instead of config.
func RollkitGenesisDocProviderFunc(nodeConfig rollconf.Config) func() (*comettypes.GenesisDoc, error) {
	return func() (*comettypes.GenesisDoc, error) {
		// Construct the genesis file path using rootify
		genFile := filepath.Join(nodeConfig.RootDir, "config", "genesis.json")
		return comettypes.GenesisDocFromFile(genFile)
	}
}
