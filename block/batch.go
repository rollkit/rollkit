package block

import (
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	coresequencer "github.com/rollkit/rollkit/core/sequencer"
)

// BatchQueue is a queue of transaction batches with timestamps
type BatchQueue struct {
	queue    []BatchWithTime
	mu       sync.Mutex
	notifyCh chan struct{}
}

// BatchWithTime is used to pass batch and time to BatchQueue
type BatchWithTime struct {
	*coresequencer.Batch
	Time time.Time
	Data [][]byte
}

// NewBatchQueue creates a new BatchQueue
func NewBatchQueue() *BatchQueue {
	return &BatchQueue{
		queue:    make([]BatchWithTime, 0),
		notifyCh: make(chan struct{}, 1),
	}
}

// AddBatch adds a new batch to the queue
func (bq *BatchQueue) AddBatch(batch BatchWithTime) {
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

// convertBatchDataToBytes converts a batch data slice to bytes
func convertBatchDataToBytes(batchData [][]byte) []byte {
	// If batchData is nil or empty, return an empty byte slice
	if len(batchData) == 0 {
		return []byte{}
	}

	// For a single item, we still need to length-prefix it for consistency
	// First, calculate the total size needed
	// Format: 4 bytes (length) + data for each entry
	totalSize := 0
	for _, data := range batchData {
		totalSize += 4 + len(data) // 4 bytes for length prefix + data length
	}

	// Allocate buffer with calculated capacity
	result := make([]byte, 0, totalSize)

	// Add length-prefixed data
	for _, data := range batchData {
		// Encode length as 4-byte big-endian integer
		lengthBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(lengthBytes, uint32(len(data)))

		// Append length prefix
		result = append(result, lengthBytes...)

		// Append actual data
		result = append(result, data...)
	}

	return result
}

// bytesToBatchData converts a length-prefixed byte array back to a slice of byte slices
func bytesToBatchData(data []byte) ([][]byte, error) {
	if len(data) == 0 {
		return [][]byte{}, nil
	}

	var result [][]byte
	offset := 0

	for offset < len(data) {
		// Check if we have at least 4 bytes for the length prefix
		if offset+4 > len(data) {
			return nil, fmt.Errorf("corrupted data: insufficient bytes for length prefix at offset %d", offset)
		}

		// Read the length prefix
		length := binary.BigEndian.Uint32(data[offset : offset+4])
		offset += 4

		// Check if we have enough bytes for the data
		if offset+int(length) > len(data) {
			return nil, fmt.Errorf("corrupted data: insufficient bytes for entry of length %d at offset %d", length, offset)
		}

		// Extract the data entry
		entry := make([]byte, length)
		copy(entry, data[offset:offset+int(length)])
		result = append(result, entry)

		// Move to the next entry
		offset += int(length)
	}

	return result, nil
}
