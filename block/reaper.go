package block

import (
	"context"
	"time"

	"cosmossdk.io/log"
	"github.com/rollkit/go-sequencing"
	coreexecutor "github.com/rollkit/rollkit/core/execution"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
)

// Reaper is responsible for periodically pulling transactions from the executor
// and submitting them to the sequencer.
type Reaper struct {
	exec      coreexecutor.Executor
	sequencer coresequencer.Sequencer
	chainID   string
	interval  time.Duration
	logger    log.Logger
	ctx       context.Context
}

// NewReaper creates a new Reaper instance.
func NewReaper(ctx context.Context, exec coreexecutor.Executor, sequencer coresequencer.Sequencer, chainID string, interval time.Duration, logger log.Logger) *Reaper {
	return &Reaper{
		exec:      exec,
		sequencer: sequencer,
		chainID:   chainID,
		interval:  interval,
		logger:    logger,
		ctx:       ctx,
	}
}

// Start runs the reaper loop which pulls and submits transactions at a fixed interval.
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

// SubmitTxs pulls transactions from the executor and submits them to the sequencer.
func (r *Reaper) SubmitTxs() {
	txs, err := r.exec.GetTxs(r.ctx)
	if err != nil {
		r.logger.Error("Reaper failed to get txs from executor", "error", err)
		return
	}

	if len(txs) == 0 {
		r.logger.Debug("Reaper found no txs to submit")
		return
	}

	r.logger.Debug("Reaper submitting txs to sequencer", "txCount", len(txs))

	_, err = r.sequencer.SubmitRollupBatchTxs(r.ctx, coresequencer.SubmitRollupBatchTxsRequest{
		RollupId: sequencing.RollupId(r.chainID),
		Batch:    &coresequencer.Batch{Transactions: txs},
	})
	if err != nil {
		r.logger.Error("Reaper failed to submit txs to sequencer", "error", err)
		return
	}

	r.logger.Debug("Reaper successfully submitted txs")
}
