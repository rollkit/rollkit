package centralized

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"cosmossdk.io/log"
	ds "github.com/ipfs/go-datastore"

	coreda "github.com/rollkit/rollkit/core/da"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	dac "github.com/rollkit/rollkit/da"
)

// ErrInvalidRollupId is returned when the rollup id is invalid
var (
	ErrInvalidRollupId = errors.New("invalid rollup id")

	initialBackoff = 100 * time.Millisecond

	maxSubmitAttempts = 30
	defaultMempoolTTL = 25
)

var _ coresequencer.Sequencer = &Sequencer{}

// Sequencer implements go-sequencing interface
type Sequencer struct {
	ctx    context.Context
	logger log.Logger
	db     ds.Batching

	dalc        *dac.DAClient
	batchTime   time.Duration
	maxBlobSize *atomic.Uint64

	maxBytes atomic.Uint64
	maxGas   atomic.Uint64

	rollupId []byte

	tq            *TransactionQueue
	lastBatchHash atomic.Value

	seenBatches      map[string]struct{}
	seenBatchesMutex sync.RWMutex
	bq               *BatchQueue

	metrics *Metrics
}

// NewSequencer creates a new Centralized Sequencer
func NewSequencer(
	ctx context.Context,
	logger log.Logger,
	db ds.Batching,
	da coreda.DA,
	daNamespace,
	rollupId []byte,
	batchTime time.Duration,
	metrics *Metrics,
) (*Sequencer, error) {
	dalc := dac.NewDAClient(da, 0, 0, coreda.Namespace(daNamespace), logger, nil)
	mBlobSize, err := dalc.DA.MaxBlobSize(ctx)
	if err != nil {
		return nil, err
	}

	maxBlobSize := &atomic.Uint64{}
	maxBlobSize.Store(mBlobSize)

	s := &Sequencer{
		ctx:         ctx,
		logger:      logger,
		dalc:        dalc,
		batchTime:   batchTime,
		maxBlobSize: maxBlobSize,
		maxBytes:    atomic.Uint64{},
		maxGas:      atomic.Uint64{},
		rollupId:    rollupId,
		tq:          NewTransactionQueue(db),
		bq:          NewBatchQueue(db),
		seenBatches: make(map[string]struct{}),
		db:          db,
		metrics:     metrics,
	}

	// Load last batch hash from DB to recover from crash
	err = s.LoadLastBatchHashFromDB(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load last batch hash from DB: %w", err)
	}

	// Load seen batches from DB to recover from crash
	err = s.LoadSeenBatchesFromDB(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load seen batches from DB: %w", err)
	}

	// Load TransactionQueue and BatchQueue from DB to recover from crash
	err = s.tq.LoadFromDB(ctx) // Load transactions
	if err != nil {
		return nil, fmt.Errorf("failed to load transaction queue from DB: %w", err)
	}
	err = s.bq.LoadFromDB(ctx) // Load batches
	if err != nil {
		return nil, fmt.Errorf("failed to load batch queue from DB: %w", err)
	}

	go s.batchSubmissionLoop(s.ctx)
	return s, nil
}

// SetMaxBytes sets the max bytes
func (c *Sequencer) SetMaxBytes(size uint64) {
	c.maxBytes.Store(size)
}

// SetMaxGas sets the max gas
func (c *Sequencer) SetMaxGas(size uint64) {
	c.maxGas.Store(size)
}

// Close safely closes the BadgerDB instance if it is open
func (c *Sequencer) Close() error {
	if c.db != nil {
		err := c.db.Close()
		if err != nil {
			return fmt.Errorf("failed to close BadgerDB: %w", err)
		}
	}
	return nil
}

// CompareAndSetMaxSize compares the passed size with the current max size and sets the max size to the smaller of the two
// Initially the max size is set to the max blob size returned by the DA layer
// This can be overwritten by the execution client if it can only handle smaller size
func (c *Sequencer) CompareAndSetMaxSize(size uint64) {
	for {
		current := c.maxBlobSize.Load()
		if size >= current {
			return
		}
		if c.maxBlobSize.CompareAndSwap(current, size) {
			return
		}
	}
}

// LoadLastBatchHashFromDB loads the last batch hash from BadgerDB into memory after a crash or restart.
func (c *Sequencer) LoadLastBatchHashFromDB(ctx context.Context) error {
	// Load the last batch hash from BadgerDB if it exists
	bz, err := c.db.Get(ctx, ds.NewKey(lastBatchKey))
	if err != nil {
		return err
	}
	c.lastBatchHash.Store(bz)

	return nil
}

// LoadSeenBatchesFromDB loads the seen batches from BadgerDB into memory after a crash or restart.
func (c *Sequencer) LoadSeenBatchesFromDB(ctx context.Context) error {

	// TODO: what is the best way to load the last seen keys into memory, is this needed?
	// err := c.db.View(func(txn *badger.Txn) error {
	// 	// Create an iterator to go through all entries in BadgerDB
	// 	it := txn.NewIterator(badger.DefaultIteratorOptions)
	// 	defer it.Close()

	// 	for it.Rewind(); it.Valid(); it.Next() {
	// 		item := it.Item()
	// 		key := item.Key()
	// 		// Add the batch hash to the seenBatches map (for fast in-memory lookups)
	// 		c.seenBatches[string(key)] = struct{}{}
	// 	}
	// 	return nil
	// })

	return nil
}

