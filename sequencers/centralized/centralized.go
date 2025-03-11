package centralized

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
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
	logger log.Logger

	rollupId  []byte
	dalc      *dac.DAClient
	batchTime time.Duration

	maxBlobSize            *atomic.Uint64
	lastSubmittedBatchHash atomic.Value
	seenBatches            sync.Map

	// submitted batches
	sbq *BatchQueue
	// pending batches
	bq *BatchQueue

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
	dalc := dac.NewDAClient(da, -1, -1, coreda.Namespace(daNamespace), logger, nil)
	mBlobSize, err := dalc.DA.MaxBlobSize(ctx)
	if err != nil {
		return nil, err
	}

	maxBlobSize := &atomic.Uint64{}
	maxBlobSize.Store(mBlobSize)

	s := &Sequencer{
		logger:      logger,
		dalc:        dalc,
		batchTime:   batchTime,
		maxBlobSize: maxBlobSize,
		rollupId:    rollupId,
		// TODO: this can cause an overhead with IO
		bq:          NewBatchQueue(db, "pending"),
		sbq:         NewBatchQueue(db, "submitted"),
		seenBatches: sync.Map{},
		metrics:     metrics,
	}

	err = s.bq.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load batch queue from DB: %w", err)
	}

	err = s.sbq.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load submitted batch queue from DB: %w", err)
	}

	go s.batchSubmissionLoop(ctx)
	return s, nil
}

