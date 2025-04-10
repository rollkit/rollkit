package grpc

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/hashicorp/raft"
	"google.golang.org/grpc"

	// Use codes/status for richer gRPC errors eventually?
	// "google.golang.org/grpc/codes"
	// "google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	attesterv1 "github.com/rollkit/rollkit/attester/api/gen/attester/v1"
	"github.com/rollkit/rollkit/attester/internal/fsm"
	// Import aggregator
)

// RaftNode defines the interface required from a raft.Raft node
// by the AttesterServer. This allows for easier mocking in tests.
type RaftNode interface {
	State() raft.RaftState
	Leader() raft.ServerAddress
	Apply(cmd []byte, timeout time.Duration) raft.ApplyFuture
	GetConfiguration() raft.ConfigurationFuture
}

// Aggregator defines the interface required from the signature aggregator.
type Aggregator interface {
	AddSignature(blockHeight uint64, blockHash []byte, attesterID string, signature []byte) (bool, error)
	GetAggregatedSignatures(blockHeight uint64) ([][]byte, bool)
}

var _ attesterv1.AttesterServiceServer = (*AttesterServer)(nil)

// AttesterServer implements the gRPC service defined in the protobuf.
// It handles incoming requests, interacts with the Raft node and potentially an aggregator.
type AttesterServer struct {
	attesterv1.UnimplementedAttesterServiceServer

	raftNode   RaftNode
	logger     *slog.Logger
	grpcSrv    *grpc.Server
	aggregator Aggregator
}

// NewAttesterServer creates a new instance of the AttesterServer.
func NewAttesterServer(raftNode RaftNode, logger *slog.Logger, aggregator Aggregator) *AttesterServer {
	return &AttesterServer{
		raftNode:   raftNode,
		logger:     logger.With("component", "grpc-server"),
		aggregator: aggregator,
	}
}

// Start initializes and starts the gRPC server on the given address.
func (s *AttesterServer) Start(listenAddress string) error {
	if listenAddress == "" {
		return fmt.Errorf("gRPC listen address cannot be empty")
	}

	lis, err := net.Listen("tcp", listenAddress)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", listenAddress, err)
	}

	// TODO: Add gRPC server options (TLS, interceptors, etc.) if needed
	grpcOptions := []grpc.ServerOption{}
	s.grpcSrv = grpc.NewServer(grpcOptions...)

	// Register the Attester service
	attesterv1.RegisterAttesterServiceServer(s.grpcSrv, s)

	s.logger.Info("gRPC server starting", "address", listenAddress)
	// Serve blocks until Stop() is called or an error occurs.
	if err := s.grpcSrv.Serve(lis); err != nil {
		// ErrServerStopped is expected on graceful shutdown, log others as errors.
		if err == grpc.ErrServerStopped {
			s.logger.Info("gRPC server stopped gracefully")
			return nil
		} else {
			s.logger.Error("gRPC server failed", "error", err)
			return fmt.Errorf("gRPC server encountered an error: %w", err)
		}
	}
	return nil
}

// Stop gracefully shuts down the gRPC server.
func (s *AttesterServer) Stop() {
	if s.grpcSrv != nil {
		s.logger.Info("Stopping gRPC server...")
		// GracefulStop waits for existing connections to finish before shutting down.
		s.grpcSrv.GracefulStop()
	}
}

