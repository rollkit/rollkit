package main

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hashicorp/raft"
	"github.com/spf13/cobra"

	"github.com/rollkit/rollkit/attester/internal/aggregator"
	"github.com/rollkit/rollkit/attester/internal/config"
	"github.com/rollkit/rollkit/attester/internal/fsm"
	internalgrpc "github.com/rollkit/rollkit/attester/internal/grpc"
	internalraft "github.com/rollkit/rollkit/attester/internal/raft"
	"github.com/rollkit/rollkit/attester/internal/signing"
	"github.com/rollkit/rollkit/attester/internal/verification"
)

// Constants for flag names and defaults
const (
	flagConfig   = "config"
	flagLogLevel = "log-level"
	flagLeader   = "leader"

	defaultConfigPath = "attester.yaml"
	defaultLogLevel   = "info"
)

var rootCmd = &cobra.Command{
	Use:   "attesterd",
	Short: "Attester node daemon",
	Long:  `A daemon process that participates in the RAFT consensus to attest blocks.`,
	Run:   runNode,
}

var (
	configFile string
	logLevel   string
	isLeader   bool
)

func init() {
	rootCmd.PersistentFlags().StringVar(&configFile, flagConfig, defaultConfigPath, "Path to the configuration file")
	rootCmd.PersistentFlags().StringVar(&logLevel, flagLogLevel, defaultLogLevel, "Logging level (e.g., debug, info, warn, error)")
	rootCmd.PersistentFlags().BoolVar(&isLeader, flagLeader, false, "Run the node as the RAFT leader (sequencer)")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error executing command: %v\n", err)
		os.Exit(1)
	}
}

// runNode sets up and runs the attester node.
func runNode(cmd *cobra.Command, args []string) {
	logger := setupLogger()

	slog.Info("Initializing attester node", "role", map[bool]string{true: "LEADER", false: "FOLLOWER"}[isLeader])

	// 1. Load configuration
	slog.Info("Loading configuration", "path", configFile)
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	// 2. Initialize Execution Verifier
	execVerifier := setupVerifier(cfg.Execution, logger)

	// 3. Load private key
	signer, err := setupSigner(cfg.Signing, logger)
	if err != nil {
		slog.Error("Failed to set up signer", "error", err)
		os.Exit(1)
	}
	slog.Info("Loaded signing key", "scheme", cfg.Signing.Scheme)

	// Declare components
	var sigAggregator *aggregator.SignatureAggregator
	var sigClient *internalgrpc.Client
	var attesterFSM *fsm.AttesterFSM
	var raftNode *raft.Raft
	var transport raft.Transport
	var grpcServer *internalgrpc.AttesterServer

	// 4. Initialize Role-Specific Components (Aggregator or gRPC Client)
	sigAggregator, sigClient, err = setupSignalingComponents(cfg, logger, isLeader, signer)
	if err != nil {
		slog.Error("Failed to set up signaling components", "error", err)
		os.Exit(1)
	}
	// Defer client closing if it was initialized
	if sigClient != nil {
		defer func() {
			if err := sigClient.Close(); err != nil {
				slog.Error("Error closing signature client", "error", err)
			}
		}()
	}

	// 5. Instantiate FSM
	slog.Info("Initializing FSM", "node_id", cfg.Node.ID, "is_leader", isLeader)
	attesterFSM, err = setupFSM(logger, signer, cfg.Node.ID, isLeader, sigAggregator, sigClient, execVerifier)
	if err != nil {
		slog.Error("Failed to initialize FSM", "error", err)
		os.Exit(1)
	}

	// 6. Instantiate RAFT node (Requires FSM)
	slog.Info("Initializing RAFT node...")
	raftNode, transport, err = setupRaft(cfg, attesterFSM, logger)
	if err != nil {
		slog.Error("Failed to initialize RAFT node", "error", err)
		os.Exit(1)
	}

	// 7. Initialize and Start gRPC Server (Leader only, requires RAFT node)
	if isLeader {
		slog.Info("Initializing gRPC server...", "address", cfg.GRPC.ListenAddress)
		grpcServer = internalgrpc.NewAttesterServer(raftNode, logger, sigAggregator)

		slog.Info("Starting gRPC server in background...")
		go func() {
			listenAddr := cfg.GRPC.ListenAddress
			if listenAddr == "" {
				slog.Error("grpc.listen_address is required in config for the leader")
				return
			}
			slog.Info("gRPC server goroutine starting", "address", listenAddr)
			if err := grpcServer.Start(listenAddr); err != nil {
				slog.Error("gRPC server failed to start or encountered error", "error", err)
			}
			slog.Info("gRPC server goroutine finished")
		}()
	}

	// --- Node is running ---

	// 8. Handle graceful shutdown
	slog.Info("Attester node running. Press Ctrl+C to exit.")
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan // Block until signal

	slog.Info("Shutting down...")

	// Orderly shutdown sequence
	performGracefulShutdown(logger, raftNode, transport, grpcServer, sigClient, isLeader)

	slog.Info("Shutdown complete.")
}

