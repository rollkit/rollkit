package single

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"cosmossdk.io/log"
	ds "github.com/ipfs/go-datastore"

	coreda "github.com/rollkit/rollkit/core/da"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	"github.com/rollkit/rollkit/types"
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

	proposer bool

	rollupId    []byte
	da          coreda.DA
	daNamespace []byte
	batchTime   time.Duration

	queue            *BatchQueue
	daSubmissionChan chan coresequencer.Batch

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
	proposer bool,
) (*Sequencer, error) {
	s := &Sequencer{
		logger:           logger,
		da:               da,
		daNamespace:      daNamespace,
		batchTime:        batchTime,
		rollupId:         rollupId,
		queue:            NewBatchQueue(db, "batches"),
		daSubmissionChan: make(chan coresequencer.Batch, 100),
		metrics:          metrics,
		proposer:         proposer,
	}

	loadCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.queue.Load(loadCtx); err != nil {
		return nil, fmt.Errorf("failed to load batch queue from DB: %w", err)
	}

	// Start the DA submission loop
	go s.daSubmissionLoop(ctx)
	return s, nil
}

// SubmitRollupBatchTxs implements sequencing.Sequencer.
func (c *Sequencer) SubmitRollupBatchTxs(ctx context.Context, req coresequencer.SubmitRollupBatchTxsRequest) (*coresequencer.SubmitRollupBatchTxsResponse, error) {
	if !c.isValid(req.RollupId) {
		return nil, ErrInvalidRollupId
	}

	if req.Batch == nil || len(req.Batch.Transactions) == 0 {
		c.logger.Info("Skipping submission of empty batch", "rollupId", string(req.RollupId))
		return &coresequencer.SubmitRollupBatchTxsResponse{}, nil
	}

	batch := coresequencer.Batch{Transactions: req.Batch.Transactions}

	select {
	case c.daSubmissionChan <- batch:
	default:
		return nil, fmt.Errorf("DA submission queue full, please retry later")
	}

	err := c.queue.AddBatch(ctx, batch)
	if err != nil {
		return nil, fmt.Errorf("failed to add batch: %w", err)
	}

	return &coresequencer.SubmitRollupBatchTxsResponse{}, nil
}

// daSubmissionLoop processes batches for DA submission in order
func (c *Sequencer) daSubmissionLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			c.logger.Info("DA submission loop stopped")
			return
		case batch := <-c.daSubmissionChan:
			err := c.submitBatchToDA(ctx, batch)
			if err != nil {
				c.logger.Error("failed to submit batch to DA", "error", err)
			}
		}
	}
}

// GetNextBatch implements sequencing.Sequencer.
func (c *Sequencer) GetNextBatch(ctx context.Context, req coresequencer.GetNextBatchRequest) (*coresequencer.GetNextBatchResponse, error) {
	if !c.isValid(req.RollupId) {
		return nil, ErrInvalidRollupId
	}

	batch, err := c.queue.Next(ctx)
	if err != nil {
		return nil, err
	}

	return &coresequencer.GetNextBatchResponse{
		Batch:     batch,
		Timestamp: time.Now(),
	}, nil
}

