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
)

// ErrInvalidId is returned when the chain id is invalid
var (
	ErrInvalidId = errors.New("invalid chain id")

	initialBackoff = 100 * time.Millisecond

	maxSubmitAttempts = 30
	defaultMempoolTTL = 25
)

var _ coresequencer.Sequencer = &Sequencer{}

// Sequencer implements core sequencing interface
type Sequencer struct {
	logger log.Logger

	proposer bool

	Id []byte
	da coreda.DA

	batchTime time.Duration

	queue               *BatchQueue              // single queue for immediate availability
	batchSubmissionChan chan coresequencer.Batch // channel for ordered DA submission

	metrics *Metrics
}

// NewSequencer creates a new Single Sequencer
func NewSequencer(
	ctx context.Context,
	logger log.Logger,
	db ds.Batching,
	da coreda.DA,
	id []byte,
	batchTime time.Duration,
	metrics *Metrics,
	proposer bool,
) (*Sequencer, error) {
	s := &Sequencer{
		logger:    logger,
		da:        da,
		batchTime: batchTime,
		Id:        id,
		queue:     NewBatchQueue(db, "batches"),
		metrics:   metrics,
		proposer:  proposer,
	}

	loadCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.queue.Load(loadCtx); err != nil {
		return nil, fmt.Errorf("failed to load batch queue from DB: %w", err)
	}

	// No DA submission loop here; handled by central manager
	return s, nil
}

// SubmitBatchTxs implements sequencing.Sequencer.
func (c *Sequencer) SubmitBatchTxs(ctx context.Context, req coresequencer.SubmitBatchTxsRequest) (*coresequencer.SubmitBatchTxsResponse, error) {
	if !c.isValid(req.Id) {
		return nil, ErrInvalidId
	}

	if req.Batch == nil || len(req.Batch.Transactions) == 0 {
		c.logger.Info("Skipping submission of empty batch", "Id", string(req.Id))
		return &coresequencer.SubmitBatchTxsResponse{}, nil
	}

	batch := coresequencer.Batch{Transactions: req.Batch.Transactions}

	if c.batchSubmissionChan == nil {
		return nil, fmt.Errorf("sequencer mis-configured: batch submission channel is nil")
	}

	select {
	case c.batchSubmissionChan <- batch:
	default:
		return nil, fmt.Errorf("DA submission queue full, please retry later")
	}

	err := c.queue.AddBatch(ctx, batch)
	if err != nil {
		return nil, fmt.Errorf("failed to add batch: %w", err)
	}

	return &coresequencer.SubmitBatchTxsResponse{}, nil
}

// GetNextBatch implements sequencing.Sequencer.
func (c *Sequencer) GetNextBatch(ctx context.Context, req coresequencer.GetNextBatchRequest) (*coresequencer.GetNextBatchResponse, error) {
	if !c.isValid(req.Id) {
		return nil, ErrInvalidId
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

// RecordMetrics updates the metrics with the given values.
// This method is intended to be called by the block manager after submitting data to the DA layer.
func (c *Sequencer) RecordMetrics(gasPrice float64, blobSize uint64, statusCode coreda.StatusCode, numPendingBlocks uint64, includedBlockHeight uint64) {
	if c.metrics != nil {
		c.metrics.GasPrice.Set(gasPrice)
		c.metrics.LastBlobSize.Set(float64(blobSize))
		c.metrics.TransactionStatus.With("status", fmt.Sprintf("%d", statusCode)).Add(1)
		c.metrics.NumPendingBlocks.Set(float64(numPendingBlocks))
		c.metrics.IncludedBlockHeight.Set(float64(includedBlockHeight))
	}
}

// recordMetrics is a private method that updates the metrics with the given values.
func (c *Sequencer) recordMetrics(gasPrice float64, blobSize uint64, statusCode coreda.StatusCode, numPendingBlocks uint64, includedBlockHeight uint64) {
	c.RecordMetrics(gasPrice, blobSize, statusCode, numPendingBlocks, includedBlockHeight)
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
	if !c.isValid(req.Id) {
		return nil, ErrInvalidId
	}

	if !c.proposer {

		proofs, err := c.da.GetProofs(ctx, req.BatchData, []byte("placeholder"))
		if err != nil {
			return nil, fmt.Errorf("failed to get proofs: %w", err)
		}

		valid, err := c.da.Validate(ctx, req.BatchData, proofs, []byte("placeholder"))
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

func (c *Sequencer) isValid(Id []byte) bool {
	return bytes.Equal(c.Id, Id)
}

func (s *Sequencer) SetBatchSubmissionChan(batchSubmissionChan chan coresequencer.Batch) {
	s.batchSubmissionChan = batchSubmissionChan
}
