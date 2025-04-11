package node

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/hashicorp/raft"

	"github.com/rollkit/rollkit/attester/internal/aggregator"
	"github.com/rollkit/rollkit/attester/internal/config"
	"github.com/rollkit/rollkit/attester/internal/fsm"
	internalgrpc "github.com/rollkit/rollkit/attester/internal/grpc"
	internalraft "github.com/rollkit/rollkit/attester/internal/raft"
	"github.com/rollkit/rollkit/attester/internal/signing"
	"github.com/rollkit/rollkit/attester/internal/verification"
)

// --- Node Lifecycle Interface ---

// NodeLifecycle defines the common interface for starting and stopping attester nodes.
type NodeLifecycle interface {
	Start(ctx context.Context) error
	Stop()
}

// --- Common Node Components ---

// commonNode holds components shared between Leader and Follower nodes.
type commonNode struct {
	logger    *slog.Logger
	cfg       *config.Config
	signer    signing.Signer
	verifier  verification.ExecutionVerifier
	fsm       *fsm.AttesterFSM
	raftNode  *raft.Raft
	transport raft.Transport
}

// --- Leader Node ---

// LeaderNode represents an attester node operating as the RAFT leader.
// It manages the signature aggregator and the gRPC server.
type LeaderNode struct {
	commonNode
	sigAggregator *aggregator.SignatureAggregator
	grpcServer    *internalgrpc.AttesterServer
}

// Start begins the LeaderNode's operation, primarily by starting the gRPC server.
func (ln *LeaderNode) Start(ctx context.Context) error {
	ln.logger.Info("Starting gRPC server in background...", "address", ln.cfg.GRPC.ListenAddress)
	go func() {
		listenAddr := ln.cfg.GRPC.ListenAddress
		if listenAddr == "" {
			// This check should ideally be caught during NewNode validation.
			ln.logger.Error("grpc.listen_address is required in config for the leader (error in Start goroutine)")
			// Consider implementing better error propagation if needed.
			return
		}
		ln.logger.Info("gRPC server goroutine starting", "address", listenAddr)
		if err := ln.grpcServer.Start(listenAddr); err != nil {
			// Handle error appropriately, e.g., log, signal main thread, etc.
			ln.logger.Error("gRPC server failed to start or encountered error", "error", err)
		}
		ln.logger.Info("gRPC server goroutine finished")
	}()
	return nil // Assume synchronous start errors handled in NewNode
}

// Stop gracefully shuts down the LeaderNode components.
func (ln *LeaderNode) Stop() {
	ln.logger.Info("Stopping LeaderNode...")

	// Shutdown gRPC server first to stop accepting new requests
	if ln.grpcServer != nil {
		ln.logger.Info("Stopping gRPC server...")
		ln.grpcServer.Stop() // Assuming this blocks until stopped
		ln.logger.Info("gRPC server stopped.")
	}

	// Shutdown Raft
	if ln.raftNode != nil {
		ln.logger.Info("Shutting down RAFT node...")
		if err := ln.raftNode.Shutdown().Error(); err != nil {
			ln.logger.Error("Error shutting down RAFT node", "error", err)
		}
		ln.logger.Info("RAFT node shutdown complete.")
	}

	// Close Raft transport
	if tcpTransport, ok := ln.transport.(io.Closer); ok {
		ln.logger.Debug("Closing RAFT transport...")
		if err := tcpTransport.Close(); err != nil {
			ln.logger.Error("Error closing RAFT transport", "error", err)
		} else {
			ln.logger.Debug("RAFT transport closed.")
		}
	} else if ln.transport != nil {
		ln.logger.Warn("RAFT transport does not implement io.Closer, cannot close automatically")
	}

	ln.logger.Info("LeaderNode stopped.")
}

// --- Follower Node ---

// FollowerNode represents an attester node operating as a RAFT follower.
// It manages the gRPC client connection to the leader.
type FollowerNode struct {
	commonNode
	sigClient *internalgrpc.Client
}

// Start begins the FollowerNode's operation.
func (fn *FollowerNode) Start(ctx context.Context) error {
	fn.logger.Info("FollowerNode started.")
	// No background processes needed for follower currently
	return nil
}

