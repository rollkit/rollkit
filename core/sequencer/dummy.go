package sequencer

import (
	"context"
	"sync"
	"time"
)

//---------------------
// DummySequencer
//---------------------

// dummySequencer is a dummy implementation of the Sequencer interface for testing
type dummySequencer struct {
	mu                  sync.RWMutex
	batches             map[string]*Batch
	batchSubmissionChan chan Batch
}

// NewDummySequencer creates a new dummy Sequencer instance
func NewDummySequencer() Sequencer {
	return &dummySequencer{
		batches: make(map[string]*Batch),
	}
}

// SubmitRollupBatchTxs submits a batch of transactions to the sequencer
func (s *dummySequencer) SubmitRollupBatchTxs(ctx context.Context, req SubmitRollupBatchTxsRequest) (*SubmitRollupBatchTxsResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.batches[string(req.RollupId)] = req.Batch
	if req.Batch != nil && len(req.Batch.Transactions) > 0 {
		s.batchSubmissionChan <- *req.Batch
	}
	return &SubmitRollupBatchTxsResponse{}, nil
}

// GetNextBatch gets the next batch from the sequencer
func (s *dummySequencer) GetNextBatch(ctx context.Context, req GetNextBatchRequest) (*GetNextBatchResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	batch, ok := s.batches[string(req.RollupId)]
	if !ok {
		batch = &Batch{Transactions: nil}
	}

	return &GetNextBatchResponse{
		Batch:     batch,
		Timestamp: time.Now(),
	}, nil
}

// VerifyBatch verifies a batch of transactions received from the sequencer
func (s *dummySequencer) VerifyBatch(ctx context.Context, req VerifyBatchRequest) (*VerifyBatchResponse, error) {
	return &VerifyBatchResponse{
		Status: true,
	}, nil
}

func (s *dummySequencer) SetBatchSubmissionChan(batchSubmissionChan chan Batch) {
	s.batchSubmissionChan = batchSubmissionChan
}
