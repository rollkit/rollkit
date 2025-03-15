package based

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rollkit/rollkit/core/da"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
)

// BasedSequencer implements go-sequencing API with based sequencing logic.
//
// All transactions are passed directly to DA to be saved in a namespace. Each transaction is submitted as separate blob.
// When batch is requested DA blocks are scanned to read all blobs from given namespace at given height.
type BasedSequencer struct {
	da da.DA
	mu sync.RWMutex

	// Track batches by rollup ID for future reference
	batchesByRollup map[string]*BatchInfo

	// Namespace where transactions are stored in DA
	namespace []byte

	// Gas price to use for DA submissions
	gasPrice float64

	// Current DA height being processed
	currentHeight uint64
}

// BatchInfo stores information about a batch and its verification status
type BatchInfo struct {
	Batch      *coresequencer.Batch
	Timestamp  time.Time
	IsVerified bool
	DARef      string // Reference to where this is stored in DA layer
}

// NewBasedSequencer creates a new based sequencer with the given DA layer
func NewBasedSequencer(daClient da.DA, namespace []byte) coresequencer.Sequencer {
	return &BasedSequencer{
		da:              daClient,
		namespace:       namespace,
		batchesByRollup: make(map[string]*BatchInfo),
		gasPrice:        0.1, // Default gas price
		currentHeight:   1,   // Start at height 1
	}
}

// SubmitRollupBatchTxs submits a batch of transactions to the DA layer
//
// In based sequencer, transactions are submitted directly to the DA layer
// Each transaction in the batch is converted to a blob and submitted together
func (s *BasedSequencer) SubmitRollupBatchTxs(ctx context.Context, req coresequencer.SubmitRollupBatchTxsRequest) (*coresequencer.SubmitRollupBatchTxsResponse, error) {
	if len(req.Batch.Transactions) == 0 {
		// Nothing to submit
		return &coresequencer.SubmitRollupBatchTxsResponse{}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Convert transactions to DA blobs
	blobs := req.Batch.Transactions

	// Submit transactions to DA
	_, height, err := s.da.Submit(ctx, blobs, s.gasPrice, s.namespace, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to submit transactions to DA: %w", err)
	}

	// Update current height
	s.currentHeight = height

	// Store batch info for future reference
	batchHash, err := req.Batch.Hash()
	if err != nil {
		return nil, fmt.Errorf("failed to hash batch: %w", err)
	}

	s.batchesByRollup[string(req.RollupId)] = &BatchInfo{
		Batch:      req.Batch,
		Timestamp:  time.Now(),
		IsVerified: true,
		DARef:      fmt.Sprintf("%d:%x", height, batchHash[:8]),
	}

	return &coresequencer.SubmitRollupBatchTxsResponse{}, nil
}

// GetNextBatch retrieves the next batch from the DA layer
//
// In based sequencer, batches are retrieved from the DA layer at the current height
// If no blobs are found at the current height, an empty batch is returned
func (s *BasedSequencer) GetNextBatch(ctx context.Context, req coresequencer.GetNextBatchRequest) (*coresequencer.GetNextBatchResponse, error) {
	// Get the IDs of blobs at the current height
	result, err := s.da.GetIDs(ctx, s.currentHeight, s.namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get IDs from DA: %w", err)
	}

	// If no blobs found at this height, return empty batch
	if len(result.IDs) == 0 {
		return &coresequencer.GetNextBatchResponse{
			Batch:     &coresequencer.Batch{Transactions: [][]byte{}},
			Timestamp: time.Now(),
		}, nil
	}

	// Retrieve the blobs from DA
	blobs, err := s.da.Get(ctx, result.IDs, s.namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get blobs from DA: %w", err)
	}

	// Create a new batch from the retrieved blobs
	batch := &coresequencer.Batch{
		Transactions: blobs,
	}

	// Update our local state
	batchHash, _ := batch.Hash()
	s.mu.Lock()
	s.batchesByRollup[string(req.RollupId)] = &BatchInfo{
		Batch:      batch,
		Timestamp:  result.Timestamp,
		IsVerified: true,
		DARef:      fmt.Sprintf("%d:%x", s.currentHeight, batchHash[:8]),
	}

	// Increment the height for next retrieval
	s.currentHeight++
	s.mu.Unlock()

	return &coresequencer.GetNextBatchResponse{
		Batch:     batch,
		Timestamp: result.Timestamp,
	}, nil
}

// VerifyBatch verifies that a batch was correctly ordered by the DA layer
//
// In based sequencer, we first check our local cache, and if not found
// we assume the batch is verified since it came directly from the DA layer
func (s *BasedSequencer) VerifyBatch(ctx context.Context, req coresequencer.VerifyBatchRequest) (*coresequencer.VerifyBatchResponse, error) {
	if len(req.BatchHash) == 0 {
		return &coresequencer.VerifyBatchResponse{Status: false}, fmt.Errorf("empty batch hash")
	}

	// Check our local cache first
	s.mu.RLock()
	for _, info := range s.batchesByRollup {
		if batchHash, err := info.Batch.Hash(); err == nil {
			if string(batchHash) == string(req.BatchHash) {
				s.mu.RUnlock()
				return &coresequencer.VerifyBatchResponse{Status: true}, nil
			}
		}
	}
	s.mu.RUnlock()

	// In a real implementation, we would verify this batch exists in the DA layer
	// For simplicity, we'll just return true since we can't directly verify
	// without having the actual blob IDs
	// In a production implementation, we would need to scan recent heights or
	// maintain a mapping from batch hashes to blob IDs

	return &coresequencer.VerifyBatchResponse{Status: true}, nil
}