func setupLogger() *slog.Logger {
	var levelVar slog.LevelVar
	if err := levelVar.UnmarshalText([]byte(logLevel)); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid log level '%s', defaulting to '%s'. Error: %v\n", logLevel, defaultLogLevel, err)
		if defaultErr := levelVar.UnmarshalText([]byte(defaultLogLevel)); defaultErr != nil {
			fmt.Fprintf(os.Stderr, "Error setting default log level: %v\n", defaultErr)
			os.Exit(1)
		}
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: levelVar.Level()}))
	slog.SetDefault(logger)
	return logger
}

// setupVerifier initializes the ExecutionVerifier based on configuration.
// It currently always returns a verifier (defaulting to NoOp) and logs warnings for issues.
func setupVerifier(cfg config.ExecutionConfig, logger *slog.Logger) verification.ExecutionVerifier {
	var execVerifier verification.ExecutionVerifier
	if cfg.Enabled {
		logger.Info("Execution verification enabled", "type", cfg.Type)
		switch cfg.Type {
		case "noop":
			execVerifier = verification.NewNoOpVerifier()
			logger.Info("Using NoOp execution verifier")
		case "fullnode":
			logger.Warn("FullNode execution verifier selected but not yet implemented. Disabling verification.")
			execVerifier = nil // Or return an error? Currently defaults to no-op later.
		default:
			logger.Warn("Unknown execution verifier type specified", "type", cfg.Type, "message", "Disabling execution verification.")
			execVerifier = nil // Or return an error? Currently defaults to no-op later.
		}
	} else {
		logger.Info("Execution verification disabled by configuration")
		execVerifier = verification.NewNoOpVerifier()
	}

	// Handle cases where verifier wasn't successfully initialized but config said enabled
	if cfg.Enabled && execVerifier == nil {
		logger.Warn("Execution verification was enabled but initialization failed, falling back to NoOpVerifier.")
		execVerifier = verification.NewNoOpVerifier()
	} else if execVerifier == nil {
		// This case handles when !cfg.Enabled and somehow execVerifier is still nil (shouldn't happen with current logic)
		execVerifier = verification.NewNoOpVerifier()
	}

	return execVerifier
}

// setupSigner initializes the Signer based on configuration.
func setupSigner(cfg config.SigningConfig, logger *slog.Logger) (signing.Signer, error) {
	logger.Info("Loading signing key", "path", cfg.PrivateKeyPath)
	signer, err := signing.LoadSigner(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to load signing key: %w", err)
	}
	// The log about the scheme is kept in runNode after successful setup.
	return signer, nil
}

// setupRaft initializes the RAFT node and transport.
func setupRaft(cfg *config.Config, fsm raft.FSM, logger *slog.Logger) (*raft.Raft, raft.Transport, error) {
	raftNode, transport, err := internalraft.NewRaftNode(cfg, fsm, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create RAFT node: %w", err)
	}
	return raftNode, transport, nil
}