// SubmitBlock is the RPC handler for external clients (e.g., DA layer) submitting blocks to the leader.
// Only the leader node should successfully process this request.
func (s *AttesterServer) SubmitBlock(ctx context.Context, req *attesterv1.SubmitBlockRequest) (*attesterv1.SubmitBlockResponse, error) {
	// 1. Check if this node is the leader
	if s.raftNode.State() != raft.Leader {
		leaderAddr := string(s.raftNode.Leader()) // Convert ServerAddress to string
		s.logger.Warn("SubmitBlock request received by non-leader node", "state", s.raftNode.State(), "leader_hint", leaderAddr)
		// Return response indicating not accepted and provide leader hint
		return &attesterv1.SubmitBlockResponse{
			Accepted:     false,
			ErrorMessage: "Node is not the RAFT leader",
			LeaderHint:   leaderAddr,
		}, nil // Return nil error, handled within the response message
	}

	s.logger.Info("Received SubmitBlock request",
		"block_height", req.BlockHeight,
		"block_hash", fmt.Sprintf("%x", req.BlockHash),
	)

	// 2. Validate block data (basic validation)
	if req.BlockHeight == 0 {
		return &attesterv1.SubmitBlockResponse{Accepted: false, ErrorMessage: "Block height cannot be zero"}, nil
	}
	if len(req.BlockHash) == 0 { // TODO: Use state.BlockHashSize constant from state package?
		return &attesterv1.SubmitBlockResponse{Accepted: false, ErrorMessage: "Block hash cannot be empty"}, nil
	}
	if len(req.DataToSign) == 0 {
		return &attesterv1.SubmitBlockResponse{Accepted: false, ErrorMessage: "Data to sign cannot be empty"}, nil
	}

	// 3. Serialize the SubmitBlockRequest as expected by the FSM
	reqData, err := proto.Marshal(req) // Marshal the incoming request
	if err != nil {
		s.logger.Error("Failed to marshal SubmitBlockRequest for RAFT log", "error", err)
		return &attesterv1.SubmitBlockResponse{Accepted: false, ErrorMessage: "Internal error marshalling log data"}, nil
	}

	// Prepend the log entry type byte
	logData := append([]byte{fsm.LogEntryTypeSubmitBlock}, reqData...)

	// Log current Raft configuration before Apply
	currentConfigFuture := s.raftNode.GetConfiguration()
	if err := currentConfigFuture.Error(); err != nil {
		s.logger.Error("Failed to get current raft configuration before Apply", "error", err)
		// Decide if this should be a fatal error for the request
	} else {
		currentConfig := currentConfigFuture.Configuration()
		serverInfo := make([]string, len(currentConfig.Servers))
		for i, server := range currentConfig.Servers {
			serverInfo[i] = fmt.Sprintf("{ID:%s Addr:%s Suffrage:%s}", server.ID, server.Address, server.Suffrage)
		}
		s.logger.Debug("Current Raft cluster configuration before Apply", "servers", serverInfo)
	}

	// 4. Propose the log data to RAFT via Apply
	// Set a reasonable timeout for Apply
	applyTimeout := 20 * time.Second // Reverted from 60s
	applyFuture := s.raftNode.Apply(logData, applyTimeout)

	// 5. Wait for the Apply future to complete
	if err := applyFuture.Error(); err != nil {
		// Log the specific error from Apply
		s.logger.Error("RAFT Apply failed", "block_height", req.BlockHeight, "error", err)
		return &attesterv1.SubmitBlockResponse{Accepted: false, ErrorMessage: fmt.Sprintf("Failed to commit block via RAFT: %v", err)}, nil
	}

	// Apply was successful
	s.logger.Info("Block successfully proposed and applied via RAFT", "block_height", req.BlockHeight)

	// The response from ApplyFuture might contain data returned by the FSM's Apply method.
	// We might not need it here, but it's available via applyFuture.Response().
	// fsmResponse := applyFuture.Response()

	// Indicate successful acceptance and proposal initiation.
	// Final attestation depends on followers signing and submitting.
	return &attesterv1.SubmitBlockResponse{
		Accepted: true,
	}, nil
}

// SubmitSignature is the RPC handler for followers submitting their signatures.
func (s *AttesterServer) SubmitSignature(ctx context.Context, req *attesterv1.SubmitSignatureRequest) (*attesterv1.SubmitSignatureResponse, error) {
	s.logger.Info("Received SubmitSignature request",
		"block_height", req.BlockHeight,
		"block_hash", fmt.Sprintf("%x", req.BlockHash),
		"attester_id", req.AttesterId,
		// Avoid logging full signature by default for brevity/security?
		// "signature", fmt.Sprintf("%x", req.Signature),
	)

	// --- Validation ---
	if req.BlockHeight == 0 {
		return nil, fmt.Errorf("block height cannot be zero")
	}
	if len(req.BlockHash) == 0 { // TODO: Use state.BlockHashSize constant?
		return nil, fmt.Errorf("block hash cannot be empty")
	}
	if req.AttesterId == "" {
		return nil, fmt.Errorf("attester ID cannot be empty")
	}
	if len(req.Signature) == 0 {
		return nil, fmt.Errorf("signature cannot be empty")
	}

	// Pass the signature to the aggregator (now via interface)
	// Ensure aggregator is not nil before calling methods on it
	if s.aggregator == nil {
		s.logger.Error("Aggregator is not configured in the server")
		// Return an internal error, as this indicates a setup problem
		return nil, fmt.Errorf("internal server error: aggregator not configured") // Consider grpc codes.Internal
	}
	quorumReached, err := s.aggregator.AddSignature(req.BlockHeight, req.BlockHash, req.AttesterId, req.Signature)
	if err != nil {
		s.logger.Error("Failed to add signature via aggregator", "error", err, "attester_id", req.AttesterId, "block_height", req.BlockHeight)
		// Return specific gRPC error codes? e.g., codes.InvalidArgument, codes.Internal
		return nil, fmt.Errorf("failed to process signature: %w", err)
	}

	s.logger.Debug("Signature processed by aggregator", "block_height", req.BlockHeight, "attester_id", req.AttesterId, "quorum_reached", quorumReached)

	// Return a simple acknowledgement.
	// The actual success depends on verification/aggregation state.
	return &attesterv1.SubmitSignatureResponse{
		Success: true, // Indicates successful reception and basic processing
	}, nil
}
