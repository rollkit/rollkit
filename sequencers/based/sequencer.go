package based

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"cosmossdk.io/log"

	datastore "github.com/ipfs/go-datastore"

	coreda "github.com/rollkit/rollkit/core/da"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	"github.com/rollkit/rollkit/types"
)

var (
	initialBackoff    = 100 * time.Millisecond
	defaultMempoolTTL = 25
	maxSubmitAttempts = 30
	batchTime         = 1 * time.Second
)

const (
	DefaultMaxBlobSize     uint64 = 1_500_000
	dsPendingTxsKey               = "/sequencer/pendingTxs"
	dsLastScannedHeightKey        = "/sequencer/lastScannedDAHeight"
)

var (
	ErrInvalidRollupId = errors.New("invalid rollup id")
	ErrInvalidMaxBytes = errors.New("invalid max bytes")
)

var _ coresequencer.Sequencer = &Sequencer{}

// Sequencer is responsible for managing rollup transactions and interacting with the
// Data Availability (DA) layer. It handles tasks such as adding transactions to a
// pending queue, retrieving batches of transactions, verifying batches, and submitting
// them to the DA layer. The Sequencer ensures that transactions are processed in a
// reliable and efficient manner while adhering to constraints like maximum blob size
// and height drift.
// Sequencer represents a structure responsible for managing the sequencing of transactions
// and interacting with the Data Availability (DA) layer.
type Sequencer struct {
	// logger is used for logging messages and events within the Sequencer.
	logger log.Logger

	// maxHeightDrift defines the maximum allowable difference between the current
	// block height and the DA layer's block height.
	maxHeightDrift uint64

	// rollupId is the unique identifier for the rollup associated with this Sequencer.
	rollupId []byte

	// DA represents the Data Availability layer interface used by the Sequencer.
	DA coreda.DA

	// pendingTxs is a persistent storage for transactions that are pending inclusion
	// in the rollup blocks.
	pendingTxs *PersistentPendingTxs

	// daStartHeight specifies the starting block height in the Data Availability layer
	// from which the Sequencer begins processing.
	daStartHeight uint64

	// store is a batching datastore used for storing and retrieving data.
	store datastore.Batching
}

// NewSequencer creates a new Sequencer instance.
func NewSequencer(
	logger log.Logger,
	daImpl coreda.DA,
	rollupId []byte,
	daStartHeight uint64,
	maxHeightDrift uint64,
	ds datastore.Batching,
) (*Sequencer, error) {
	pending, err := NewPersistentPendingTxs(ds)
	if err != nil {
		return nil, err
	}
	return &Sequencer{
		logger:         logger,
		maxHeightDrift: maxHeightDrift,
		rollupId:       rollupId,
		DA:             daImpl,
		daStartHeight:  daStartHeight,
		pendingTxs:     pending,
		store:          ds,
	}, nil
}

// AddToPendingTxs adds transactions to the pending queue.
func (s *Sequencer) AddToPendingTxs(txs [][]byte, ids [][]byte, timestamp time.Time) {
	s.pendingTxs.Push(txs, ids, timestamp)
}

// SubmitRollupBatchTxs implements sequencer.Sequencer.
func (s *Sequencer) SubmitRollupBatchTxs(ctx context.Context, req coresequencer.SubmitRollupBatchTxsRequest) (*coresequencer.SubmitRollupBatchTxsResponse, error) {
	if !s.isValid(req.RollupId) {
		return nil, ErrInvalidRollupId
	}
	// Use the helper function for submission
	// Assuming default gas price (-1) and no options, similar to how DAClient was used before
	res := types.SubmitWithHelpers(ctx, s.DA, s.logger, req.Batch.Transactions, -1, []byte("placeholder"), nil)
	if res.Code != coreda.StatusSuccess {
		return nil, fmt.Errorf("failed to submit batch to DA via helper: %s", res.Message)
	}
	return &coresequencer.SubmitRollupBatchTxsResponse{}, nil
}

