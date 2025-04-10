package verification

import (
	"context"

	"github.com/rollkit/rollkit/attester/internal/state"
)

// ExecutionVerifier defines the interface for verifying block execution.
type ExecutionVerifier interface {
	// VerifyExecution checks the execution validity of a block.
	// It receives context, block identifiers (height/hash), and
	// necessary block data for verification.
	// Returns nil if verification is successful or not applicable,
	// or an error if verification fails.
	VerifyExecution(ctx context.Context, height uint64, hash state.BlockHash, blockData []byte) error
}

var _ ExecutionVerifier = (*NoOpVerifier)(nil)

// NoOpVerifier is an implementation that performs no verification.
type NoOpVerifier struct{}

// VerifyExecution always returns nil for NoOpVerifier.
func (v *NoOpVerifier) VerifyExecution(ctx context.Context, height uint64, hash state.BlockHash, blockData []byte) error {
	return nil
}

// NewNoOpVerifier creates a new NoOpVerifier.
func NewNoOpVerifier() *NoOpVerifier {
	return &NoOpVerifier{}
}
