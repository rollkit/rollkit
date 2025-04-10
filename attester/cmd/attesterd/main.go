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

// Variables for flags
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
	var execVerifier verification.ExecutionVerifier
	if cfg.Execution.Enabled {
		slog.Info("Execution verification enabled", "type", cfg.Execution.Type)
		switch cfg.Execution.Type {
		case "noop":
			execVerifier = verification.NewNoOpVerifier()
			slog.Info("Using NoOp execution verifier")
		case "fullnode":
			slog.Warn("FullNode execution verifier selected but not yet implemented. Disabling verification.")
			execVerifier = nil
		default:
			slog.Warn("Unknown execution verifier type specified", "type", cfg.Execution.Type, "message", "Disabling execution verification.")
			execVerifier = nil
		}
	} else {
		slog.Info("Execution verification disabled by configuration")
		execVerifier = verification.NewNoOpVerifier()
	}

	// 3. Load private key
	slog.Info("Loading signing key", "path", cfg.Signing.PrivateKeyPath)
	signer, err := signing.LoadSigner(cfg.Signing)
	if err != nil {
		slog.Error("Failed to load signing key", "error", err)
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
	if isLeader {
		slog.Info("Initializing signature aggregator...")
		// Validate and setup Aggregator
		if cfg.Aggregator.QuorumThreshold <= 0 {
			slog.Error("aggregator.quorum_threshold must be positive for the leader")
			os.Exit(1)
		}
		if len(cfg.Aggregator.Attesters) == 0 {
			slog.Error("aggregator.attesters map cannot be empty for the leader")
			os.Exit(1)
		}
		attesterKeys := make(map[string]ed25519.PublicKey)
		slog.Info("Loading attester public keys...")
		for attesterID, keyPath := range cfg.Aggregator.Attesters {
			if keyPath == "" {
				slog.Error("Public key path is empty for attester", "id", attesterID)
				os.Exit(1)
			}
			keyBytes, err := os.ReadFile(keyPath)
			if err != nil {
				slog.Error("Failed to read public key file for attester", "id", attesterID, "path", keyPath, "error", err)
				os.Exit(1)
			}
			if len(keyBytes) != ed25519.PublicKeySize {
				slog.Error("Invalid public key size for attester", "id", attesterID, "path", keyPath, "expected_size", ed25519.PublicKeySize, "actual_size", len(keyBytes))
				os.Exit(1)
			}
			attesterKeys[attesterID] = ed25519.PublicKey(keyBytes)
			slog.Debug("Loaded public key", "attester_id", attesterID, "path", keyPath)
		}
		slog.Info("Successfully loaded public keys", "count", len(attesterKeys))

		// Add leader's own public key to the map for the aggregator
		leaderPubKey := signer.PublicKey()
		if leaderPubKey != nil {
			if len(leaderPubKey) == ed25519.PublicKeySize {
				attesterKeys[cfg.Node.ID] = leaderPubKey
				slog.Info("Added leader's own public key to the aggregator's known keys", "leader_id", cfg.Node.ID)
			} else {
				slog.Error("Leader's own public key has invalid size", "leader_id", cfg.Node.ID, "size", len(leaderPubKey))
				os.Exit(1)
			}
		} else {
			slog.Error("Failed to get leader's own public key from signer")
			os.Exit(1)
		}

		sigAggregator, err = aggregator.NewSignatureAggregator(logger, cfg.Aggregator.QuorumThreshold, attesterKeys)
		if err != nil {
			slog.Error("Failed to initialize signature aggregator", "error", err)
			os.Exit(1)
		}
	} else {
		slog.Info("Initializing signature client...")
		// Validate and setup gRPC Client
		if cfg.Network.SequencerSigEndpoint == "" {
			slog.Error("network.sequencer_sig_endpoint is required in config for followers")
			os.Exit(1)
		}
		clientCtx, clientCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer clientCancel()
		sigClient, err = internalgrpc.NewClient(clientCtx, cfg.Network.SequencerSigEndpoint, logger)
		if err != nil {
			slog.Error("Failed to initialize signature client", "error", err)
			os.Exit(1)
		}
		defer func() {
			if err := sigClient.Close(); err != nil {
				slog.Error("Error closing signature client", "error", err)
			}
		}()
	}

	// 5. Instantiate FSM
	slog.Info("Initializing FSM", "node_id", cfg.Node.ID, "is_leader", isLeader)
	attesterFSM, err = fsm.NewAttesterFSM(logger, signer, cfg.Node.ID, isLeader, sigAggregator, sigClient, execVerifier)
	if err != nil {
		slog.Error("Failed to initialize FSM", "error", err)
		os.Exit(1)
	}

	// 6. Instantiate RAFT node (Requires FSM)
	slog.Info("Initializing RAFT node...")
	raftNode, transport, err = internalraft.NewRaftNode(cfg, attesterFSM, logger)
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

	// Shutdown Raft first to stop processing new entries
	if raftNode != nil {
		slog.Info("Shutting down RAFT node...")
		if err := raftNode.Shutdown().Error(); err != nil {
			slog.Error("Error shutting down RAFT node", "error", err)
		}
		slog.Info("RAFT node shutdown complete.")
	}

	// Close Raft transport after Raft node is shut down
	if tcpTransport, ok := transport.(io.Closer); ok {
		slog.Debug("Closing RAFT transport...")
		if err := tcpTransport.Close(); err != nil {
			slog.Error("Error closing RAFT transport", "error", err)
		} else {
			slog.Debug("RAFT transport closed.")
		}
	} else if transport != nil {
		slog.Warn("RAFT transport does not implement io.Closer, cannot close automatically")
	}

	// Shutdown gRPC server (if leader) after Raft to stop accepting new requests
	if isLeader && grpcServer != nil {
		slog.Info("Stopping gRPC server...")
		grpcServer.Stop() // Assuming this blocks until stopped
		slog.Info("gRPC server stopped.")
	}

	// Close gRPC client connection (if follower) - already deferred

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
