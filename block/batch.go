package block

import (
	"sync"
)

// BatchQueue is a queue of transactions
type BatchQueue struct {
	queue []BatchWithTime
	mu    sync.Mutex
}

// NewBatchQueue creates a new TransactionQueue
func NewBatchQueue() *BatchQueue {
	return &BatchQueue{
		queue: make([]BatchWithTime, 0),
	}
}

// AddBatch adds a new transaction to the queue
func (bq *BatchQueue) AddBatch(batch BatchWithTime) {
	bq.mu.Lock()
	defer bq.mu.Unlock()
	bq.queue = append(bq.queue, batch)
}

// Next returns the next transaction in the queue
func (bq *BatchQueue) Next() *BatchWithTime {
	bq.mu.Lock()
	defer bq.mu.Unlock()
	if len(bq.queue) == 0 {
		return nil
	}
	batch := bq.queue[0]
	bq.queue = bq.queue[1:]
	return &batch
}
