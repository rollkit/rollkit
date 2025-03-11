package centralized

import (
	"context"
	"fmt"
	"sync"

	ds "github.com/ipfs/go-datastore"
	ktds "github.com/ipfs/go-datastore/keytransform"
	"github.com/ipfs/go-datastore/query"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"

	pb "github.com/rollkit/rollkit/types/pb/rollkit"
)

func newPrefixKV(kvStore ds.Batching, prefix string) ds.Batching {
	return (ktds.Wrap(kvStore, ktds.PrefixTransform{Prefix: ds.NewKey(prefix)}).Children()[0]).(ds.Batching)
}

// BatchQueue implements a persistent queue for transaction batches
type BatchQueue struct {
	queue []coresequencer.Batch
	mu    sync.Mutex
	// the datastore to store the batches
	// it is closed by the process which passes it in
	db ds.Batching
}

// NewBatchQueue creates a new TransactionQueue
func NewBatchQueue(db ds.Batching, prefix string) *BatchQueue {
	return &BatchQueue{
		queue: make([]coresequencer.Batch, 0),
		db:    newPrefixKV(db, prefix),
	}
}

// AddBatch adds a new transaction to the queue and writes it to the WAL
func (bq *BatchQueue) AddBatch(ctx context.Context, batch coresequencer.Batch) error {
	hash, err := batch.Hash()
	if err != nil {
		return err
	}

	pbBatch := &pb.Batch{
		Txs: batch.Transactions,
	}

	encodedBatch, err := pbBatch.Marshal()
	if err != nil {
		return err
	}

	// First write to WAL for durability
	if err := bq.db.Put(ctx, ds.NewKey(string(hash)), encodedBatch); err != nil {
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
		pbBatch := &pb.Batch{}
		err := pbBatch.Unmarshal(result.Value)
		if err != nil {
			fmt.Printf("Error decoding batch: %v\n", err)
			continue
		}
		bq.queue = append(bq.queue, coresequencer.Batch{Transactions: pbBatch.Txs})
	}

	return nil
}
