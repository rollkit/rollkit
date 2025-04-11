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

type TxsWithTimestamp struct {
	Txs       [][]byte
	IDs       [][]byte
	Timestamp time.Time
}

type PersistentPendingTxs struct {
	store datastore.Batching
	list  []TxsWithTimestamp
}

func NewPersistentPendingTxs(store datastore.Batching) (*PersistentPendingTxs, error) {
	pt := &PersistentPendingTxs{store: store, list: []TxsWithTimestamp{}}
	if err := pt.Load(); err != nil && err != datastore.ErrNotFound {
		return nil, err
	}
	return pt, nil
}

func (pt *PersistentPendingTxs) Push(txs [][]byte, ids [][]byte, timestamp time.Time) error {
	pt.list = append(pt.list, TxsWithTimestamp{Txs: txs, IDs: ids, Timestamp: timestamp})
	return pt.Save()
}

func (pt *PersistentPendingTxs) PopUpToMaxBytes(maxBytes uint64) ([][]byte, [][]byte, uint64, time.Time) {
	var (
		poppedTxs [][]byte
		ids       [][]byte
		totalSize uint64
		timestamp time.Time = time.Now()
	)

	for len(pt.list) > 0 {
		first := pt.list[0]
		timestamp = first.Timestamp
		for i, tx := range first.Txs {
			txSize := uint64(len(tx))
			if totalSize+txSize > maxBytes {
				pt.list[0] = TxsWithTimestamp{
					Txs:       first.Txs[i:],
					IDs:       first.IDs[i:],
					Timestamp: first.Timestamp,
				}
				pt.Save()
				return poppedTxs, ids, totalSize, timestamp
			}
			poppedTxs = append(poppedTxs, tx)
			ids = append(ids, first.IDs[i])
			totalSize += txSize
		}
		pt.list = pt.list[1:]
	}
	pt.Save()
	return poppedTxs, ids, totalSize, timestamp
}

func (pt *PersistentPendingTxs) Save() error {
	data, err := json.Marshal(pt.list)
	if err != nil {
		return err
	}
	return pt.store.Put(context.Background(), datastore.NewKey(dsPendingTxsKey), data)
}

func (pt *PersistentPendingTxs) Load() error {
	data, err := pt.store.Get(context.Background(), datastore.NewKey(dsPendingTxsKey))
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &pt.list)
}

var _ coresequencer.Sequencer = &Sequencer{}

type Sequencer struct {
	logger         log.Logger
	maxHeightDrift uint64
	rollupId       []byte
	DA             coreda.DA
	dalc           coreda.Client
	pendingTxs     *PersistentPendingTxs
	daStartHeight  uint64
	store          datastore.Batching
}

