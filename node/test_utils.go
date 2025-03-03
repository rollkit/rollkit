package node

import (
	"context"
	"fmt"
	"sync"
	"time"

	coresequencer "github.com/rollkit/rollkit/core/sequencer"
)

//---------------------
// DummySequencer
//---------------------

// dummySequencer is a dummy implementation of the Sequencer interface for testing
type dummySequencer struct {
	mu      sync.RWMutex
	batches map[string]*coresequencer.Batch
}

// NewDummySequencer creates a new dummy Sequencer instance
func NewDummySequencer() coresequencer.Sequencer {
	return &dummySequencer{
		batches: make(map[string]*coresequencer.Batch),
	}
}

// SubmitRollupBatchTxs submits a batch of transactions to the sequencer
func (s *dummySequencer) SubmitRollupBatchTxs(ctx context.Context, req coresequencer.SubmitRollupBatchTxsRequest) (*coresequencer.SubmitRollupBatchTxsResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.batches[string(req.RollupId)] = req.Batch
	return &coresequencer.SubmitRollupBatchTxsResponse{}, nil
}

// GetNextBatch gets the next batch from the sequencer
func (s *dummySequencer) GetNextBatch(ctx context.Context, req coresequencer.GetNextBatchRequest) (*coresequencer.GetNextBatchResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	batch, ok := s.batches[string(req.RollupId)]
	if !ok {
		return nil, fmt.Errorf("no batch found for rollup ID: %s", string(req.RollupId))
	}

	return &coresequencer.GetNextBatchResponse{
		Batch:     batch,
		Timestamp: time.Now(),
	}, nil
}

// VerifyBatch verifies a batch of transactions received from the sequencer
func (s *dummySequencer) VerifyBatch(ctx context.Context, req coresequencer.VerifyBatchRequest) (*coresequencer.VerifyBatchResponse, error) {
	return &coresequencer.VerifyBatchResponse{
		Status: true,
	}, nil
}
