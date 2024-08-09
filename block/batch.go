package block

import (
	"sync"

	"github.com/rollkit/go-sequencing"
)

// BatchQueue is a queue of transactions
type BatchQueue struct {
	queue []sequencing.Batch
	mu    sync.Mutex
}

// NewBatchQueue creates a new TransactionQueue
func NewBatchQueue() *BatchQueue {
	return &BatchQueue{
		queue: make([]sequencing.Batch, 0),
	}
}

// AddBatch adds a new transaction to the queue
func (bq *BatchQueue) AddBatch(batch sequencing.Batch) {
	bq.mu.Lock()
	defer bq.mu.Unlock()
	bq.queue = append(bq.queue, batch)
}

// Next returns the next transaction in the queue
func (bq *BatchQueue) Next() *sequencing.Batch {
	bq.mu.Lock()
	defer bq.mu.Unlock()
	if len(bq.queue) == 0 {
		return nil
	}
	batch := bq.queue[0]
	bq.queue = bq.queue[1:]
	return &batch
}
