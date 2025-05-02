package block

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"cosmossdk.io/log"
	ds "github.com/ipfs/go-datastore"
	dsync "github.com/ipfs/go-datastore/sync"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	testmocks "github.com/rollkit/rollkit/test/mocks"
)

func TestReaper_SubmitTxs_Success(t *testing.T) {

	mockExec := testmocks.NewExecutor(t)
	mockSeq := testmocks.NewSequencer(t)
	store := dsync.MutexWrap(ds.NewMapDatastore())
	logger := log.NewNopLogger()
	chainID := "test-chain"
	interval := 100 * time.Millisecond

	reaper := NewReaper(t.Context(), mockExec, mockSeq, chainID, interval, logger, store)

	// Prepare transaction and its hash
	tx := []byte("tx1")

	// Mock interactions for the first SubmitTxs call
	mockExec.On("GetTxs", mock.Anything).Return([][]byte{tx}, nil).Once()
	submitReqMatcher := mock.MatchedBy(func(req coresequencer.SubmitRollupBatchTxsRequest) bool {
		return string(req.RollupId) == chainID && len(req.Batch.Transactions) == 1 && string(req.Batch.Transactions[0]) == string(tx)
	})
	mockSeq.On("SubmitRollupBatchTxs", mock.Anything, submitReqMatcher).Return(&coresequencer.SubmitRollupBatchTxsResponse{}, nil).Once()

	// Run once and ensure transaction is submitted
	reaper.SubmitTxs()
	mockSeq.AssertCalled(t, "SubmitRollupBatchTxs", mock.Anything, submitReqMatcher)

	mockExec.On("GetTxs", mock.Anything).Return([][]byte{tx}, nil).Once()

	// Run again, should not resubmit
	reaper.SubmitTxs()

	// Verify the final state: GetTxs called twice, SubmitRollupBatchTxs called only once
	mockExec.AssertExpectations(t)
	mockSeq.AssertExpectations(t)
}

func TestReaper_SubmitTxs_NoTxs(t *testing.T) {

	mockExec := testmocks.NewExecutor(t)
	mockSeq := testmocks.NewSequencer(t)
	store := dsync.MutexWrap(ds.NewMapDatastore())
	logger := log.NewNopLogger()
	chainID := "test-chain"
	interval := 100 * time.Millisecond

	reaper := NewReaper(t.Context(), mockExec, mockSeq, chainID, interval, logger, store)

	// Mock GetTxs returning no transactions
	mockExec.On("GetTxs", mock.Anything).Return([][]byte{}, nil).Once()

	// Run once and ensure nothing is submitted
	reaper.SubmitTxs()

	// Verify GetTxs was called
	mockExec.AssertExpectations(t)
	mockSeq.AssertNotCalled(t, "SubmitRollupBatchTxs", mock.Anything, mock.Anything)
}

func TestReaper_TxPersistence_AcrossRestarts(t *testing.T) {
	require := require.New(t)

	// Use separate mocks for each instance but share the store
	mockExec1 := testmocks.NewExecutor(t)
	mockSeq1 := testmocks.NewSequencer(t)
	mockExec2 := testmocks.NewExecutor(t)
	mockSeq2 := testmocks.NewSequencer(t)

	store := dsync.MutexWrap(ds.NewMapDatastore())
	logger := log.NewNopLogger()
	chainID := "test-chain"
	interval := 100 * time.Millisecond

	// Prepare transaction and its hash
	tx := []byte("tx-persist")
	txHash := sha256.Sum256(tx)
	txKey := ds.NewKey(hex.EncodeToString(txHash[:]))

	// First reaper instance
	reaper1 := NewReaper(t.Context(), mockExec1, mockSeq1, chainID, interval, logger, store)

	// Mock interactions for the first instance
	mockExec1.On("GetTxs", mock.Anything).Return([][]byte{tx}, nil).Once()
	submitReqMatcher := mock.MatchedBy(func(req coresequencer.SubmitRollupBatchTxsRequest) bool {
		return string(req.RollupId) == chainID && len(req.Batch.Transactions) == 1 && string(req.Batch.Transactions[0]) == string(tx)
	})
	mockSeq1.On("SubmitRollupBatchTxs", mock.Anything, submitReqMatcher).Return(&coresequencer.SubmitRollupBatchTxsResponse{}, nil).Once()

	reaper1.SubmitTxs()

	// Verify the tx was marked as seen in the real store after the first run
	has, err := store.Has(t.Context(), txKey)
	require.NoError(err)
	require.True(has, "Transaction should be marked as seen in the datastore after first submission")

	// Create a new reaper instance simulating a restart
	reaper2 := NewReaper(t.Context(), mockExec2, mockSeq2, chainID, interval, logger, store)

	// Mock interactions for the second instance
	mockExec2.On("GetTxs", mock.Anything).Return([][]byte{tx}, nil).Once()

	// Should not submit it again
	reaper2.SubmitTxs()

	// Verify the final state:
	mockExec1.AssertExpectations(t)
	mockSeq1.AssertExpectations(t)
	mockExec2.AssertExpectations(t)
	mockSeq2.AssertNotCalled(t, "SubmitRollupBatchTxs", mock.Anything, mock.Anything)
}