func (c *Sequencer) recordMetrics(gasPrice float64, blobSize uint64, statusCode coreda.StatusCode, numPendingBlocks int, includedBlockHeight uint64) {
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
// The function attempts to submit a batch multiple times (up to maxSubmitAttempts),
// handling partial submissions where only some transactions within the batch are accepted.
// Different strategies are used based on the response from the DA layer:
// - On success: Reduces gas price gradually (but not below initial price)
// - On mempool issues: Increases gas price and uses a longer backoff
// - On size issues: Reduces the blob size and uses exponential backoff
// - On other errors: Uses exponential backoff
//
// It returns an error if not all transactions could be submitted after all attempts.
func (c *Sequencer) submitBatchToDA(ctx context.Context, batch coresequencer.Batch) error {
	currentBatch := batch
	submittedAllTxs := false
	var backoff time.Duration
	totalTxCount := len(batch.Transactions)
	submittedTxCount := 0
	attempt := 0

	// Store initial values to be able to reset or compare later
	initialGasPrice, err := c.da.GasPrice(ctx)
	if err != nil {
		return fmt.Errorf("failed to get initial gas price: %w", err)
	}

	gasPrice := initialGasPrice

daSubmitRetryLoop:
	for !submittedAllTxs && attempt < maxSubmitAttempts {
		// Wait for backoff duration or exit if context is done
		select {
		case <-ctx.Done():
			break daSubmitRetryLoop
		case <-time.After(backoff):
		}

		// Attempt to submit the batch to the DA layer using the helper function
		res := types.SubmitWithHelpers(ctx, c.da, c.logger, currentBatch.Transactions, gasPrice, c.daNamespace, nil)

		gasMultiplier, multErr := c.da.GasMultiplier(ctx)
		if multErr != nil {
			c.logger.Error("failed to get gas multiplier", "error", multErr)
			gasMultiplier = 0
		}

		switch res.Code {
		case coreda.StatusSuccess:
			submittedTxs := int(res.SubmittedCount)
			c.logger.Info("successfully submitted transactions to DA layer",
				"gasPrice", gasPrice,
				"height", res.Height,
				"submittedTxs", submittedTxs,
				"remainingTxs", len(currentBatch.Transactions)-submittedTxs)

			submittedTxCount += submittedTxs

			// Check if all transactions in the current batch were submitted
			if submittedTxs == len(currentBatch.Transactions) {
				submittedAllTxs = true
			} else {
				// Update the current batch to contain only the remaining transactions
				currentBatch.Transactions = currentBatch.Transactions[submittedTxs:]
			}

			// Reset submission parameters after success
			backoff = 0

			// Gradually reduce gas price on success, but not below initial price
			if gasMultiplier > 0 && gasPrice != 0 {
				gasPrice = gasPrice / gasMultiplier
				if gasPrice < initialGasPrice {
					gasPrice = initialGasPrice
				}
			}
			c.logger.Debug("resetting DA layer submission options", "backoff", backoff, "gasPrice", gasPrice)

		case coreda.StatusNotIncludedInBlock, coreda.StatusAlreadyInMempool:
			c.logger.Error("single sequencer: DA layer submission failed", "error", res.Message, "attempt", attempt)
			backoff = c.batchTime * time.Duration(defaultMempoolTTL)
			if gasMultiplier > 0 && gasPrice != 0 {
				gasPrice = gasPrice * gasMultiplier
			}
			c.logger.Info("retrying DA layer submission with", "backoff", backoff, "gasPrice", gasPrice)

		case coreda.StatusTooBig:
			// Blob size adjustment is handled within DA impl or SubmitWithOptions call
			// fallthrough to default exponential backoff
			fallthrough

		default:
			c.logger.Error("single sequencer: DA layer submission failed", "error", res.Message, "attempt", attempt)
			backoff = c.exponentialBackoff(backoff)
		}

		c.recordMetrics(gasPrice, res.BlobSize, res.Code, len(currentBatch.Transactions), res.Height)
		attempt++
	}

	// Return error if not all transactions were submitted after all attempts
	if !submittedAllTxs {
		return fmt.Errorf(
			"failed to submit all transactions to DA layer, submitted %d txs (%d left) after %d attempts",
			submittedTxCount,
			totalTxCount-submittedTxCount,
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

// VerifyBatch implements sequencing.Sequencer.
func (c *Sequencer) VerifyBatch(ctx context.Context, req coresequencer.VerifyBatchRequest) (*coresequencer.VerifyBatchResponse, error) {
	if !c.isValid(req.RollupId) {
		return nil, ErrInvalidRollupId
	}

	if !c.proposer {

		proofs, err := c.da.GetProofs(ctx, req.BatchData, c.daNamespace)
		if err != nil {
			return nil, fmt.Errorf("failed to get proofs: %w", err)
		}

		valid, err := c.da.Validate(ctx, req.BatchData, proofs, c.daNamespace)
		if err != nil {
			return nil, fmt.Errorf("failed to validate proof: %w", err)
		}

		for _, v := range valid {
			if !v {
				return &coresequencer.VerifyBatchResponse{Status: false}, nil
			}
		}
		return &coresequencer.VerifyBatchResponse{Status: true}, nil
	}
	return &coresequencer.VerifyBatchResponse{Status: true}, nil
}

func (c *Sequencer) isValid(rollupId []byte) bool {
	return bytes.Equal(c.rollupId, rollupId)
}
