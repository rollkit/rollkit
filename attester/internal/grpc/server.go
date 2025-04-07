package grpc

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	"github.com/hashicorp/raft"
	"google.golang.org/grpc"

	attesterv1 "github.com/rollkit/rollkit/attester/api/gen/attester/v1"
	"github.com/rollkit/rollkit/attester/internal/aggregator" // Import aggregator
)

// AttesterServer implements the gRPC service defined in the protobuf.
// It handles incoming requests, interacts with the Raft node and potentially an aggregator.
type AttesterServer struct {
	attesterv1.UnimplementedAttesterServiceServer // Embed the unimplemented server

	raftNode   *raft.Raft
	logger     *slog.Logger
	grpcSrv    *grpc.Server
	aggregator *aggregator.SignatureAggregator // Add signature aggregator
}

// NewAttesterServer creates a new instance of the AttesterServer.
func NewAttesterServer(raftNode *raft.Raft, logger *slog.Logger, aggregator *aggregator.SignatureAggregator) *AttesterServer {
	return &AttesterServer{
		raftNode:   raftNode,
		logger:     logger.With("component", "grpc-server"),
		aggregator: aggregator, // Store the aggregator
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

	// Pass the signature to the aggregator
	// Note: The aggregator needs to implement signature verification internally.
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
