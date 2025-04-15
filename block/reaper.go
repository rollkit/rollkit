package block

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"cosmossdk.io/log"
	ds "github.com/ipfs/go-datastore"
	"github.com/rollkit/go-sequencing"
	coreexecutor "github.com/rollkit/rollkit/core/execution"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
)

type Reaper struct {
	exec      coreexecutor.Executor
	sequencer coresequencer.Sequencer
	chainID   string
	interval  time.Duration
	logger    log.Logger
	ctx       context.Context
	seenStore ds.Batching
}

// NewReaper creates a new Reaper instance with persistent seenTx storage.
func NewReaper(ctx context.Context, exec coreexecutor.Executor, sequencer coresequencer.Sequencer, chainID string, interval time.Duration, logger log.Logger, store ds.Batching) *Reaper {
	return &Reaper{
		exec:      exec,
		sequencer: sequencer,
		chainID:   chainID,
		interval:  interval,
		logger:    logger,
		ctx:       ctx,
		seenStore: store,
	}
}

// Start begins the reaping process at the specified interval.
func (r *Reaper) Start() {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	r.logger.Info("Reaper started", "interval", r.interval)

	for {
		select {
		case <-r.ctx.Done():
			r.logger.Info("Reaper stopped")
			return
		case <-ticker.C:
			r.SubmitTxs()
		}
	}
}

// SubmitTxs retrieves transactions from the executor and submits them to the sequencer.
func (r *Reaper) SubmitTxs() {
	txs, err := r.exec.GetTxs(r.ctx)
	if err != nil {
		r.logger.Error("Reaper failed to get txs from executor", "error", err)
		return
	}

	var newTxs [][]byte
	for _, tx := range txs {
		txHash := hashTx(tx)
		key := ds.NewKey(txHash)
		has, err := r.seenStore.Has(r.ctx, key)
		if err != nil {
			r.logger.Error("Failed to check seenStore", "error", err)
			continue
		}
		if !has {
			newTxs = append(newTxs, tx)
		}
	}

	if len(newTxs) == 0 {
		r.logger.Debug("Reaper found no new txs to submit")
		return
	}

	r.logger.Debug("Reaper submitting txs to sequencer", "txCount", len(newTxs))

	_, err = r.sequencer.SubmitRollupBatchTxs(r.ctx, coresequencer.SubmitRollupBatchTxsRequest{
		RollupId: sequencing.RollupId(r.chainID),
		Batch:    &coresequencer.Batch{Transactions: newTxs},
	})
	if err != nil {
		r.logger.Error("Reaper failed to submit txs to sequencer", "error", err)
		return
	}

	for _, tx := range newTxs {
		txHash := hashTx(tx)
		key := ds.NewKey(txHash)
		if err := r.seenStore.Put(r.ctx, key, []byte{1}); err != nil {
			r.logger.Error("Failed to persist seen tx", "txHash", txHash, "error", err)
		}
	}

	r.logger.Debug("Reaper successfully submitted txs")
}

func hashTx(tx []byte) string {
	hash := sha256.Sum256(tx)
	return hex.EncodeToString(hash[:])
}
