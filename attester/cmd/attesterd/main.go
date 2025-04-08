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

func runNode(cmd *cobra.Command, args []string) {
	var levelVar slog.LevelVar
	if err := levelVar.UnmarshalText([]byte(logLevel)); err != nil {
		// Use standard log before logger is fully setup for this specific error
		fmt.Fprintf(os.Stderr, "Invalid log level '%s', defaulting to '%s'. Error: %v\n", logLevel, defaultLogLevel, err)
		// Default to info level if parsing fails
		if defaultErr := levelVar.UnmarshalText([]byte(defaultLogLevel)); defaultErr != nil {
			// This should ideally not happen if defaultLogLevel is valid
			fmt.Fprintf(os.Stderr, "Error setting default log level: %v\n", defaultErr)
			os.Exit(1)
		}
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: levelVar.Level()}))
	slog.SetDefault(logger) // Make it the default logger

	if isLeader {
		slog.Info("Initializing attester node as LEADER...")
	} else {
		slog.Info("Initializing attester node as FOLLOWER...")
	}

	// 1. Load configuration
	slog.Info("Loading configuration", "path", configFile)
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1) // Use os.Exit after logging error
	}
	// TODO: Validate loaded configuration further if needed

	// 3. Load private key
	slog.Info("Loading signing key", "path", cfg.Signing.PrivateKeyPath)
	signer, err := signing.LoadSigner(cfg.Signing)
	if err != nil {
		slog.Error("Failed to load signing key", "error", err)
		os.Exit(1)
	}
	slog.Info("Loaded signing key", "scheme", cfg.Signing.Scheme)

	// Declare components needed across roles/scopes
	var sigAggregator *aggregator.SignatureAggregator
	var attesterFSM *fsm.AttesterFSM
	var raftNode *raft.Raft
	var transport raft.Transport
	var grpcServer *internalgrpc.AttesterServer
	var sigClient *internalgrpc.Client

	if isLeader {
		// 6. Instantiate Aggregator (Leader only)
		slog.Info("Initializing signature aggregator...")

		// Validate aggregator config for leader
		if cfg.Aggregator.QuorumThreshold <= 0 {
			slog.Error("aggregator.quorum_threshold must be positive for the leader")
			os.Exit(1)
		}
		if len(cfg.Aggregator.Attesters) == 0 {
			slog.Error("aggregator.attesters map cannot be empty for the leader")
			os.Exit(1)
		}

		// Load attester public keys from files specified in config
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

		// Create the aggregator with loaded keys and quorum threshold from config
		sigAggregator, err = aggregator.NewSignatureAggregator(logger, cfg.Aggregator.QuorumThreshold, attesterKeys)
		if err != nil {
			slog.Error("Failed to initialize signature aggregator", "error", err)
			os.Exit(1)
		}

		// Instantiate gRPC server (NOW that raftNode exists)
		slog.Info("Initializing gRPC server...")
		grpcServer = internalgrpc.NewAttesterServer(raftNode, logger, sigAggregator)

		// 8. Start gRPC server in a goroutine (Leader only)
		slog.Info("Starting gRPC server...")
		go func() {
			listenAddr := cfg.GRPC.ListenAddress
			if listenAddr == "" {
				slog.Error("grpc.listen_address is required in config for the leader")
				return
			}
			slog.Info("gRPC server listening", "address", listenAddr)
			if err := grpcServer.Start(listenAddr); err != nil {
				slog.Error("Failed to start gRPC server", "error", err)
			}
		}()
	} else {
		// Instantiate Signature Client (Follower only)
		slog.Info("Initializing signature client...")
		if cfg.Network.SequencerSigEndpoint == "" {
			slog.Error("network.sequencer_sig_endpoint is required in config for followers")
			os.Exit(1)
		}
		// Use background context with timeout for initial connection
		clientCtx, clientCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer clientCancel() // Ensure context is cancelled eventually

		sigClient, err = internalgrpc.NewClient(clientCtx, cfg.Network.SequencerSigEndpoint, logger) // Corrected constructor name
		if err != nil {
			slog.Error("Failed to initialize signature client", "error", err)
			os.Exit(1)
		}
		// Defer closing the client connection
		defer func() {
			if err := sigClient.Close(); err != nil {
				slog.Error("Error closing signature client", "error", err)
			}
		}()
	}

	// 4. Instantiate FSM (Pass node ID, aggregator, client)
	slog.Info("Initializing FSM", "data_dir", cfg.Raft.DataDir, "node_id", cfg.Node.ID)
	attesterFSM = fsm.NewAttesterFSM(logger, signer, cfg.Node.ID, cfg.Raft.DataDir, sigAggregator, sigClient)

	// 5. Instantiate RAFT node (Pass the FSM)
	slog.Info("Initializing RAFT node...")
	raftNode, transport, err = internalraft.NewRaftNode(cfg, attesterFSM, logger)
	if err != nil {
		slog.Error("Failed to initialize RAFT node", "error", err)
		os.Exit(1)
	}
	defer func() {
		if tcpTransport, ok := transport.(io.Closer); ok {
			slog.Debug("Closing RAFT transport...")
			if err := tcpTransport.Close(); err != nil {
				slog.Error("Error closing RAFT transport", "error", err)
			}
		} else {
			slog.Warn("RAFT transport does not implement io.Closer, cannot close automatically")
		}
	}()

	if isLeader {
		// 8. Start gRPC server in a goroutine (Leader only)
		slog.Info("Starting gRPC server...")
		go func() {
			listenAddr := cfg.GRPC.ListenAddress
			if listenAddr == "" {
				slog.Error("grpc.listen_address is required in config for the leader")
				return
			}
			slog.Info("gRPC server listening", "address", listenAddr)
			if err := grpcServer.Start(listenAddr); err != nil {
				slog.Error("Failed to start gRPC server", "error", err)
			}
		}()
	}

	// 9. Handle graceful shutdown (Renumbered)
	slog.Info("Attester node running. Press Ctrl+C to exit.")
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan // Block until a signal is received

	slog.Info("Shutting down...")

	// Shutdown logic for gRPC (leader) and Raft
	if isLeader && grpcServer != nil {
		slog.Info("Stopping gRPC server...")
		grpcServer.Stop() // Call Stop on our server instance
	}
	if raftNode != nil {
		slog.Info("Shutting down RAFT node...")
		if err := raftNode.Shutdown().Error(); err != nil {
			slog.Error("Error shutting down RAFT node", "error", err)
		}
	}

	slog.Info("Shutdown complete.")
}