// SubmitRollupBatchTxs implements sequencing.Sequencer.
func (c *Sequencer) SubmitRollupBatchTxs(ctx context.Context, req coresequencer.SubmitRollupBatchTxsRequest) (*coresequencer.SubmitRollupBatchTxsResponse, error) {
	if !c.isValid(req.RollupId) {
		return nil, ErrInvalidRollupId
	}

	// create a batch from the set of transactions and store it in the queue
	batch := coresequencer.Batch{Transactions: req.Batch.Transactions}
	err := c.bq.AddBatch(ctx, batch)
	if err != nil {
		return nil, fmt.Errorf("failed to add batch: %w", err)
	}

	lastSubmittedBatchHash := c.lastSubmittedBatchHash.Load()
	var batchHash []byte
	if lastSubmittedBatchHash != nil {
		batchHash = lastSubmittedBatchHash.([]byte)
	}

	return &coresequencer.SubmitRollupBatchTxsResponse{LastSubmittedBatchHash: batchHash}, nil
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

// batchSubmissionLoop is a loop that publishes batches to the DA layer
// It is called in a separate goroutine when the Sequencer is created
func (c *Sequencer) batchSubmissionLoop(ctx context.Context) {
	batchTimer := time.NewTimer(0)
	defer batchTimer.Stop()

	for {
		select {
		case <-ctx.Done():
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

// publishBatch publishes a batch to the DA layer
func (c *Sequencer) publishBatch(ctx context.Context) error {
	batch, err := c.bq.Next(ctx)
	if err != nil {
		return fmt.Errorf("failed to get next batch: %w", err)
	}
	if batch.Transactions == nil {
		return nil
	}
	err = c.submitBatchToDA(ctx, *batch)
	if err != nil {
		// On failure, re-add the batch to the transaction queue for future retry
		revertErr := c.bq.AddBatch(ctx, *batch)
		if revertErr != nil {
			return fmt.Errorf("failed to revert batch to queue: %w", revertErr)
		}
		return fmt.Errorf("failed to submit batch to DA: %w", err)
	}

	err = c.sbq.AddBatch(ctx, *batch)
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

// submitBatchToDA submits a batch of transactions to the Data Availability (DA) layer.
// It implements a retry mechanism with exponential backoff and gas price adjustments
// to handle various failure scenarios.
//
// The function attempts to submit the batch multiple times (up to maxSubmitAttempts)
// with different strategies based on the response from the DA layer:
// - On success: Reduces gas price gradually (but not below initial price)
// - On mempool issues: Increases gas price and uses a longer backoff
// - On size issues: Reduces the blob size and uses exponential backoff
// - On other errors: Uses exponential backoff
//
// It returns an error if not all batches could be submitted after all attempts.
func (c *Sequencer) submitBatchToDA(ctx context.Context, batch coresequencer.Batch) error {
	// Initialize a slice with the batch to submit
	batchesToSubmit := []*coresequencer.Batch{&batch}
	submittedAllBlocks := false
	var backoff time.Duration
	numSubmittedBatches := 0
	attempt := 0

	// Store initial values to be able to reset or compare later
	maxBlobSize := c.maxBlobSize.Load()
	initialMaxBlobSize := maxBlobSize
	initialGasPrice := c.dalc.GasPrice
	gasPrice := c.dalc.GasPrice

daSubmitRetryLoop:
	for !submittedAllBlocks && attempt < maxSubmitAttempts {
		// Wait for backoff duration or exit if context is done
		select {
		case <-ctx.Done():
			break daSubmitRetryLoop
		case <-time.After(backoff):
		}

		// Attempt to submit the batch to the DA layer
		res := c.dalc.SubmitBatch(ctx, batchesToSubmit, maxBlobSize, gasPrice)
		switch res.Code {
		case dac.StatusSuccess:
			//TODO: upon success, mark the batch as processed in the WAL & update the last submitted batch hash
			// Count total transactions for logging
			txCount := 0
			for _, batch := range batchesToSubmit {
				txCount += len(batch.Transactions)
			}
			c.logger.Info("successfully submitted batches to DA layer", "gasPrice", gasPrice, "daHeight", res.DAHeight, "batchCount", res.SubmittedCount, "txCount", txCount)

			// Check if all batches were submitted
			if res.SubmittedCount == uint64(len(batchesToSubmit)) {
				submittedAllBlocks = true
			}

			// Split the batches into submitted and not submitted
			submittedBatches, notSubmittedBatches := batchesToSubmit[:res.SubmittedCount], batchesToSubmit[res.SubmittedCount:]
			numSubmittedBatches += len(submittedBatches)
			batchesToSubmit = notSubmittedBatches

			// Reset submission parameters after success
			backoff = 0
			maxBlobSize = initialMaxBlobSize

			// Gradually reduce gas price on success, but not below initial price
			if c.dalc.GasMultiplier > 0 && gasPrice != 0 {
				gasPrice = gasPrice / c.dalc.GasMultiplier
				if gasPrice < initialGasPrice {
					gasPrice = initialGasPrice
				}
			}
			c.logger.Debug("resetting DA layer submission options", "backoff", backoff, "gasPrice", gasPrice, "maxBlobSize", maxBlobSize)

		case dac.StatusNotIncludedInBlock, dac.StatusAlreadyInMempool:
			// For mempool-related issues, use a longer backoff and increase gas price
			c.logger.Error("DA layer submission failed", "error", res.Message, "attempt", attempt)
			backoff = c.batchTime * time.Duration(defaultMempoolTTL)

			// Increase gas price to prioritize the transaction
			if c.dalc.GasMultiplier > 0 && gasPrice != 0 {
				gasPrice = gasPrice * c.dalc.GasMultiplier
			}
			c.logger.Info("retrying DA layer submission with", "backoff", backoff, "gasPrice", gasPrice, "maxBlobSize", maxBlobSize)

		case dac.StatusTooBig:
			// If the blob is too big, reduce the max blob size
			maxBlobSize = maxBlobSize / 4 // TODO: this should be fetched from the DA layer?
			c.maxBlobSize.Store(maxBlobSize)
			fallthrough

		default:
			// For other errors, use exponential backoff
			c.logger.Error("DA layer submission failed", "error", res.Message, "attempt", attempt)
			backoff = c.exponentialBackoff(backoff)
		}

		// Record metrics for monitoring
		c.recordMetrics(gasPrice, res.BlobSize, res.Code, len(batchesToSubmit), res.DAHeight)
		attempt += 1
	}

	// Return error if not all batches were submitted after all attempts
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

// GetNextBatch implements sequencing.Sequencer.
func (c *Sequencer) GetNextBatch(ctx context.Context, req coresequencer.GetNextBatchRequest) (*coresequencer.GetNextBatchResponse, error) {
	if !c.isValid(req.RollupId) {
		return nil, ErrInvalidRollupId
	}
	now := time.Now()

	// Set the max size if it is provided
	if req.MaxBytes > 0 {
		c.CompareAndSetMaxSize(req.MaxBytes)
	}

	batch, err := c.sbq.Next(ctx)
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

	hexHash := hex.EncodeToString(h)
	c.seenBatches.Store(hexHash, struct{}{})

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
	key := hex.EncodeToString(req.BatchHash)
	_, exists := c.seenBatches.Load(key)
	if exists {
		return &coresequencer.VerifyBatchResponse{Status: true}, nil
	}
	return &coresequencer.VerifyBatchResponse{Status: false}, nil
}

func (c *Sequencer) isValid(rollupId []byte) bool {
	return bytes.Equal(c.rollupId, rollupId)
}