// Stop gracefully shuts down the FollowerNode components.
func (fn *FollowerNode) Stop() {
	fn.logger.Info("Stopping FollowerNode...")

	// Close gRPC client connection first
	if fn.sigClient != nil {
		fn.logger.Info("Closing signature client connection...")
		if err := fn.sigClient.Close(); err != nil {
			fn.logger.Error("Error closing signature client", "error", err)
		} else {
			fn.logger.Info("Signature client connection closed.")
		}
	}

	// Shutdown Raft
	if fn.raftNode != nil {
		fn.logger.Info("Shutting down RAFT node...")
		if err := fn.raftNode.Shutdown().Error(); err != nil {
			fn.logger.Error("Error shutting down RAFT node", "error", err)
		}
		fn.logger.Info("RAFT node shutdown complete.")
	}

	// Close Raft transport
	if tcpTransport, ok := fn.transport.(io.Closer); ok {
		fn.logger.Debug("Closing RAFT transport...")
		if err := tcpTransport.Close(); err != nil {
			fn.logger.Error("Error closing RAFT transport", "error", err)
		} else {
			fn.logger.Debug("RAFT transport closed.")
		}
	} else if fn.transport != nil {
		fn.logger.Warn("RAFT transport does not implement io.Closer, cannot close automatically")
	}

	fn.logger.Info("FollowerNode stopped.")
}

// --- Node Factory ---

// NewNode creates either a LeaderNode or a FollowerNode based on the provided configuration and leader status.
func NewNode(cfg *config.Config, logger *slog.Logger, isLeader bool) (NodeLifecycle, error) {
	// 1. Setup Verifier
	execVerifier := setupVerifier(cfg.Execution, logger)

	// 2. Setup Signer
	signer, err := setupSigner(cfg.Signing, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to set up signer: %w", err)
	}
	logger.Info("Loaded signing key", "scheme", cfg.Signing.Scheme)

	// 3. Setup Role-Specific Components (Aggregator or Client)
	var sigAggregator *aggregator.SignatureAggregator
	var sigClient *internalgrpc.Client

	if isLeader {
		logger.Info("Initializing leader-specific components (aggregator)...")
		if cfg.Aggregator.QuorumThreshold <= 0 {
			return nil, fmt.Errorf("aggregator.quorum_threshold must be positive for the leader")
		}
		if len(cfg.Aggregator.Attesters) == 0 {
			return nil, fmt.Errorf("aggregator.attesters map cannot be empty for the leader")
		}
		attesterKeys, err := loadAttesterKeys(cfg, signer, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to load attester keys: %w", err)
		}
		sigAggregator, err = aggregator.NewSignatureAggregator(logger, cfg.Aggregator.QuorumThreshold, attesterKeys)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize signature aggregator: %w", err)
		}
		logger.Info("Signature aggregator initialized")
	} else {
		logger.Info("Initializing follower-specific components (client)...")
		if cfg.Network.SequencerSigEndpoint == "" {
			return nil, fmt.Errorf("network.sequencer_sig_endpoint is required in config for followers")
		}
		clientCtx, clientCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer clientCancel()
		sigClient, err = internalgrpc.NewClient(clientCtx, cfg.Network.SequencerSigEndpoint, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize signature client: %w", err)
		}
		logger.Info("Signature client initialized", "endpoint", cfg.Network.SequencerSigEndpoint)
	}

	// 4. Setup FSM
	logger.Info("Initializing FSM", "node_id", cfg.Node.ID, "is_leader", isLeader)
	attesterFSM, err := setupFSM(logger, signer, cfg.Node.ID, isLeader, sigAggregator, sigClient, execVerifier)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize FSM: %w", err)
	}

	// 5. Setup RAFT
	logger.Info("Initializing RAFT node...")
	raftNode, transport, err := setupRaft(cfg, attesterFSM, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize RAFT node: %w", err)
	}

	// Create common node components
	cNode := commonNode{
		logger:    logger,
		cfg:       cfg,
		signer:    signer,
		verifier:  execVerifier,
		fsm:       attesterFSM,
		raftNode:  raftNode,
		transport: transport,
	}

	// 6. Construct and Return Leader or Follower Node
	if isLeader {
		logger.Info("Initializing gRPC server components for leader...")
		if cfg.GRPC.ListenAddress == "" {
			return nil, fmt.Errorf("grpc.listen_address is required in config for the leader")
		}
		grpcServer := internalgrpc.NewAttesterServer(raftNode, logger, sigAggregator)

		leader := &LeaderNode{
			commonNode:    cNode,
			sigAggregator: sigAggregator,
			grpcServer:    grpcServer,
		}
		logger.Info("Leader node constructed.")
		return leader, nil
	} else {
		follower := &FollowerNode{
			commonNode: cNode,
			sigClient:  sigClient,
		}
		logger.Info("Follower node constructed.")
		return follower, nil
	}
}