func (c *Sequencer) setLastBatchHash(ctx context.Context, hash []byte) error {
	return c.db.Put(ctx, ds.NewKey(lastBatchKey), hash)
}

func (c *Sequencer) addSeenBatch(ctx context.Context, hash []byte) error {
	key := ds.NewKey("seen:" + hex.EncodeToString(hash))
	// Store with TTL if supported by the datastore implementation
	// For datastores that don't support TTL natively, implement a cleanup routine
	return c.db.Put(ctx, key, []byte{1})
}

func (c *Sequencer) batchSubmissionLoop(ctx context.Context) {
	batchTimer := time.NewTimer(0)
	defer batchTimer.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-batchTimer.C:
		}
		start := time.Now()
		err := c.publishBatch(ctx)
		if err != nil && ctx.Err() == nil {
			c.logger.Error("error while publishing block", "error", err)
		}
		batchTimer.Reset(getRemainingSleep(start, c.batchTime, 0))
	}
}

func (c *Sequencer) publishBatch(ctx context.Context) error {
	batch := c.tq.GetNextBatch(ctx, c.maxBlobSize.Load())
	if batch.Transactions == nil {
		return nil
	}
	err := c.submitBatchToDA(batch)
	if err != nil {
		// On failure, re-add the batch to the transaction queue for future retry
		revertErr := c.tq.AddBatchBackToQueue(ctx, batch)
		if revertErr != nil {
			return fmt.Errorf("failed to revert batch to queue: %w", revertErr)
		}
		return fmt.Errorf("failed to submit batch to DA: %w", err)
	}
	err = c.bq.AddBatch(ctx, batch)
	if err != nil {
		return err
	}
	return nil
}

func (c *Sequencer) recordMetrics(gasPrice float64, blobSize uint64, statusCode dac.StatusCode, numPendingBlocks int, includedBlockHeight uint64) {
	if c.metrics != nil {
		c.metrics.GasPrice.Set(gasPrice)
		c.metrics.LastBlobSize.Set(float64(blobSize))
		c.metrics.TransactionStatus.With("status", fmt.Sprintf("%d", statusCode)).Add(1)
		c.metrics.NumPendingBlocks.Set(float64(numPendingBlocks))
		c.metrics.IncludedBlockHeight.Set(float64(includedBlockHeight))
	}
}

func (c *Sequencer) submitBatchToDA(batch coresequencer.Batch) error {
	batchesToSubmit := []*coresequencer.Batch{&batch}
	submittedAllBlocks := false
	var backoff time.Duration
	numSubmittedBatches := 0
	attempt := 0

	maxBlobSize := c.maxBlobSize.Load()
	initialMaxBlobSize := maxBlobSize
	initialGasPrice := c.dalc.GasPrice
	gasPrice := c.dalc.GasPrice

daSubmitRetryLoop:
	for !submittedAllBlocks && attempt < maxSubmitAttempts {
		select {
		case <-c.ctx.Done():
			break daSubmitRetryLoop
		case <-time.After(backoff):
		}

		res := c.dalc.SubmitBatch(c.ctx, batchesToSubmit, maxBlobSize, gasPrice)
		switch res.Code {
		case dac.StatusSuccess:
			txCount := 0
			for _, batch := range batchesToSubmit {
				txCount += len(batch.Transactions)
			}
			c.logger.Info("successfully submitted batches to DA layer", "gasPrice", gasPrice, "daHeight", res.DAHeight, "batchCount", res.SubmittedCount, "txCount", txCount)
			if res.SubmittedCount == uint64(len(batchesToSubmit)) {
				submittedAllBlocks = true
			}
			submittedBatches, notSubmittedBatches := batchesToSubmit[:res.SubmittedCount], batchesToSubmit[res.SubmittedCount:]
			numSubmittedBatches += len(submittedBatches)
			batchesToSubmit = notSubmittedBatches
			// reset submission options when successful
			// scale back gasPrice gradually
			backoff = 0
			maxBlobSize = initialMaxBlobSize
			if c.dalc.GasMultiplier > 0 && gasPrice != 0 {
				gasPrice = gasPrice / c.dalc.GasMultiplier
				if gasPrice < initialGasPrice {
					gasPrice = initialGasPrice
				}
			}
			c.logger.Debug("resetting DA layer submission options", "backoff", backoff, "gasPrice", gasPrice, "maxBlobSize", maxBlobSize)
		case dac.StatusNotIncludedInBlock, dac.StatusAlreadyInMempool:
			c.logger.Error("DA layer submission failed", "error", res.Message, "attempt", attempt)
			backoff = c.batchTime * time.Duration(defaultMempoolTTL)
			if c.dalc.GasMultiplier > 0 && gasPrice != 0 {
				gasPrice = gasPrice * c.dalc.GasMultiplier
			}
			c.logger.Info("retrying DA layer submission with", "backoff", backoff, "gasPrice", gasPrice, "maxBlobSize", maxBlobSize)

		case dac.StatusTooBig:
			maxBlobSize = maxBlobSize / 4
			c.maxBlobSize.Store(maxBlobSize)
			fallthrough
		default:
			c.logger.Error("DA layer submission failed", "error", res.Message, "attempt", attempt)
			backoff = c.exponentialBackoff(backoff)
		}

		c.recordMetrics(gasPrice, res.BlobSize, res.Code, len(batchesToSubmit), res.DAHeight)
		attempt += 1
	}

	if !submittedAllBlocks {
		return fmt.Errorf(
			"failed to submit all blocks to DA layer, submitted %d blocks (%d left) after %d attempts",
			numSubmittedBatches,
			len(batchesToSubmit),
			attempt,
		)
	}
	return nil
}

