package sequencer

import (
	"context"
	"sync"
	"time"
)

//---------------------
// DummySequencer
//---------------------

var _ Sequencer = (*DummySequencer)(nil)

// DummySequencer is a dummy implementation of the Sequencer interface for testing
type DummySequencer struct {
	mu                  sync.RWMutex
	batches             map[string]*Batch
	batchSubmissionChan chan Batch
}

// NewDummySequencer creates a new dummy Sequencer instance
func NewDummySequencer() *DummySequencer {
	return &DummySequencer{
		batches: make(map[string]*Batch),
	}
}

// SubmitBatchTxs submits a batch of transactions to the sequencer
func (s *DummySequencer) SubmitBatchTxs(ctx context.Context, req SubmitBatchTxsRequest) (*SubmitBatchTxsResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.batches[string(req.Id)] = req.Batch
	if req.Batch != nil && len(req.Batch.Transactions) > 0 {
		s.batchSubmissionChan <- *req.Batch
	}
	return &SubmitBatchTxsResponse{}, nil
}

// GetNextBatch gets the next batch from the sequencer
func (s *DummySequencer) GetNextBatch(ctx context.Context, req GetNextBatchRequest) (*GetNextBatchResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	batch, ok := s.batches[string(req.Id)]
	if !ok {
		batch = &Batch{Transactions: nil}
	}

	return &GetNextBatchResponse{
		Batch:     batch,
		Timestamp: time.Now(),
	}, nil
}

// VerifyBatch verifies a batch of transactions received from the sequencer
func (s *DummySequencer) VerifyBatch(ctx context.Context, req VerifyBatchRequest) (*VerifyBatchResponse, error) {
	return &VerifyBatchResponse{
		Status: true,
	}, nil
}

func (s *DummySequencer) SetBatchSubmissionChan(batchSubmissionChan chan Batch) {
	s.batchSubmissionChan = batchSubmissionChan
}
