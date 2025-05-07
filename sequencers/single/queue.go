package single

import (
	"context"
	"fmt"
	"sync"

	ds "github.com/ipfs/go-datastore"
	ktds "github.com/ipfs/go-datastore/keytransform"
	"github.com/ipfs/go-datastore/query"
	"google.golang.org/protobuf/proto"

	coresequencer "github.com/rollkit/rollkit/core/sequencer"

	pb "github.com/rollkit/rollkit/types/pb/rollkit/v1"
)

func newPrefixKV(kvStore ds.Batching, prefix string) ds.Batching {
	return ktds.Wrap(kvStore, ktds.PrefixTransform{Prefix: ds.NewKey(prefix)})
}

// BatchQueue implements a persistent queue for transaction batches
type BatchQueue struct {
	queue  []coresequencer.Batch
	mu     sync.Mutex
	db     ds.Batching
	prefix string
}

// NewBatchQueue creates a new TransactionQueue
func NewBatchQueue(db ds.Batching, prefix string) *BatchQueue {
	return &BatchQueue{
		queue:  make([]coresequencer.Batch, 0),
		db:     newPrefixKV(db, prefix),
		prefix: prefix,
	}
}

// AddBatch adds a new transaction to the queue and writes it to the WAL
func (bq *BatchQueue) AddBatch(ctx context.Context, batch coresequencer.Batch) error {
	bq.mu.Lock()
	defer bq.mu.Unlock()

	hash, err := batch.Hash()
	if err != nil {
		return err
	}

	pbBatch := &pb.Batch{
		Txs: batch.Transactions,
	}

	encodedBatch, err := proto.Marshal(pbBatch)
	if err != nil {
		return err
	}

	// First write to DB for durability
	if err := bq.db.Put(ctx, ds.NewKey(string(hash)), encodedBatch); err != nil {
		return err
	}

	// Then add to in-memory queue
	bq.queue = append(bq.queue, batch)

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

	// List all entries from the datastore using the correct prefix
	q := query.Query{
		Prefix: bq.prefix, // Use the stored prefix for filtering
	}
	results, err := bq.db.Query(ctx, q)
	if err != nil {
		return fmt.Errorf("error querying datastore with prefix '%s': %w", bq.prefix, err)
	}
	defer results.Close()

	// Load each batch
	for result := range results.Next() {
		if result.Error != nil {
			fmt.Printf("Error reading entry from datastore with prefix '%s': %v\n", bq.prefix, result.Error)
			continue
		}
		pbBatch := &pb.Batch{}
		err := proto.Unmarshal(result.Value, pbBatch)
		if err != nil {
			return fmt.Errorf("Error decoding batch for key '%s' (prefix '%s'): %v. Attempting to delete entry.\n", result.Key, bq.prefix, err)
		}
		bq.queue = append(bq.queue, coresequencer.Batch{Transactions: pbBatch.Txs})
	}

	return nil
}