// --- Helper Functions (Unexported) ---

// loadAttesterKeys loads public keys for the leader's signature aggregator.
func loadAttesterKeys(cfg *config.Config, signer signing.Signer, logger *slog.Logger) (map[string]ed25519.PublicKey, error) {
	attesterKeys := make(map[string]ed25519.PublicKey)
	logger.Info("Loading attester public keys for aggregator...")
	for attesterID, keyPath := range cfg.Aggregator.Attesters {
		if keyPath == "" {
			return nil, fmt.Errorf("public key path is empty for attester: %s", attesterID)
		}
		keyBytes, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read public key file for attester %s (path: %s): %w", attesterID, keyPath, err)
		}
		if len(keyBytes) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("invalid public key size for attester %s (path: %s), expected %d, got %d", attesterID, keyPath, ed25519.PublicKeySize, len(keyBytes))
		}
		attesterKeys[attesterID] = ed25519.PublicKey(keyBytes)
		logger.Debug("Loaded public key", "attester_id", attesterID, "path", keyPath)
	}
	logger.Info("Successfully loaded configured attester public keys", "count", len(attesterKeys))

	// Add leader's own public key
	leaderPubKey := signer.PublicKey()
	if leaderPubKey == nil {
		return nil, fmt.Errorf("failed to get leader's own public key from signer (leader ID: %s)", cfg.Node.ID)
	}
	if len(leaderPubKey) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("leader's own public key has invalid size (leader ID: %s): %d", cfg.Node.ID, len(leaderPubKey))
	}
	attesterKeys[cfg.Node.ID] = leaderPubKey
	logger.Info("Added leader's own public key to the aggregator's known keys", "leader_id", cfg.Node.ID)

	return attesterKeys, nil
}

// setupVerifier initializes the ExecutionVerifier based on configuration.
func setupVerifier(cfg config.ExecutionConfig, logger *slog.Logger) verification.ExecutionVerifier {
	var execVerifier verification.ExecutionVerifier
	if cfg.Enabled {
		logger.Info("Execution verification enabled", "type", cfg.Type)
		switch cfg.Type {
		case "noop":
			execVerifier = verification.NewNoOpVerifier()
			logger.Info("Using NoOp execution verifier")
		case "fullnode":
			logger.Warn("FullNode execution verifier selected but not yet implemented. Defaulting to NoOp.")
			execVerifier = verification.NewNoOpVerifier() // Default to NoOp
		default:
			logger.Warn("Unknown execution verifier type specified, defaulting to NoOp.", "type", cfg.Type)
			execVerifier = verification.NewNoOpVerifier() // Default to NoOp
		}
	} else {
		logger.Info("Execution verification disabled by configuration")
		execVerifier = verification.NewNoOpVerifier()
	}
	if execVerifier == nil {
		logger.Warn("Execution verifier initialization resulted in nil, falling back to NoOpVerifier.")
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
	return signer, nil
}

// setupFSM initializes the AttesterFSM.
func setupFSM(logger *slog.Logger, signer signing.Signer, nodeID string, isLeader bool, sigAggregator *aggregator.SignatureAggregator, sigClient *internalgrpc.Client, execVerifier verification.ExecutionVerifier) (*fsm.AttesterFSM, error) {
	attesterFSM, err := fsm.NewAttesterFSM(logger, signer, nodeID, isLeader, sigAggregator, sigClient, execVerifier)
	if err != nil {
		return nil, fmt.Errorf("failed to create FSM: %w", err)
	}
	return attesterFSM, nil
}

// setupRaft initializes the RAFT node and transport.
func setupRaft(cfg *config.Config, fsm raft.FSM, logger *slog.Logger) (*raft.Raft, raft.Transport, error) {
	raftNode, transport, err := internalraft.NewRaftNode(cfg, fsm, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create RAFT node: %w", err)
	}
	return raftNode, transport, nil
}
