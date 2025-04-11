package node

import (
	"context"

	internalgrpc "github.com/rollkit/rollkit/attester/internal/grpc"
)

type FollowerNode struct {
	commonNode
	sigClient *internalgrpc.Client
}

func (fn *FollowerNode) Start(ctx context.Context) error {
	fn.logger.Info("FollowerNode started.")

	return nil
}

// Stop gracefully shuts down the FollowerNode components.
func (fn *FollowerNode) Stop() {
	fn.logger.Info("Stopping FollowerNode...")

	if fn.sigClient != nil {
		fn.logger.Info("Closing signature client connection...")
		if err := fn.sigClient.Close(); err != nil {
			fn.logger.Error("Error closing signature client", "error", err)
		} else {
			fn.logger.Info("Signature client connection closed.")
		}
	}

	fn.commonNode.Stop()

	fn.logger.Info("FollowerNode stopped.")
}
