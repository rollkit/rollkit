package node

import (
	"context"
	"io"

	"github.com/rollkit/rollkit/attester/internal/aggregator"
	internalgrpc "github.com/rollkit/rollkit/attester/internal/grpc"
)

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
