package centralized

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync"

	ds "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
)

// BatchQueue implements a persistent queue for transaction batches
type BatchQueue struct {
	queue []coresequencer.Batch
	mu    sync.Mutex
	db    ds.Batching
}

// NewBatchQueue creates a new TransactionQueue
func NewBatchQueue(db ds.Batching) *BatchQueue {

	return &BatchQueue{
		queue: make([]coresequencer.Batch, 0),
		db:    db,
	}
}

func (bq *BatchQueue) Close() error {
	return bq.db.Close()
}

// AddBatch adds a new transaction to the queue and writes it to the WAL
func (bq *BatchQueue) AddBatch(ctx context.Context, batch coresequencer.Batch) error {
	hash, err := batch.Hash()
	if err != nil {
		return err
	}

	// First write to WAL for durability
	if err := bq.db.Put(ctx, ds.NewKey(string(hash)), encode(&batch)); err != nil {
		return err
	}

	// Then add to in-memory queue
	bq.mu.Lock()
	bq.queue = append(bq.queue, batch)
	bq.mu.Unlock()

	return nil
}

// Next extracts a batch of transactions from the queue and marks it as processed in the WAL
func (bq *BatchQueue) Next(ctx context.Context) (*coresequencer.Batch, error) {
	bq.mu.Lock()
	defer bq.mu.Unlock()

	if len(bq.queue) == 0 {
		return &coresequencer.Batch{Transactions: nil}, nil
	}

	batch := bq.queue[0]
	bq.queue = bq.queue[1:]

	hash, err := batch.Hash()
	if err != nil {
		return &coresequencer.Batch{Transactions: nil}, err
	}

	// Delete the batch from the WAL since it's been processed
	err = bq.db.Delete(ctx, ds.NewKey(string(hash)))
	if err != nil {
		// Log the error but continue
		fmt.Printf("Error deleting processed batch: %v\n", err)
	}

	return &batch, nil
}

// Load reloads all batches from WAL file into the in-memory queue after a crash or restart
func (bq *BatchQueue) Load(ctx context.Context) error {
	bq.mu.Lock()
	defer bq.mu.Unlock()

	// Clear the current queue
	bq.queue = make([]coresequencer.Batch, 0)

	// List all entries from the datastore
	results, err := bq.db.Query(ctx, query.Query{})
	if err != nil {
		return err
	}
	defer results.Close()

	// Load each batch
	for result := range results.Next() {
		batch, err := decode(result.Value)
		if err != nil {
			fmt.Printf("Error decoding batch: %v\n", err)
			continue
		}
		bq.queue = append(bq.queue, batch)
	}

	return nil
}

// Encode serializes the batch into a byte slice with minimal overhead
// Format: [num_txs (8 bytes)][tx1_len (8 bytes)][tx1_data][tx2_len (8 bytes)][tx2_data]...
func encode(batch *coresequencer.Batch) []byte {
	if batch == nil || len(batch.Transactions) == 0 {
		// For empty/nil batch, just return 8 bytes of zeros (num_txs = 0)
		return make([]byte, 8)
	}

	// Calculate total size needed
	totalSize := 8 // 8 bytes for number of transactions
	for _, tx := range batch.Transactions {
		totalSize += 8 + len(tx) // 8 bytes for length + transaction data
	}

	// Allocate buffer with exact size needed
	buf := make([]byte, totalSize)
	offset := 0

	// Write number of transactions
	binary.BigEndian.PutUint64(buf[offset:], uint64(len(batch.Transactions)))
	offset += 8

	// Write each transaction
	for _, tx := range batch.Transactions {
		// Write transaction length
		binary.BigEndian.PutUint64(buf[offset:], uint64(len(tx)))
		offset += 8
		// Write transaction data
		copy(buf[offset:], tx)
		offset += len(tx)
	}

	return buf
}

// Decode deserializes a byte slice into a batch
func decode(data []byte) (coresequencer.Batch, error) {
	batch := coresequencer.Batch{}

	if len(data) < 8 {
		return coresequencer.Batch{}, fmt.Errorf("invalid batch data: too short")
	}

	// Read number of transactions
	numTxs := binary.BigEndian.Uint64(data[:8])
	offset := 8

	// Initialize transactions slice
	batch.Transactions = make([][]byte, numTxs)

	// Read each transaction
	for i := uint64(0); i < numTxs; i++ {
		if len(data[offset:]) < 8 {
			return coresequencer.Batch{}, fmt.Errorf("invalid batch data: truncated at transaction %d length", i)
		}

		// Read transaction length
		txLen := binary.BigEndian.Uint64(data[offset:])
		offset += 8

		if len(data[offset:]) < int(txLen) {
			return coresequencer.Batch{}, fmt.Errorf("invalid batch data: truncated at transaction %d data", i)
		}

		// Read transaction data
		batch.Transactions[i] = make([]byte, txLen)
		copy(batch.Transactions[i], data[offset:offset+int(txLen)])
		offset += int(txLen)
	}

	return batch, nil
}
