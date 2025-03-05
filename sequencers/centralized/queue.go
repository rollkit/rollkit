package centralized

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"

	ds "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	pb "github.com/rollkit/rollkit/types/pb/rollkit"
)

// BatchQueue ...
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

// AddBatch adds a new transaction to the queue
func (bq *BatchQueue) AddBatch(ctx context.Context, batch coresequencer.Batch) error {
	bq.mu.Lock()
	bq.queue = append(bq.queue, batch)
	bq.mu.Unlock()

	// Get the hash and bytes of the batch
	h, err := batch.Hash()
	if err != nil {
		return err
	}

	// Marshal the batch
	protoBatch := &pb.Batch{
		Txs: batch.Transactions,
	}
	batchBytes, err := protoBatch.Marshal()
	if err != nil {
		return err
	}

	// Store the batch in BadgerDB
	return bq.db.Put(ctx, ds.NewKey(hex.EncodeToString(h)), batchBytes)

}

// Next extracts a batch of transactions from the queue
func (bq *BatchQueue) Next(ctx context.Context) (*coresequencer.Batch, error) {
	bq.mu.Lock()
	defer bq.mu.Unlock()
	if len(bq.queue) == 0 {
		return &coresequencer.Batch{Transactions: nil}, nil
	}
	batch := bq.queue[0]
	bq.queue = bq.queue[1:]

	h, err := batch.Hash()
	if err != nil {
		return &coresequencer.Batch{Transactions: nil}, err
	}

	// Remove the batch from BadgerDB after processing
	err = bq.db.Delete(ctx, ds.NewKey(hex.EncodeToString(h)))
	if err != nil {
		return &coresequencer.Batch{Transactions: nil}, err
	}

	return &batch, nil
}

// LoadFromDB reloads all batches from BadgerDB into the in-memory queue after a crash or restart.
func (bq *BatchQueue) LoadFromDB(ctx context.Context) error {
	bq.mu.Lock()
	defer bq.mu.Unlock()

	// TODO: implement this
	// err := db.View(func(txn *badger.Txn) error {
	// 	// Create an iterator to go through all batches stored in BadgerDB
	// 	it := txn.NewIterator(badger.DefaultIteratorOptions)
	// 	defer it.Close()

	// 	for it.Rewind(); it.Valid(); it.Next() {
	// 		item := it.Item()
	// 		err := item.Value(func(val []byte) error {
	// 			var batch coresequencer.Batch
	// 			// Unmarshal the batch bytes and add them to the in-memory queue
	// 			err := batch.Unmarshal(val)
	// 			if err != nil {
	// 				return err
	// 			}
	// 			bq.queue = append(bq.queue, batch)
	// 			return nil
	// 		})
	// 		if err != nil {
	// 			return err
	// 		}
	// 	}
	// 	return nil
	// })

	return nil
}

// TransactionQueue is a queue of transactions
type TransactionQueue struct {
	queue [][]byte
	mu    sync.Mutex

	db ds.Batching
}

// NewTransactionQueue creates a new TransactionQueue
func NewTransactionQueue(db ds.Batching) *TransactionQueue {
	return &TransactionQueue{
		queue: make([][]byte, 0),
		db:    db,
	}
}

// GetTransactionHash to get hash from transaction bytes using SHA-256
func GetTransactionHash(txBytes []byte) string {
	hashBytes := sha256.Sum256(txBytes)
	return hex.EncodeToString(hashBytes[:])
}

// AddTransaction adds a new transaction to the queue
func (tq *TransactionQueue) AddTransaction(ctx context.Context, tx []byte) error {
	tq.mu.Lock()
	tq.queue = append(tq.queue, tx)
	tq.mu.Unlock()

	// Store transaction in BadgerDB
	return tq.db.Put(ctx, ds.NewKey(GetTransactionHash(tx)), tx)
}

// GetNextBatch extracts a batch of transactions from the queue
func (tq *TransactionQueue) GetNextBatch(ctx context.Context, max uint64) coresequencer.Batch {
	tq.mu.Lock()
	defer tq.mu.Unlock()

	var batch [][]byte
	batchSize := len(tq.queue)
	if batchSize == 0 {
		return coresequencer.Batch{Transactions: nil}
	}
	for {
		batch = tq.queue[:batchSize]
		blobSize := totalBytes(batch)
		if uint64(blobSize) <= max {
			break
		}
		batchSize = batchSize - 1
	}

	// Use a batch operation for all DB operations
	dbBatch, err := tq.db.Batch(ctx)
	if err != nil {
		return coresequencer.Batch{Transactions: nil}
	}

	for _, tx := range batch {
		txHash := GetTransactionHash(tx)
		err := dbBatch.Delete(ctx, ds.NewKey("tx:"+txHash))
		if err != nil {
			return coresequencer.Batch{Transactions: nil}
		}
	}

	if err := dbBatch.Commit(ctx); err != nil {
		return coresequencer.Batch{Transactions: nil}
	}

	tq.queue = tq.queue[batchSize:]
	return coresequencer.Batch{Transactions: batch}
}

// LoadFromDB reloads all transactions from BadgerDB into the in-memory queue
func (tq *TransactionQueue) LoadFromDB(ctx context.Context) error {
	tq.mu.Lock()
	defer tq.mu.Unlock()

	// Clear existing queue
	tq.queue = make([][]byte, 0)

	// Query all transaction keys
	results, err := tq.db.Query(ctx, query.Query{
		Prefix: "tx:",
	})
	if err != nil {
		return err
	}
	defer results.Close()

	// Load transactions into memory
	for entry := range results.Next() {
		if entry.Error != nil {
			return entry.Error
		}
		tq.queue = append(tq.queue, entry.Value)
	}

	return nil
}

// AddBatchBackToQueue re-adds the batch to the transaction queue (and BadgerDB) after a failure.
func (tq *TransactionQueue) AddBatchBackToQueue(ctx context.Context, batch coresequencer.Batch) error {
	tq.mu.Lock()
	defer tq.mu.Unlock()

	// Add the batch back to the in-memory transaction queue
	tq.queue = append(tq.queue, batch.Transactions...)

	// Optionally, persist the batch back to BadgerDB
	for _, tx := range batch.Transactions {
		err := tq.db.Put(ctx, ds.NewKey(GetTransactionHash(tx)), tx)
		if err != nil {
			return fmt.Errorf("failed to revert transaction to DB: %w", err)
		}
	}

	return nil
}

func totalBytes(data [][]byte) int {
	total := 0
	for _, sub := range data {
		total += len(sub)
	}
	return total
}