func NewSequencer(
	logger log.Logger,
	da coreda.DA,
	dalc coreda.Client,
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
		DA:             da,
		dalc:           dalc,
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
	if err := s.submitBatchToDA(ctx, *req.Batch); err != nil {
		return nil, err
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
		lastDAHeight = scanned
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
		// fetch the next batch of transactions from DA
		res := s.dalc.Retrieve(ctx, nextDAHeight)
		if res.BaseResult.Code == coreda.StatusError {
			// stop fetching more transactions and return the current batch
			s.logger.Warn("failed to retrieve transactions from DA layer", "error", res.BaseResult.Message)
			break OuterLoop
		}
		if len(res.Data) == 0 { // TODO: some heights may not have rollup blobs, find a better way to handle this
			// stop fetching more transactions and return the current batch
			s.logger.Debug("no transactions to retrieve from DA layer for", "height", nextDAHeight)
			// don't break yet, wait for maxHeightDrift to elapse
		}

		for i, tx := range res.Data {
			txSize := uint64(len(tx))
			if size+txSize >= maxBytes {
				// Push remaining transactions back to the queue
				s.pendingTxs.Push(res.Data[i:], res.BaseResult.IDs[i:], res.BaseResult.Timestamp)
				break OuterLoop
			}
			resp.Batch.Transactions = append(resp.Batch.Transactions, tx)
			resp.BatchData = append(resp.BatchData, res.BaseResult.IDs[i])
			resp.Timestamp = res.BaseResult.Timestamp // update timestamp to the last one
			size += txSize
		}
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
	namespace, err := s.dalc.GetNamespace(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get namespace: %w", err)
	}
	// get the proofs
	proofs, err := s.DA.GetProofs(ctx, req.BatchData, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get proofs: %w", err)
	}

	// verify the proof
	valid, err := s.DA.Validate(ctx, req.BatchData, proofs, namespace)
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
func (s *Sequencer) submitBatchToDA(ctx context.Context, batch coresequencer.Batch) error {
	currentBatch := batch
	submittedAllTxs := false
	var backoff time.Duration
	totalTxCount := len(batch.Transactions)
	submittedTxCount := 0
	attempt := 0

	// Store initial values to be able to reset or compare later
	// initialGasPrice, err := s.dalc.GasPrice(ctx)
	// if err != nil {
	// 	return fmt.Errorf("failed to get initial gas price: %w", err)
	// }
	initialGasPrice := -1.0
	initialMaxBlobSize, err := s.dalc.MaxBlobSize(ctx)
	if err != nil {
		return fmt.Errorf("failed to get initial max blob size: %w", err)
	}
	maxBlobSize := initialMaxBlobSize
	gasPrice := initialGasPrice

daSubmitRetryLoop:
	for !submittedAllTxs && attempt < maxSubmitAttempts {
		// Wait for backoff duration or exit if context is done
		select {
		case <-ctx.Done():
			break daSubmitRetryLoop
		case <-time.After(backoff):
		}

		// Attempt to submit the batch to the DA layer
		res := s.dalc.Submit(ctx, currentBatch.Transactions, maxBlobSize, gasPrice)

		// gasMultiplier, err := s.dalc.GasMultiplier(ctx)
		// if err != nil {
		// 	return fmt.Errorf("failed to get gas multiplier: %w", err)
		// }
		gasMultiplier := 1.1

		switch res.Code {
		case coreda.StatusSuccess:
			// Count submitted transactions for this attempt
			submittedTxs := int(res.SubmittedCount)
			s.logger.Info("[based] successfully submitted transactions to DA layer",
				"gasPrice", gasPrice,
				"height", res.Height,
				"submittedTxs", submittedTxs,
				"remainingTxs", len(currentBatch.Transactions)-submittedTxs)

			// Update overall progress
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
			maxBlobSize = initialMaxBlobSize

			// Gradually reduce gas price on success, but not below initial price
			if gasMultiplier > 0 && gasPrice != 0 {
				gasPrice = gasPrice / gasMultiplier
				if gasPrice < initialGasPrice {
					gasPrice = initialGasPrice
				}
			}
			s.logger.Debug("resetting DA layer submission options", "backoff", backoff, "gasPrice", gasPrice)

		case coreda.StatusNotIncludedInBlock, coreda.StatusAlreadyInMempool:
			// For mempool-related issues, use a longer backoff and increase gas price
			s.logger.Error("DA layer submission failed", "error", res.Message, "attempt", attempt)
			backoff = batchTime * time.Duration(defaultMempoolTTL)

			// Increase gas price to prioritize the transaction
			if gasMultiplier > 0 && gasPrice != 0 {
				gasPrice = gasPrice * gasMultiplier
			}
			s.logger.Info("retrying DA layer submission with", "backoff", backoff, "gasPrice", gasPrice)

		case coreda.StatusTooBig:
			// if the blob size is too big, it means we are trying to consume the entire block on Celestia
			// If the blob is too big, reduce the max blob size
			maxBlobSize = maxBlobSize / 4 // TODO: this should be fetched from the DA layer?
			fallthrough

		default:
			// For other errors, use exponential backoff
			s.logger.Error("DA layer submission failed", "error", res.Message, "attempt", attempt)
			backoff = s.exponentialBackoff(backoff)
		}

		attempt += 1
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
	if backoff > batchTime {
		backoff = batchTime
	}
	return backoff
}
