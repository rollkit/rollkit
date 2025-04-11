package node

import (
	"context"
	"io"

	internalgrpc "github.com/rollkit/rollkit/attester/internal/grpc"
)

type FollowerNode struct {
	commonNode
	sigClient *internalgrpc.Client
}

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