func (c *Sequencer) exponentialBackoff(backoff time.Duration) time.Duration {
	backoff *= 2
	if backoff == 0 {
		backoff = initialBackoff
	}
	if backoff > c.batchTime {
		backoff = c.batchTime
	}
	return backoff
}

func getRemainingSleep(start time.Time, blockTime time.Duration, sleep time.Duration) time.Duration {
	elapsed := time.Since(start)
	remaining := blockTime - elapsed
	if remaining < 0 {
		return 0
	}
	return remaining + sleep
}

// SubmitRollupBatchTxs implements sequencing.Sequencer.
func (c *Sequencer) SubmitRollupBatchTxs(ctx context.Context, req coresequencer.SubmitRollupBatchTxsRequest) (*coresequencer.SubmitRollupBatchTxsResponse, error) {
	if !c.isValid(req.RollupId) {
		return nil, ErrInvalidRollupId
	}

	if len(req.Batch.Transactions) > 0 {
		for _, tx := range req.Batch.Transactions {
			err := c.tq.AddTransaction(ctx, tx)
			if err != nil {
				return nil, fmt.Errorf("failed to add transaction: %w", err)
			}
		}
	}

	return &coresequencer.SubmitRollupBatchTxsResponse{}, nil
}

// GetNextBatch implements sequencing.Sequencer.
func (c *Sequencer) GetNextBatch(ctx context.Context, req coresequencer.GetNextBatchRequest) (*coresequencer.GetNextBatchResponse, error) {
	if !c.isValid(req.RollupId) {
		return nil, ErrInvalidRollupId
	}
	now := time.Now()
	lastBatchHash := c.lastBatchHash.Load()

	if !reflect.DeepEqual(lastBatchHash, req.LastBatchHash) {
		return nil, fmt.Errorf("batch hash mismatch: lastBatchHash = %x, req.LastBatchHash = %x", lastBatchHash, req.LastBatchHash)
	}

	// Set the max size if it is provided
	if req.MaxBytes > 0 {
		c.CompareAndSetMaxSize(req.MaxBytes)
	}

	batch, err := c.bq.Next(ctx)
	if err != nil {
		return nil, err
	}

	batchRes := &coresequencer.GetNextBatchResponse{Batch: batch, Timestamp: now}
	if batch.Transactions == nil {
		return batchRes, nil
	}

	h, err := batch.Hash()
	if err != nil {
		return c.recover(ctx, *batch, err)
	}

	c.lastBatchHash.Store(h)
	err = c.setLastBatchHash(ctx, h)
	if err != nil {
		return c.recover(ctx, *batch, err)
	}

	hexHash := hex.EncodeToString(h)
	c.seenBatchesMutex.Lock()
	c.seenBatches[hexHash] = struct{}{}
	c.seenBatchesMutex.Unlock()
	err = c.addSeenBatch(ctx, h)
	if err != nil {
		return c.recover(ctx, *batch, err)
	}

	return batchRes, nil
}

func (c *Sequencer) recover(ctx context.Context, batch coresequencer.Batch, err error) (*coresequencer.GetNextBatchResponse, error) {
	// Revert the batch if Hash() errors out by adding it back to the BatchQueue
	revertErr := c.bq.AddBatch(ctx, batch)
	if revertErr != nil {
		return nil, fmt.Errorf("failed to revert batch: %w", revertErr)
	}
	return nil, fmt.Errorf("failed to generate hash for batch: %w", err)
}

// VerifyBatch implements sequencing.Sequencer.
func (c *Sequencer) VerifyBatch(ctx context.Context, req coresequencer.VerifyBatchRequest) (*coresequencer.VerifyBatchResponse, error) {
	//TODO: need to add DA verification
	if !c.isValid(req.RollupId) {
		return nil, ErrInvalidRollupId
	}
	c.seenBatchesMutex.RLock()
	defer c.seenBatchesMutex.RUnlock()
	key := hex.EncodeToString(req.BatchHash)
	if _, exists := c.seenBatches[key]; exists {
		return &coresequencer.VerifyBatchResponse{Status: true}, nil
	}
	return &coresequencer.VerifyBatchResponse{Status: false}, nil
}

func (c *Sequencer) isValid(rollupId []byte) bool {
	return bytes.Equal(c.rollupId, rollupId)
}
