package aggregator

import (
	"crypto/ed25519"
	"fmt"
	"log/slog"
	"sync"
	// Internal packages that might be needed:
	// "github.com/rollkit/rollkit/attester/internal/config"
	// "github.com/rollkit/rollkit/attester/internal/state"
	// "github.com/rollkit/rollkit/attester/internal/signing" // Might not be needed directly now
)

// SignatureAggregator collects signatures from attestors for specific blocks.
type SignatureAggregator struct {
	mu sync.RWMutex

	// State
	signatures map[uint64]map[string][]byte // blockHeight -> attesterID -> signature
	blockData  map[string][]byte            // blockHash (as string) -> dataToSign
	// TODO: Add state for quorum status per block?

	// Configuration
	quorumThreshold int
	attesterKeys    map[string]ed25519.PublicKey // Map Attester ID -> Public Key

	logger *slog.Logger
}

// NewSignatureAggregator creates a new aggregator.
func NewSignatureAggregator(logger *slog.Logger, quorumThreshold int, attesterKeys map[string]ed25519.PublicKey) (*SignatureAggregator, error) {
	if quorumThreshold <= 0 {
		return nil, fmt.Errorf("quorum threshold must be positive")
	}
	// Require non-empty keys for now
	if len(attesterKeys) == 0 {
		return nil, fmt.Errorf("attester keys map cannot be empty")
	}

	return &SignatureAggregator{
		signatures:      make(map[uint64]map[string][]byte),
		blockData:       make(map[string][]byte),
		quorumThreshold: quorumThreshold,
		attesterKeys:    attesterKeys,
		logger:          logger.With("component", "aggregator"),
	}, nil
}

// SetBlockData stores the data that was signed for a given block hash.
// This is typically called by the FSM after applying a block.
func (a *SignatureAggregator) SetBlockData(blockHash []byte, dataToSign []byte) {
	a.mu.Lock()
	defer a.mu.Unlock()

	blockHashStr := fmt.Sprintf("%x", blockHash)
	if _, exists := a.blockData[blockHashStr]; exists {
		a.logger.Warn("Block data already set, ignoring duplicate", "block_hash", blockHashStr)
		return
	}
	a.blockData[blockHashStr] = dataToSign
	a.logger.Debug("Stored data to sign for block", "block_hash", blockHashStr, "data_len", len(dataToSign))
	// TODO: Consider pruning old blockData entries
}

// AddSignature validates and adds a signature for a given block height and attester.
// It returns true if quorum was reached for the block after adding this signature.
func (a *SignatureAggregator) AddSignature(blockHeight uint64, blockHash []byte, attesterID string, signature []byte) (bool, error) {
	a.mu.Lock()
	// Defer unlock until the end, covering reads from blockData as well.
	defer a.mu.Unlock()

	blockHashStr := fmt.Sprintf("%x", blockHash)
	logArgs := []any{
		"block_height", blockHeight,
		"block_hash", blockHashStr,
		"attester_id", attesterID,
	}
	a.logger.Debug("Attempting to add signature", logArgs...)

	// --- Signature Verification ---
	// 1. Get the public key for the attesterID.
	pubKey, exists := a.attesterKeys[attesterID]
	if !exists {
		return false, fmt.Errorf("unknown attester ID: %s", attesterID)
	}
	if len(pubKey) != ed25519.PublicKeySize {
		return false, fmt.Errorf("invalid public key size for attester ID %s: expected %d, got %d",
			attesterID, ed25519.PublicKeySize, len(pubKey))
	}

	// 2. Get the expected data that was signed for this blockHash.
	expectedDataToSign, dataExists := a.blockData[blockHashStr]
	if !dataExists {
		// Data hasn't been set yet by the FSM (or was pruned). Cannot verify yet.
		// Option 1: Return error. Option 2: Store pending verification?
		// Let's return an error for now.
		a.logger.Warn("Cannot verify signature: data to sign not available (yet?) for block", logArgs...)
		return false, fmt.Errorf("cannot verify signature for block %s, data not available", blockHashStr)
	}

	// 3. Verify the signature.
	verified := ed25519.Verify(pubKey, expectedDataToSign, signature)
	if !verified {
		a.logger.Warn("Invalid signature received", logArgs...)
		return false, fmt.Errorf("invalid signature from attester %s for block %s", attesterID, blockHashStr)
	}
	a.logger.Debug("Signature verified successfully", logArgs...)

	// Initialize map for the block height if it doesn't exist
	if _, ok := a.signatures[blockHeight]; !ok {
		a.signatures[blockHeight] = make(map[string][]byte)
	}

	// Check if signature from this attester already exists for this block
	if _, exists := a.signatures[blockHeight][attesterID]; exists {
		a.logger.Warn("Duplicate signature received", logArgs...)
		quorumReached := len(a.signatures[blockHeight]) >= a.quorumThreshold
		return quorumReached, nil
	}

	// Store the validated signature
	a.signatures[blockHeight][attesterID] = signature
	a.logger.Info("Validated signature added successfully",
		append(logArgs,
			"signatures_count", len(a.signatures[blockHeight]),
			"quorum_threshold", a.quorumThreshold)...)

	// Check if quorum is now met
	quorumReached := len(a.signatures[blockHeight]) >= a.quorumThreshold
	if quorumReached {
		a.logger.Info("Quorum reached for block", "block_height", blockHeight, "block_hash", blockHashStr)
		// --- TODO: Trigger action on quorum ---
	}

	return quorumReached, nil
}

// GetAggregatedSignatures retrieves the collected valid signatures for a given block height
// if the quorum threshold has been met.
// It returns the slice of signatures and true if quorum is met, otherwise nil/empty slice and false.
func (a *SignatureAggregator) GetAggregatedSignatures(blockHeight uint64) ([][]byte, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	sigsForHeight, exists := a.signatures[blockHeight]
	if !exists {
		// No signatures recorded for this height yet
		return nil, false
	}

	if len(sigsForHeight) < a.quorumThreshold {
		// Quorum not yet met
		return nil, false
	}

	// Quorum met, collect signatures
	collectedSigs := make([][]byte, 0, len(sigsForHeight))
	for _, sig := range sigsForHeight {
		collectedSigs = append(collectedSigs, sig)
	}

	// Optionally sort the signatures for deterministic output?
	// sort.Slice(collectedSigs, func(i, j int) bool { ... })

	return collectedSigs, true
}

// TODO: Add methods like:
// - IsQuorumReached(blockHeight uint64) bool
// - PruneSignatures(olderThanHeight uint64) // To prevent memory leaks