// GetNextBatch implements sequencer.Sequencer.
func (s *Sequencer) GetNextBatch(ctx context.Context, req coresequencer.GetNextBatchRequest) (*coresequencer.GetNextBatchResponse, error) {
	if !s.isValid(req.RollupId) {
		return nil, ErrInvalidRollupId
	}
	maxBytes := DefaultMaxBlobSize
	if req.MaxBytes != 0 {
		//return nil, ErrInvalidMaxBytes
		maxBytes = req.MaxBytes
	}

	// Fetch all pending transactions from the queue from the last DA height pull
	// if enough transactions are available, return the next batch
	// otherwise, try to fetch more transactions from DA using the next DA height
	// if the pulled transactions exceeds the maxBytes
	// push the remaining transactions back to the queue
	txs, ids, size, timestamp := s.pendingTxs.PopUpToMaxBytes(maxBytes)
	resp := &coresequencer.GetNextBatchResponse{
		Batch: &coresequencer.Batch{
			Transactions: txs,
		},
		BatchData: ids,
		Timestamp: timestamp,
	}

	// try to fetch more txs from the based layer
	lastDAHeight := s.daStartHeight
	lastScannedHeightRaw, err := s.store.Get(ctx, datastore.NewKey(dsLastScannedHeightKey))
	if err == nil {
		var scanned uint64
		_ = json.Unmarshal(lastScannedHeightRaw, &scanned)
		// DA start height will force the scan to start from the DA start height
		if scanned > lastDAHeight {
			lastDAHeight = scanned
		}
	}
	nextDAHeight := lastDAHeight

	if len(req.LastBatchData) > 0 {
		scanned := s.lastDAHeight(req.LastBatchData)
		if scanned > lastDAHeight {
			lastDAHeight = scanned
			nextDAHeight = lastDAHeight + 1
		}
	}
OuterLoop:
	for size < maxBytes {
		// if we have exceeded maxHeightDrift, stop fetching more transactions
		if nextDAHeight > lastDAHeight+s.maxHeightDrift {
			s.logger.Debug("exceeded max height drift, stopping fetching more transactions")
			break OuterLoop
		}
		// fetch the next batch of transactions from DA using the helper
		res := types.RetrieveWithHelpers(ctx, s.DA, s.logger, nextDAHeight)
		if res.Code == coreda.StatusError {
			// stop fetching more transactions and return the current batch
			s.logger.Warn("failed to retrieve transactions from DA layer via helper", "error", res.Message)
			break OuterLoop
		}
		if len(res.Data) == 0 { // TODO: some heights may not have rollup blobs, find a better way to handle this
			// stop fetching more transactions and return the current batch
			s.logger.Debug("no transactions to retrieve from DA layer via helper for", "height", nextDAHeight)
			// don't break yet, wait for maxHeightDrift to elapse
		} else if res.Code == coreda.StatusSuccess {
			for i, tx := range res.Data {
				txSize := uint64(len(tx))
				if size+txSize >= maxBytes {
					// Push remaining transactions back to the queue
					s.pendingTxs.Push(res.Data[i:], res.IDs[i:], res.Timestamp)
					break OuterLoop
				}
				resp.Batch.Transactions = append(resp.Batch.Transactions, tx)
				resp.BatchData = append(resp.BatchData, res.IDs[i])
				resp.Timestamp = res.Timestamp // update timestamp to the last one
				size += txSize
			}
		}
		// Always increment height to continue scanning, even if StatusNotFound or StatusError occurred for this height
		nextDAHeight++
	}

	s.logger.Debug("retrieved transactions from DA layer",
		"txs", len(resp.Batch.Transactions),
		"ids", len(resp.BatchData),
		"size", size,
		"timestamp", resp.Timestamp,
	)

	// Persist last scanned height
	s.store.Put(ctx, datastore.NewKey(dsLastScannedHeightKey), []byte(fmt.Sprintf("%d", nextDAHeight)))

	if len(resp.Batch.Transactions) == 0 {
		return nil, nil
	}
	return resp, nil
}

// VerifyBatch implements sequencer.Sequencer.
func (s *Sequencer) VerifyBatch(ctx context.Context, req coresequencer.VerifyBatchRequest) (*coresequencer.VerifyBatchResponse, error) {
	if !s.isValid(req.RollupId) {
		return nil, ErrInvalidRollupId
	}
	// Use stored namespace
	proofs, err := s.DA.GetProofs(ctx, req.BatchData, []byte("placeholder"))
	if err != nil {
		return nil, fmt.Errorf("failed to get proofs: %w", err)
	}

	// verify the proof
	valid, err := s.DA.Validate(ctx, req.BatchData, proofs, []byte("placeholder"))
	if err != nil {
		return nil, fmt.Errorf("failed to validate proof: %w", err)
	}
	// if all the proofs are valid, return true
	for _, v := range valid {
		if !v {
			return &coresequencer.VerifyBatchResponse{Status: false}, nil
		}
	}
	return &coresequencer.VerifyBatchResponse{Status: true}, nil
}

func (c *Sequencer) isValid(rollupId []byte) bool {
	return bytes.Equal(c.rollupId, rollupId)
}

func (s *Sequencer) lastDAHeight(ids [][]byte) uint64 {
	height, _ := coreda.SplitID(ids[len(ids)-1])
	return height
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
// This function is now simplified as it uses the SubmitWithHelpers function.
func (s *Sequencer) submitBatchToDA(ctx context.Context, batch coresequencer.Batch) error {
	// The complex retry logic is now encapsulated within the helper or the caller that needs it.
	// This sequencer's responsibility is simplified to just calling the helper.
	// Assuming default gas price (-1) and no options for this specific call context.
	res := types.SubmitWithHelpers(ctx, s.DA, s.logger, batch.Transactions, -1, []byte("placeholder"), nil)

	if res.Code != coreda.StatusSuccess {
		// Log the error returned by the helper
		s.logger.Error("Failed to submit batch to DA via helper", "error", res.Message, "status", res.Code)
		return fmt.Errorf("failed to submit batch to DA: %s (code: %d)", res.Message, res.Code)
	}

	s.logger.Info("[based] successfully submitted batch to DA layer via helper", "submitted_count", res.SubmittedCount)
	return nil
}

func (c *Sequencer) exponentialBackoff(backoff time.Duration) time.Duration {
	backoff *= 2
	if backoff == 0 {
		backoff = initialBackoff
	}
	if backoff > batchTime {
		backoff = batchTime
	}
	return backoff
}
