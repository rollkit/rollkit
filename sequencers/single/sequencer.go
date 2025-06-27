package single

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rollkit/rollkit/pkg/log"
	ds "github.com/ipfs/go-datastore"

	coreda "github.com/rollkit/rollkit/core/da"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
)

// ErrInvalidId is returned when the chain id is invalid
var (
	ErrInvalidId = errors.New("invalid chain id")
)

var _ coresequencer.Sequencer = &Sequencer{}

// Sequencer implements core sequencing interface
type Sequencer struct {
	logger log.Logger

	proposer bool

	Id []byte
	da coreda.DA

	batchTime time.Duration

	queue *BatchQueue // single queue for immediate availability

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
	return NewSequencerWithQueueSize(ctx, logger, db, da, id, batchTime, metrics, proposer, 1000)
}

// NewSequencerWithQueueSize creates a new Single Sequencer with configurable queue size
func NewSequencerWithQueueSize(
	ctx context.Context,
	logger log.Logger,
	db ds.Batching,
	da coreda.DA,
	id []byte,
	batchTime time.Duration,
	metrics *Metrics,
	proposer bool,
	maxQueueSize int,
) (*Sequencer, error) {
	s := &Sequencer{
		logger:    logger,
		da:        da,
		batchTime: batchTime,
		Id:        id,
		queue:     NewBatchQueue(db, "batches", maxQueueSize),
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

	err := c.queue.AddBatch(ctx, batch)
	if err != nil {
		if errors.Is(err, ErrQueueFull) {
			c.logger.Warn("Batch queue is full, rejecting batch submission",
				"txCount", len(batch.Transactions),
				"chainId", string(req.Id))
			return nil, fmt.Errorf("batch queue is full, cannot accept more batches: %w", err)
		}
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

// VerifyBatch implements sequencing.Sequencer.
func (c *Sequencer) VerifyBatch(ctx context.Context, req coresequencer.VerifyBatchRequest) (*coresequencer.VerifyBatchResponse, error) {
	if !c.isValid(req.Id) {
		return nil, ErrInvalidId
	}

	if !c.proposer {

		proofs, err := c.da.GetProofs(ctx, req.BatchData, c.Id)
		if err != nil {
			return nil, fmt.Errorf("failed to get proofs: %w", err)
		}

		valid, err := c.da.Validate(ctx, req.BatchData, proofs, c.Id)
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
