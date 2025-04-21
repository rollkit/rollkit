package block

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"cosmossdk.io/log"
	ds "github.com/ipfs/go-datastore"
	dsync "github.com/ipfs/go-datastore/sync"

	execution "github.com/rollkit/rollkit/core/execution"
	sequencer "github.com/rollkit/rollkit/core/sequencer"
)

func TestReaper_SubmitTxs_Success(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec := execution.NewDummyExecutor()
	seq := sequencer.NewDummySequencer()
	store := dsync.MutexWrap(ds.NewMapDatastore())
	logger := log.NewNopLogger()
	chainID := "test-chain"
	interval := 100 * time.Millisecond

	reaper := NewReaper(ctx, exec, seq, chainID, interval, logger, store)

	// Inject a transaction
	tx := []byte("tx1")
	exec.InjectTx(tx)

	// Run once and ensure transaction is submitted
	reaper.SubmitTxs()

	res, err := seq.GetNextBatch(ctx, sequencer.GetNextBatchRequest{RollupId: []byte(chainID)})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if len(res.Batch.Transactions) != 1 {
		t.Fatalf("expected 1 transaction submitted, got %d", len(res.Batch.Transactions))
	}

	// Run again, should not resubmit
	reaper.SubmitTxs()

	res, err = seq.GetNextBatch(ctx, sequencer.GetNextBatchRequest{RollupId: []byte(chainID)})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if len(res.Batch.Transactions) != 1 {
		t.Fatalf("expected still 1 transaction, got %d", len(res.Batch.Transactions))
	}
}

func TestReaper_SubmitTxs_NoTxs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec := execution.NewDummyExecutor()
	seq := sequencer.NewDummySequencer()
	store := dsync.MutexWrap(ds.NewMapDatastore())
	logger := log.NewNopLogger()
	chainID := "test-chain"
	interval := 100 * time.Millisecond

	reaper := NewReaper(ctx, exec, seq, chainID, interval, logger, store)

	// Run once and ensure nothing is submitted
	reaper.SubmitTxs()

	_, err := seq.GetNextBatch(ctx, sequencer.GetNextBatchRequest{RollupId: []byte(chainID)})
	if err == nil {
		t.Fatalf("expected error due to no batch submitted")
	}
}

func TestReaper_SubmitTxs_InvalidChainID(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec := execution.NewDummyExecutor()
	seq := sequencer.NewDummySequencer()
	store := dsync.MutexWrap(ds.NewMapDatastore())
	logger := log.NewNopLogger()
	chainID := "test-chain"
	interval := 100 * time.Millisecond

	reaper := NewReaper(ctx, exec, seq, chainID, interval, logger, store)

	// Inject a transaction
	exec.InjectTx([]byte("tx1"))

	// Run with correct ID to store the tx
	reaper.SubmitTxs()

	_, err := seq.GetNextBatch(ctx, sequencer.GetNextBatchRequest{RollupId: []byte("wrong-id")})
	if err == nil {
		t.Fatal("expected error due to invalid rollup ID")
	}
}

func TestReaper_TxPersistence_AcrossRestarts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec := execution.NewDummyExecutor()
	seq := sequencer.NewDummySequencer()
	store := dsync.MutexWrap(ds.NewMapDatastore())
	logger := log.NewNopLogger()
	chainID := "test-chain"
	interval := 100 * time.Millisecond

	// First reaper instance
	reaper := NewReaper(ctx, exec, seq, chainID, interval, logger, store)
	tx := []byte("tx-persist")
	exec.InjectTx(tx)
	reaper.SubmitTxs()

	// Create a new reaper instance simulating a restart
	reaper2 := NewReaper(ctx, exec, seq, chainID, interval, logger, store)
	exec.InjectTx(tx) // re-inject the same tx

	// Should not submit it again
	reaper2.SubmitTxs()

	txHash := sha256.Sum256(tx)
	key := ds.NewKey(hex.EncodeToString(txHash[:]))
	has, err := store.Has(ctx, key)
	if err != nil || !has {
		t.Fatalf("expected tx to be marked as seen, err=%v", err)
	}
}
