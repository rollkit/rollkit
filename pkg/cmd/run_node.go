package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"cosmossdk.io/log"
	cometprivval "github.com/cometbft/cometbft/privval"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"

	seqGRPC "github.com/rollkit/go-sequencing/proxy/grpc"
	seqTest "github.com/rollkit/go-sequencing/test"

	coreda "github.com/rollkit/rollkit/core/da"
	coreexecutor "github.com/rollkit/rollkit/core/execution"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	"github.com/rollkit/rollkit/node"
	rollconf "github.com/rollkit/rollkit/pkg/config"
	genesispkg "github.com/rollkit/rollkit/pkg/genesis"
	rollos "github.com/rollkit/rollkit/pkg/os"
	"github.com/rollkit/rollkit/pkg/signer"
)

var (
	// initialize the rollkit node configuration
	nodeConfig = rollconf.DefaultNodeConfig

	// initialize the logger with the cometBFT defaults
	logger = log.NewLogger(os.Stdout)

	errSequencerAlreadyRunning = errors.New("sequencer already running")
)

// NewRunNodeCmd returns the command that allows the CLI to start a node.
func NewRunNodeCmd(
	executor coreexecutor.Executor,
	sequencer coresequencer.Sequencer,
	dac coreda.Client,
	keyProvider signer.KeyProvider,
) *cobra.Command {
	if executor == nil {
		panic("executor cannot be nil")
	}
	if sequencer == nil {
		panic("sequencer cannot be nil")
	}
	if dac == nil {
		panic("da client cannot be nil")
	}
	if keyProvider == nil {
		panic("key provider cannot be nil")
	}

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

			// Initialize the config files
			return initFiles()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel() // Ensure context is cancelled when command exits

			// Get the signing key
			signingKey, err := keyProvider.GetSigningKey()
			if err != nil {
				return fmt.Errorf("failed to get signing key: %w", err)
			}

			// initialize the metrics
			metrics := node.DefaultMetricsProvider(rollconf.DefaultInstrumentationConfig())

			sequencerRollupID := nodeConfig.Node.SequencerRollupID
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
			genesisPath := filepath.Join(nodeConfig.RootDir, nodeConfig.ConfigDir, "genesis.json")
			genesis, err := genesispkg.LoadGenesis(genesisPath)
			if err != nil {
				return fmt.Errorf("failed to load genesis: %w", err)
			}

			// create the rollkit node
			rollnode, err := node.NewNode(
				ctx,
				nodeConfig,
				executor,
				sequencer,
				dac,
				signingKey,
				genesis,
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
				rollos.TrapSignal(logger, func() {
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

// TODO (Ferret-san): modify so that it initiates files with rollkit configurations by default
// note that such a change would also require changing the cosmos-sdk
func initFiles() error {
	// Create config and data directories using nodeConfig values
	configDir := filepath.Join(nodeConfig.RootDir, nodeConfig.ConfigDir)
	dataDir := filepath.Join(nodeConfig.RootDir, nodeConfig.DBPath)

	if err := os.MkdirAll(configDir, rollconf.DefaultDirPerm); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	if err := os.MkdirAll(dataDir, rollconf.DefaultDirPerm); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// Generate the private validator config files
	cometprivvalKeyFile := filepath.Join(configDir, "priv_validator_key.json")
	cometprivvalStateFile := filepath.Join(dataDir, "priv_validator_state.json")
	if rollos.FileExists(cometprivvalKeyFile) {
		logger.Info("Found private validator", "keyFile", cometprivvalKeyFile,
			"stateFile", cometprivvalStateFile)
	} else {
		pv := cometprivval.GenFilePV(cometprivvalKeyFile, cometprivvalStateFile)
		pv.Save()
		logger.Info("Generated private validator", "keyFile", cometprivvalKeyFile,
			"stateFile", cometprivvalStateFile)
	}

	// Generate the genesis file
	genFile := filepath.Join(configDir, "genesis.json")
	if rollos.FileExists(genFile) {
		logger.Info("Found genesis file", "path", genFile)
	} else {
		// Create a default genesis
		genesis := genesispkg.NewGenesis(
			"test-chain",
			uint64(1),
			time.Now(),
			genesispkg.GenesisExtraData{}, // No proposer address for now
			nil,                           // No raw bytes for now
		)

		// Marshal the genesis struct directly
		genesisBytes, err := json.MarshalIndent(genesis, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal genesis: %w", err)
		}

		// Write genesis bytes directly to file
		if err := os.WriteFile(genFile, genesisBytes, 0600); err != nil {
			return fmt.Errorf("failed to write genesis file: %w", err)
		}
		logger.Info("Generated genesis file", "path", genFile)
	}

	return nil
}

func parseConfig(cmd *cobra.Command) error {
	// Load configuration with the correct order of precedence:
	// DefaultNodeConfig -> Yaml -> Flags
	var err error
	nodeConfig, err = rollconf.LoadNodeConfig(cmd)
	if err != nil {
		return err
	}

	// Validate the root directory
	if err := rollconf.EnsureRoot(nodeConfig.RootDir); err != nil {
		return err
	}

	// Setup logger with configuration from nodeConfig
	logger = setupLogger(nodeConfig)

	return nil
}

// setupLogger configures and returns a logger based on the provided configuration.
// It applies the following settings from the config:
//   - Log format (text or JSON)
//   - Log level (debug, info, warn, error)
//   - Stack traces for error logs
//
// The returned logger is already configured with the "module" field set to "main".
func setupLogger(config rollconf.Config) log.Logger {
	// Configure basic logger options
	var logOptions []log.Option

	// Configure logger format
	if config.Log.Format == "json" {
		logOptions = append(logOptions, log.OutputJSONOption())
	}

	// Configure logger level
	switch strings.ToLower(config.Log.Level) {
	case "debug":
		logOptions = append(logOptions, log.LevelOption(zerolog.DebugLevel))
	case "info":
		logOptions = append(logOptions, log.LevelOption(zerolog.InfoLevel))
	case "warn":
		logOptions = append(logOptions, log.LevelOption(zerolog.WarnLevel))
	case "error":
		logOptions = append(logOptions, log.LevelOption(zerolog.ErrorLevel))
	}

	// Configure stack traces
	if config.Log.Trace {
		logOptions = append(logOptions, log.TraceOption(true))
	}

	// Initialize logger with configured options
	configuredLogger := log.NewLogger(os.Stdout)
	if len(logOptions) > 0 {
		configuredLogger = log.NewLogger(os.Stdout, logOptions...)
	}

	// Add module to logger
	return configuredLogger.With("module", "main")
}