// setupSignalingComponents initializes either the SignatureAggregator (leader) or the Client (follower).
func setupSignalingComponents(cfg *config.Config, logger *slog.Logger, isLeader bool, signer signing.Signer) (*aggregator.SignatureAggregator, *internalgrpc.Client, error) {
	var sigAggregator *aggregator.SignatureAggregator
	var sigClient *internalgrpc.Client
	var err error

	if isLeader {
		logger.Info("Initializing signature aggregator...")
		// Validate and setup Aggregator
		if cfg.Aggregator.QuorumThreshold <= 0 {
			return nil, nil, fmt.Errorf("aggregator.quorum_threshold must be positive for the leader")
		}
		if len(cfg.Aggregator.Attesters) == 0 {
			return nil, nil, fmt.Errorf("aggregator.attesters map cannot be empty for the leader")
		}
		attesterKeys := make(map[string]ed25519.PublicKey)
		logger.Info("Loading attester public keys...")
		for attesterID, keyPath := range cfg.Aggregator.Attesters {
			if keyPath == "" {
				return nil, nil, fmt.Errorf("public key path is empty for attester: %s", attesterID)
			}
			keyBytes, err := os.ReadFile(keyPath)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to read public key file for attester %s (path: %s): %w", attesterID, keyPath, err)
			}
			if len(keyBytes) != ed25519.PublicKeySize {
				return nil, nil, fmt.Errorf("invalid public key size for attester %s (path: %s), expected %d, got %d", attesterID, keyPath, ed25519.PublicKeySize, len(keyBytes))
			}
			attesterKeys[attesterID] = ed25519.PublicKey(keyBytes)
			logger.Debug("Loaded public key", "attester_id", attesterID, "path", keyPath)
		}
		logger.Info("Successfully loaded public keys", "count", len(attesterKeys))

		// Add leader's own public key to the map for the aggregator
		leaderPubKey := signer.PublicKey()
		if leaderPubKey != nil {
			if len(leaderPubKey) == ed25519.PublicKeySize {
				attesterKeys[cfg.Node.ID] = leaderPubKey
				logger.Info("Added leader's own public key to the aggregator's known keys", "leader_id", cfg.Node.ID)
			} else {
				return nil, nil, fmt.Errorf("leader's own public key has invalid size (leader ID: %s): %d", cfg.Node.ID, len(leaderPubKey))
			}
		} else {
			return nil, nil, fmt.Errorf("failed to get leader's own public key from signer (leader ID: %s)", cfg.Node.ID)
		}

		sigAggregator, err = aggregator.NewSignatureAggregator(logger, cfg.Aggregator.QuorumThreshold, attesterKeys)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to initialize signature aggregator: %w", err)
		}
	} else {
		logger.Info("Initializing signature client...")
		// Validate and setup gRPC Client
		if cfg.Network.SequencerSigEndpoint == "" {
			return nil, nil, fmt.Errorf("network.sequencer_sig_endpoint is required in config for followers")
		}
		clientCtx, clientCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer clientCancel() // Ensure context cancellation for this setup phase
		sigClient, err = internalgrpc.NewClient(clientCtx, cfg.Network.SequencerSigEndpoint, logger)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to initialize signature client: %w", err)
		}
		// The actual closing of the client connection should happen during shutdown,
		// so the defer in runNode handles that.
	}
	return sigAggregator, sigClient, nil
}

// setupFSM initializes the AttesterFSM.
func setupFSM(logger *slog.Logger, signer signing.Signer, nodeID string, isLeader bool, sigAggregator *aggregator.SignatureAggregator, sigClient *internalgrpc.Client, execVerifier verification.ExecutionVerifier) (*fsm.AttesterFSM, error) {
	attesterFSM, err := fsm.NewAttesterFSM(logger, signer, nodeID, isLeader, sigAggregator, sigClient, execVerifier)
	if err != nil {
		return nil, fmt.Errorf("failed to create FSM: %w", err)
	}
	return attesterFSM, nil
}

// performGracefulShutdown handles the orderly shutdown of node components.
func performGracefulShutdown(logger *slog.Logger, raftNode *raft.Raft, transport raft.Transport, grpcServer *internalgrpc.AttesterServer, sigClient *internalgrpc.Client, isLeader bool) {
	// Shutdown Raft first to stop processing new entries
	if raftNode != nil {
		logger.Info("Shutting down RAFT node...")
		if err := raftNode.Shutdown().Error(); err != nil {
			logger.Error("Error shutting down RAFT node", "error", err)
		}
		logger.Info("RAFT node shutdown complete.")
	}

	// Close Raft transport after Raft node is shut down
	if tcpTransport, ok := transport.(io.Closer); ok {
		logger.Debug("Closing RAFT transport...")
		if err := tcpTransport.Close(); err != nil {
			logger.Error("Error closing RAFT transport", "error", err)
		} else {
			logger.Debug("RAFT transport closed.")
		}
	} else if transport != nil {
		logger.Warn("RAFT transport does not implement io.Closer, cannot close automatically")
	}

	// Shutdown gRPC server (if leader) after Raft to stop accepting new requests
	if isLeader && grpcServer != nil {
		logger.Info("Stopping gRPC server...")
		grpcServer.Stop() // Assuming this blocks until stopped
		logger.Info("gRPC server stopped.")
	}

	// Close gRPC client connection (if follower)
	// This is handled by the defer statement in runNode for sigClient, so no action needed here.
	if !isLeader && sigClient != nil {
		logger.Debug("gRPC client connection closure is deferred in runNode.")
	}
}
