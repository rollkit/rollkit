package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cosmossdk.io/log"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	coreda "github.com/rollkit/rollkit/core/da"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	"github.com/rollkit/rollkit/da"
	"github.com/rollkit/rollkit/node"
	rollconf "github.com/rollkit/rollkit/pkg/config"
	genesispkg "github.com/rollkit/rollkit/pkg/genesis"
	rollos "github.com/rollkit/rollkit/pkg/os"
	"github.com/rollkit/rollkit/pkg/signer"
	"github.com/rollkit/rollkit/pkg/signer/file"
	testExecutor "github.com/rollkit/rollkit/test/executors/kv"
)

var (
	// initialize the rollkit node configuration
	nodeConfig = rollconf.DefaultNodeConfig

	// initialize the logger with the cometBFT defaults
	logger = log.NewLogger(os.Stdout)
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

			// Initialize the config files
			return initFiles()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel() // Ensure context is cancelled when command exits

			// Check if passphrase is required and provided
			if nodeConfig.Node.Aggregator && nodeConfig.Signer.SignerType == "file" {
				passphrase, err := cmd.Flags().GetString(rollconf.FlagSignerPassphrase)
				if err != nil {
					return err
				}
				if passphrase == "" {
					return fmt.Errorf("passphrase is required for aggregator nodes using local file signer")
				}
			}

			kvExecutor := createDirectKVExecutor(ctx)

			// initialize the metrics
			metrics := node.DefaultMetricsProvider(rollconf.DefaultInstrumentationConfig())

			logger.Info("Executor address", "address", nodeConfig.Node.ExecutorAddress)

			//create a new remote signer
			var signer signer.Signer
			if nodeConfig.Signer.SignerType == "file" {
				passphrase, err := cmd.Flags().GetString(rollconf.FlagSignerPassphrase)
				if err != nil {
					return err
				}

				signer, err = file.NewFileSystemSigner(nodeConfig.Signer.SignerPath, []byte(passphrase))
				if err != nil {
					return err
				}
			} else if nodeConfig.Signer.SignerType == "grpc" {
				panic("grpc remote signer not implemented")
			} else {
				return fmt.Errorf("unknown remote signer type: %s", nodeConfig.Signer.SignerType)
			}

			dummySequencer := coresequencer.NewDummySequencer()

			genesisPath := filepath.Join(nodeConfig.RootDir, nodeConfig.ConfigDir, "genesis.json")
			genesis, err := genesispkg.LoadGenesis(genesisPath)
			if err != nil {
				return fmt.Errorf("failed to load genesis: %w", err)
			}

			dummyDA := coreda.NewDummyDA(100_000, 0, 0)
			dummyDALC := da.NewDAClient(dummyDA, nodeConfig.DA.GasPrice, nodeConfig.DA.GasMultiplier, []byte(nodeConfig.DA.Namespace), []byte(nodeConfig.DA.SubmitOptions), logger)
			// create the rollkit node
			rollnode, err := node.NewNode(
				ctx,
				nodeConfig,
				// THIS IS FOR TESTING ONLY
				kvExecutor,
				dummySequencer,
				dummyDALC,
				signer,
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
	// Create config and data directories using nodeConfig values
	configDir := filepath.Join(nodeConfig.RootDir, nodeConfig.ConfigDir)
	dataDir := filepath.Join(nodeConfig.RootDir, nodeConfig.DBPath)

	if err := os.MkdirAll(configDir, rollconf.DefaultDirPerm); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	if err := os.MkdirAll(dataDir, rollconf.DefaultDirPerm); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
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
