package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/ipfs/go-datastore"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"

	"github.com/evstack/ev-node/block"
	coreexecutor "github.com/evstack/ev-node/core/execution"
	coresequencer "github.com/evstack/ev-node/core/sequencer"
	"github.com/evstack/ev-node/node"
	rollconf "github.com/evstack/ev-node/pkg/config"
	blobrpc "github.com/evstack/ev-node/pkg/da/jsonrpc"
	genesispkg "github.com/evstack/ev-node/pkg/genesis"
	"github.com/evstack/ev-node/pkg/p2p"
	"github.com/evstack/ev-node/pkg/p2p/key"
	pkgsigner "github.com/evstack/ev-node/pkg/signer"
	"github.com/evstack/ev-node/pkg/telemetry"
)

// ParseConfig is a helper function that loads the node configuration and validates it.
func ParseConfig(cmd *cobra.Command) (rollconf.Config, error) {
	nodeConfig, err := rollconf.Load(cmd)
	if err != nil {
		return rollconf.Config{}, fmt.Errorf("failed to load node config: %w", err)
	}

	if err := nodeConfig.Validate(); err != nil {
		return rollconf.Config{}, fmt.Errorf("failed to validate node config: %w", err)
	}

	return nodeConfig, nil
}

// ParseStartConfig loads and validates configuration required to start a node.
func ParseStartConfig(cmd *cobra.Command) (rollconf.Config, error) {
	nodeConfig, err := ParseConfig(cmd)
	if err != nil {
		return rollconf.Config{}, err
	}
	if (nodeConfig.Node.Aggregator || nodeConfig.Node.Promotable) && nodeConfig.GetDAAddress() == "" {
		return rollconf.Config{}, fmt.Errorf("DA address is required when aggregator or promotable mode is enabled")
	}

	return nodeConfig, nil
}

