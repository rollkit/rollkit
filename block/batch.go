package block

import (
	"sync"
)

// BatchQueue is a queue of transaction batches with timestamps
type BatchQueue struct {
	queue    []BatchData
	mu       sync.Mutex
	notifyCh chan struct{}
}

// NewBatchQueue creates a new BatchQueue
func NewBatchQueue() *BatchQueue {
	return &BatchQueue{
		queue:    make([]BatchData, 0),
		notifyCh: make(chan struct{}, 1),
	}
}

// AddBatch adds a new batch to the queue
func (bq *BatchQueue) AddBatch(batch BatchData) {
	bq.mu.Lock()
	defer bq.mu.Unlock()
	bq.queue = append(bq.queue, batch)
	select {
	case bq.notifyCh <- struct{}{}: // Send notification if there's no pending notification
	default:
		// Do nothing if a notification is already pending
	}
}

// Next returns the next batch in the queue
func (bq *BatchQueue) Next() *BatchData {
	bq.mu.Lock()
	defer bq.mu.Unlock()
	if len(bq.queue) == 0 {
		return nil
	}
	batch := bq.queue[0]
	bq.queue = bq.queue[1:]
	return &batch
}
