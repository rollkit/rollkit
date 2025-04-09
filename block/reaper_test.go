package block

import (
	"context"
	"testing"
	"time"

	"cosmossdk.io/log"
	execution "github.com/rollkit/rollkit/core/execution"
	sequencer "github.com/rollkit/rollkit/core/sequencer"
)

func TestReaper_SubmitTxs_Success(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec := execution.NewDummyExecutor()
	seq := sequencer.NewDummySequencer()
	logger := log.NewNopLogger()
	chainID := "test-chain"
	interval := 100 * time.Millisecond

	reaper := NewReaper(ctx, exec, seq, chainID, interval, logger)

	// Inject a transaction
	exec.InjectTx([]byte("tx1"))

	// Run once and ensure transaction is submitted
	reaper.SubmitTxs()

	res, err := seq.GetNextBatch(ctx, sequencer.GetNextBatchRequest{RollupId: []byte(chainID)})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if len(res.Batch.Transactions) != 1 {
		t.Fatalf("expected 1 transaction submitted, got %d", len(res.Batch.Transactions))
	}
}

func TestReaper_SubmitTxs_NoTxs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec := execution.NewDummyExecutor()
	seq := sequencer.NewDummySequencer()
	logger := log.NewNopLogger()
	chainID := "test-chain"
	interval := 100 * time.Millisecond

	reaper := NewReaper(ctx, exec, seq, chainID, interval, logger)

	// Run once and ensure nothing is submitted
	reaper.SubmitTxs()

	res, err := seq.GetNextBatch(ctx, sequencer.GetNextBatchRequest{RollupId: []byte(chainID)})
	if err == nil {
		t.Fatalf("expected error due to no batch submitted, got batch: %+v", res)
	}
}

func TestReaper_SubmitTxs_InvalidChainID(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec := execution.NewDummyExecutor()
	seq := sequencer.NewDummySequencer()
	logger := log.NewNopLogger()
	chainID := "test-chain"
	interval := 100 * time.Millisecond

	reaper := NewReaper(ctx, exec, seq, chainID, interval, logger)

	// Inject a transaction
	exec.InjectTx([]byte("tx1"))

	// Override chain ID to invalid one (different in GetNextBatch call)
	reaper.SubmitTxs()

	_, err := seq.GetNextBatch(ctx, sequencer.GetNextBatchRequest{RollupId: []byte("different-chain")})
	if err == nil {
		t.Fatal("expected error due to invalid rollup ID")
	}
}
