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

type NodeLifecycle interface {
	Start(ctx context.Context) error
	Stop()
}

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

// Stop gracefully shuts down the common components (Raft node and transport).
func (cn *commonNode) Stop() {
	// Shutdown Raft
	if cn.raftNode != nil {
		cn.logger.Info("Shutting down RAFT node...")
		if err := cn.raftNode.Shutdown().Error(); err != nil {
			cn.logger.Error("Error shutting down RAFT node", "error", err)
		}
		cn.logger.Info("RAFT node shutdown complete.")
	}

	// Close Raft transport
	if tcpTransport, ok := cn.transport.(io.Closer); ok {
		cn.logger.Debug("Closing RAFT transport...")
		if err := tcpTransport.Close(); err != nil {
			cn.logger.Error("Error closing RAFT transport", "error", err)
		} else {
			cn.logger.Debug("RAFT transport closed.")
		}
	} else if cn.transport != nil {
		cn.logger.Warn("RAFT transport does not implement io.Closer, cannot close automatically")
	}
}

// NewNode creates either a LeaderNode or a FollowerNode based on the provided configuration and leader status.
func NewNode(cfg *config.Config, logger *slog.Logger, isLeader bool) (NodeLifecycle, error) {
	execVerifier := setupVerifier(cfg.Execution, logger)

	signer, err := setupSigner(cfg.Signing, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to set up signer: %w", err)
	}
	logger.Info("Loaded signing key", "scheme", cfg.Signing.Scheme)

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

	logger.Info("Initializing FSM", "node_id", cfg.Node.ID, "is_leader", isLeader)
	attesterFSM, err := setupFSM(logger, signer, cfg.Node.ID, isLeader, sigAggregator, sigClient, execVerifier)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize FSM: %w", err)
	}

	logger.Info("Initializing RAFT node...")
	raftNode, transport, err := setupRaft(cfg, attesterFSM, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize RAFT node: %w", err)
	}

	cNode := commonNode{
		logger:    logger,
		cfg:       cfg,
		signer:    signer,
		verifier:  execVerifier,
		fsm:       attesterFSM,
		raftNode:  raftNode,
		transport: transport,
	}

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