// SetupLogger configures and returns a logger based on the provided configuration.
// It applies the following settings from the config:
//   - Log format (text or JSON)
//   - Log level (debug, info, warn, error)
//   - Stack traces for error logs
//
// The returned logger is already configured with the "module" field set to "main".
func SetupLogger(config rollconf.LogConfig) zerolog.Logger {
	// Configure output
	var output = os.Stderr

	// Configure logger format
	var logger zerolog.Logger
	if config.Format == "json" {
		logger = zerolog.New(output)
	} else {
		logger = zerolog.New(zerolog.ConsoleWriter{Out: output})
	}

	// Configure logger level
	level, err := zerolog.ParseLevel(config.Level)
	if err != nil {
		// Default to info if parsing fails
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	// Add timestamp and set up logger with component
	logger = logger.With().Timestamp().Str("component", "main").Logger()

	return logger
}

// StartNode handles the node startup logic. daOpts configure the DA client,
// e.g. block.WithSubmitter to sign blob submissions locally.
func StartNode(
	logger zerolog.Logger,
	cmd *cobra.Command,
	executor coreexecutor.Executor,
	sequencer coresequencer.Sequencer,
	nodeKey *key.NodeKey,
	datastore datastore.Batching,
	nodeConfig rollconf.Config,
	genesis genesispkg.Genesis,
	nodeOptions node.NodeOptions,
	daOpts ...block.DAClientOption,
) error {
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	if nodeConfig.Instrumentation.IsTracingEnabled() {
		shutdownTracing, err := telemetry.InitTracing(ctx, nodeConfig.Instrumentation, logger)
		if err != nil {
			return fmt.Errorf("failed to initialize tracing: %w", err)
		}
		defer func() {
			// best-effort shutdown within a short timeout
			c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := shutdownTracing(c); err != nil {
				logger.Error().Err(err).Msg("failed to shutdown tracing")
			}
		}()
	}

	// Validate and load pkgsigner first (before attempting DA connection, which may fail
	// eagerly over WebSocket if no DA server is running).
	var signer pkgsigner.Signer
	needsSigner := nodeConfig.Node.Aggregator || nodeConfig.Node.Promotable
	if needsSigner && !nodeConfig.Node.BasedSequencer {
		passphrase := ""
		if nodeConfig.Signer.SignerType == "file" {
			passphraseFile, err := cmd.Flags().GetString(rollconf.FlagSignerPassphraseFile)
			if err != nil {
				return fmt.Errorf("failed to get '%s' flag: %w", rollconf.FlagSignerPassphraseFile, err)
			}

			if passphraseFile == "" {
				return fmt.Errorf("passphrase file must be provided via --evnode.signer.passphrase_file")
			}

			passphraseBytes, err := os.ReadFile(passphraseFile)
			if err != nil {
				return fmt.Errorf("failed to read passphrase from file '%s': %w", passphraseFile, err)
			}
			passphrase = strings.TrimSpace(string(passphraseBytes))

			if passphrase == "" {
				return fmt.Errorf("passphrase file '%s' is empty", passphraseFile)
			}
		}

		var err error
		signer, err = pkgsigner.NewSigner(ctx, &nodeConfig, passphrase)
		if err != nil {
			return fmt.Errorf("initialize signer via factory: %w", err)
		}

		if nodeConfig.Signer.SignerType == "kms" {
			switch nodeConfig.Signer.KMS.Provider {
			case "aws":
				logger.Info().Msg("initialized AWS KMS signer via factory")
			case "gcp":
				logger.Info().Msg("initialized GCP KMS signer via factory")
			default:
				logger.Info().Str("provider", nodeConfig.Signer.KMS.Provider).Msg("initialized KMS signer via factory")
			}
		}
	}

	var daClient block.FullDAClient
	if nodeConfig.DAEnabled() {
		blobClient, err := blobrpc.NewWSClient(ctx, logger, nodeConfig.GetDAAddress(), nodeConfig.DA.AuthToken, "")
		if err != nil {
			return fmt.Errorf("failed to create blob client: %w", err)
		}
		defer blobClient.Close()
		daClient = block.NewDAClient(blobClient, nodeConfig, logger, daOpts...)
	} else {
		logger.Info().Msg("DA address is not configured; syncing through P2P only")
	}

	// sanity check for based sequencer
	if nodeConfig.Node.BasedSequencer && genesis.DAStartHeight == 0 {
		return fmt.Errorf("based sequencing requires DAStartHeight to be set in genesis. This value should be identical for all nodes of the chain")
	}

	metrics := node.DefaultMetricsProvider(nodeConfig.Instrumentation)

	// wrap executor with tracing decorator if tracing enabled (outside core to keep it zero-dep)
	if nodeConfig.Instrumentation.IsTracingEnabled() {
		executor = telemetry.WithTracingExecutor(executor)
	}

	p2pClient, err := p2p.NewClient(nodeConfig.P2P, nodeKey.PrivKey, datastore, genesis.ChainID, logger, nil)
	if err != nil {
		return fmt.Errorf("create p2p client: %w", err)
	}

	// Create and start the node
	rollnode, err := node.NewNode(
		nodeConfig,
		executor,
		sequencer,
		daClient,
		signer,
		p2pClient,
		genesis,
		datastore,
		metrics,
		logger,
		nodeOptions,
	)
	if err != nil {
		return fmt.Errorf("failed to create node: %w", err)
	}

	// Run the node with graceful shutdown
	errCh := make(chan error, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				buf := make([]byte, 1024)
				n := runtime.Stack(buf, false)
				err := fmt.Errorf("node panicked: %v\nstack trace:\n%s", r, buf[:n])
				logger.Error().Interface("panic", r).Str("stacktrace", string(buf[:n])).Msg("Recovered from panic in node")
				select {
				case errCh <- err:
				default:
					logger.Error().Err(err).Msg("Error channel full")
				}
			}
		}()

		err := rollnode.Run(ctx)
		select {
		case errCh <- err:
		default:
			logger.Error().Err(err).Msg("Error channel full")
		}
	}()

	// Wait for interrupt signal to gracefully shut down the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	select {
	case <-quit:
		logger.Info().Msg("shutting down node...")
		// Proactively resign Raft leadership before cancelling the worker context.
		// This gives the cluster a chance to elect a new leader before this node
		// stops producing blocks, shrinking the unconfirmed-block window.
		if resigner, ok := rollnode.(node.LeaderResigner); ok {
			resignCtx, resignCancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer resignCancel()
			if err := resigner.ResignLeader(resignCtx); err != nil {
				if errors.Is(err, context.DeadlineExceeded) {
					logger.Warn().Msg("leadership resign timed out")
				} else {
					logger.Warn().Err(err).Msg("leadership resign on shutdown failed")
				}
			} else {
				logger.Info().Msg("leadership resigned before shutdown")
			}
		}
		cancel()
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			logger.Error().Err(err).Msg("node error")
		}
		cancel()
		return err
	}

	// Wait for node to finish shutting down after signal
	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			logger.Error().Err(err).Msg("Error during shutdown")
			return err
		}
	case <-time.After(10 * time.Second):
		return fmt.Errorf("shutdown timeout exceeded")
	}

	return nil
}
