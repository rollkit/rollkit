package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log/v2"
	"github.com/spf13/cobra"

	coreda "github.com/evstack/ev-node/core/da"
	coreexecutor "github.com/evstack/ev-node/core/execution"
	coresequencer "github.com/evstack/ev-node/core/sequencer"
	"github.com/evstack/ev-node/node"
	rollconf "github.com/evstack/ev-node/pkg/config"
	genesispkg "github.com/evstack/ev-node/pkg/genesis"
	"github.com/evstack/ev-node/pkg/p2p"
	"github.com/evstack/ev-node/pkg/signer"
	"github.com/evstack/ev-node/pkg/signer/file"
)

// ParseConfig is an helpers that loads the node configuration and validates it.
func ParseConfig(cmd *cobra.Command) (rollconf.Config, error) {
	nodeConfig, err := rollconf.Load(cmd)
	if err != nil {
		return rollconf.Config{}, fmt.Errorf("failed to load node config: %w", err)
	}

	if nodeConfig.Node.Pruning.Strategy != rollconf.PruningConfigStrategyCustom {
		pruningConfig := rollconf.GetPruningConfigFromStrategy(nodeConfig.Node.Pruning.Strategy)
		nodeConfig.Node.Pruning = pruningConfig
	}

	if err := nodeConfig.Validate(); err != nil {
		return rollconf.Config{}, fmt.Errorf("failed to validate node config: %w", err)
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
func SetupLogger(config rollconf.LogConfig) logging.EventLogger {
	logCfg := logging.Config{
		Stderr: true, // Default to stderr
	}

	// Configure logger format
	if config.Format == "json" {
		logCfg.Format = logging.JSONOutput
	}

	// Configure logger level
	level, err := logging.LevelFromString(config.Level)
	if err == nil {
		logCfg.Level = level
	} else {
		// Default to info if parsing fails
		logCfg.Level = logging.LevelInfo
	}

	logging.SetupLogging(logCfg)

	// Suppress noisy external component logs by default, unless debug logging is enabled
	if logCfg.Level != logging.LevelDebug {
		_ = logging.SetLogLevel("header/store", "FATAL")
		_ = logging.SetLogLevel("header/sync", "FATAL")
		_ = logging.SetLogLevel("header/p2p", "FATAL")
	}

	// Return a logger instance for the "main" subsystem
	return logging.Logger("main")
}

// StartNode handles the node startup logic
func StartNode(
	logger logging.EventLogger,
	cmd *cobra.Command,
	executor coreexecutor.Executor,
	sequencer coresequencer.Sequencer,
	da coreda.DA,
	p2pClient *p2p.Client,
	datastore datastore.Batching,
	nodeConfig rollconf.Config,
	nodeOptions node.NodeOptions,
) error {
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	// create a new remote signer
	var signer signer.Signer
	if nodeConfig.Signer.SignerType == "file" && nodeConfig.Node.Aggregator {
		passphrase, err := cmd.Flags().GetString(rollconf.FlagSignerPassphrase)
		if err != nil {
			return err
		}

		signer, err = file.LoadFileSystemSigner(nodeConfig.Signer.SignerPath, []byte(passphrase))
		if err != nil {
			return err
		}
	} else if nodeConfig.Signer.SignerType == "grpc" {
		panic("grpc remote signer not implemented")
	} else if nodeConfig.Node.Aggregator {
		return fmt.Errorf("unknown remote signer type: %s", nodeConfig.Signer.SignerType)
	}

	metrics := node.DefaultMetricsProvider(nodeConfig.Instrumentation)

	genesisPath := filepath.Join(filepath.Dir(nodeConfig.ConfigPath()), "genesis.json")
	genesis, err := genesispkg.LoadGenesis(genesisPath)
	if err != nil {
		return fmt.Errorf("failed to load genesis: %w", err)
	}

	// Create and start the node
	rollnode, err := node.NewNode(
		ctx,
		nodeConfig,
		executor,
		sequencer,
		da,
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

	// Wait for interrupt signal to gracefully shut down the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	select {
	case <-quit:
		logger.Info("shutting down node...")
		cancel()
	case err := <-errCh:
		logger.Error("node error", "error", err)
		cancel()
		return err
	}

	// Wait for node to finish shutting down
	select {
	case <-time.After(5 * time.Second):
		logger.Info("Node shutdown timed out")
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("Error during shutdown", "error", err)
			return err
		}
	}

	return nil
}
