package single

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"cosmossdk.io/log"
	ds "github.com/ipfs/go-datastore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	coreda "github.com/rollkit/rollkit/core/da"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	damocks "github.com/rollkit/rollkit/test/mocks"
)

func TestNewSequencer(t *testing.T) {
	// Create a new sequencer with mock DA client
	dummyDA := coreda.NewDummyDA(100_000_000, 0, 0)
	metrics, _ := NopMetrics()
	db := ds.NewMapDatastore()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	seq, err := NewSequencer(ctx, log.NewNopLogger(), db, dummyDA, []byte("namespace"), []byte("rollup1"), 10*time.Second, metrics, false)
	if err != nil {
		t.Fatalf("Failed to create sequencer: %v", err)
	}
	defer func() {
		err := db.Close()
		if err != nil {
			t.Fatalf("Failed to close sequencer: %v", err)
		}
	}()

	// Check if the sequencer was created with the correct values
	if seq == nil {
		t.Fatal("Expected sequencer to not be nil")
	}

	if seq.queue == nil {
		t.Fatal("Expected batch queue to not be nil")
	}
	if seq.dalc == nil {
		t.Fatal("Expected DA client to not be nil")
	}
}

func TestSequencer_SubmitRollupBatchTxs(t *testing.T) {
	// Initialize a new sequencer
	metrics, _ := NopMetrics()
	dummyDA := coreda.NewDummyDA(100_000_000, 0, 0)
	db := ds.NewMapDatastore()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	rollupId := []byte("rollup1")
	seq, err := NewSequencer(ctx, log.NewNopLogger(), db, dummyDA, []byte("namespace"), rollupId, 10*time.Second, metrics, false)
	if err != nil {
		t.Fatalf("Failed to create sequencer: %v", err)
	}
	defer func() {
		err := db.Close()
		if err != nil {
			t.Fatalf("Failed to close sequencer: %v", err)
		}
	}()

	// Test with initial rollup ID
	tx := []byte("transaction1")

	res, err := seq.SubmitRollupBatchTxs(context.Background(), coresequencer.SubmitRollupBatchTxsRequest{RollupId: rollupId, Batch: &coresequencer.Batch{Transactions: [][]byte{tx}}})
	if err != nil {
		t.Fatalf("Failed to submit rollup transaction: %v", err)
	}
	if res == nil {
		t.Fatal("Expected response to not be nil")
	}

	// Verify the transaction was added
	nextBatchresp, err := seq.GetNextBatch(context.Background(), coresequencer.GetNextBatchRequest{RollupId: rollupId})
	if err != nil {
		t.Fatalf("Failed to get next batch: %v", err)
	}
	if len(nextBatchresp.Batch.Transactions) != 1 {
		t.Fatalf("Expected 1 transaction, got %d", len(nextBatchresp.Batch.Transactions))
	}

	// Test with a different rollup ID (expecting an error due to mismatch)
	res, err = seq.SubmitRollupBatchTxs(context.Background(), coresequencer.SubmitRollupBatchTxsRequest{RollupId: []byte("rollup2"), Batch: &coresequencer.Batch{Transactions: [][]byte{tx}}})
	if err == nil {
		t.Fatal("Expected error for invalid rollup ID, got nil")
	}
	if !errors.Is(err, ErrInvalidRollupId) {
		t.Fatalf("Expected ErrInvalidRollupId, got %v", err)
	}
	if res != nil {
		t.Fatal("Expected nil response for error case")
	}
}

func TestSequencer_SubmitRollupBatchTxs_EmptyBatch(t *testing.T) {
	// Initialize a new sequencer
	metrics, _ := NopMetrics()
	dummyDA := coreda.NewDummyDA(100_000_000, 0, 0)
	db := ds.NewMapDatastore()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	rollupId := []byte("rollup1")
	seq, err := NewSequencer(ctx, log.NewNopLogger(), db, dummyDA, []byte("namespace"), rollupId, 10*time.Second, metrics, false)
	require.NoError(t, err, "Failed to create sequencer")
	defer func() {
		err := db.Close()
		require.NoError(t, err, "Failed to close sequencer")
	}()

	// Test submitting an empty batch
	res, err := seq.SubmitRollupBatchTxs(context.Background(), coresequencer.SubmitRollupBatchTxsRequest{
		RollupId: rollupId,
		Batch:    &coresequencer.Batch{Transactions: [][]byte{}}, // Empty transactions
	})
	require.NoError(t, err, "Submitting empty batch should not return an error")
	require.NotNil(t, res, "Response should not be nil for empty batch submission")

	// Verify that no batch was added to the queue
	nextBatchResp, err := seq.GetNextBatch(context.Background(), coresequencer.GetNextBatchRequest{RollupId: rollupId})
	require.NoError(t, err, "Getting next batch after empty submission failed")
	require.NotNil(t, nextBatchResp, "GetNextBatch response should not be nil")
	require.Empty(t, nextBatchResp.Batch.Transactions, "Queue should be empty after submitting an empty batch")

	// Test submitting a nil batch
	res, err = seq.SubmitRollupBatchTxs(context.Background(), coresequencer.SubmitRollupBatchTxsRequest{
		RollupId: rollupId,
		Batch:    nil, // Nil batch
	})
	require.NoError(t, err, "Submitting nil batch should not return an error")
	require.NotNil(t, res, "Response should not be nil for nil batch submission")

	// Verify again that no batch was added to the queue
	nextBatchResp, err = seq.GetNextBatch(context.Background(), coresequencer.GetNextBatchRequest{RollupId: rollupId})
	require.NoError(t, err, "Getting next batch after nil submission failed")
	require.NotNil(t, nextBatchResp, "GetNextBatch response should not be nil")
	require.Empty(t, nextBatchResp.Batch.Transactions, "Queue should still be empty after submitting a nil batch")
}

func TestSequencer_GetNextBatch_NoLastBatch(t *testing.T) {
	db := ds.NewMapDatastore()

	seq := &Sequencer{
		queue:    NewBatchQueue(db, "batches"),
		rollupId: []byte("rollup"),
	}
	defer func() {
		err := db.Close()
		if err != nil {
			t.Fatalf("Failed to close sequencer: %v", err)
		}
	}()

	// Test case where lastBatchHash and seq.lastBatchHash are both nil
	res, err := seq.GetNextBatch(context.Background(), coresequencer.GetNextBatchRequest{RollupId: seq.rollupId})
	if err != nil {
		t.Fatalf("Failed to get next batch: %v", err)
	}

	// Ensure the time is approximately the same
	if res.Timestamp.Day() != time.Now().Day() {
		t.Fatalf("Expected timestamp day to be %d, got %d", time.Now().Day(), res.Timestamp.Day())
	}

	// Should return an empty batch
	if len(res.Batch.Transactions) != 0 {
		t.Fatalf("Expected empty batch, got %d transactions", len(res.Batch.Transactions))
	}
}

func TestSequencer_GetNextBatch_Success(t *testing.T) {
	// Initialize a new sequencer with a mock batch
	mockBatch := &coresequencer.Batch{Transactions: [][]byte{[]byte("tx1"), []byte("tx2")}}

	db := ds.NewMapDatastore()

	seq := &Sequencer{
		logger:           log.NewNopLogger(),
		queue:            NewBatchQueue(db, "batches"),
		daSubmissionChan: make(chan coresequencer.Batch, 100),
		rollupId:         []byte("rollup"),
	}
	defer func() {
		err := db.Close()
		if err != nil {
			t.Fatalf("Failed to close sequencer: %v", err)
		}
	}()

	// Add mock batch to the BatchQueue
	err := seq.queue.AddBatch(context.Background(), *mockBatch)
	if err != nil {
		t.Fatalf("Failed to add batch: %v", err)
	}

	// Test success case with no previous lastBatchHash
	res, err := seq.GetNextBatch(context.Background(), coresequencer.GetNextBatchRequest{RollupId: seq.rollupId})
	if err != nil {
		t.Fatalf("Failed to get next batch: %v", err)
	}

	// Ensure the time is approximately the same
	if res.Timestamp.Day() != time.Now().Day() {
		t.Fatalf("Expected timestamp day to be %d, got %d", time.Now().Day(), res.Timestamp.Day())
	}

	fmt.Println("res.Batch.Transactions", res.Batch)
	// Ensure that the transactions are present
	if len(res.Batch.Transactions) != 2 {
		t.Fatalf("Expected 2 transactions, got %d", len(res.Batch.Transactions))
	}

	batchHash, err := mockBatch.Hash()
	if err != nil {
		t.Fatalf("Failed to get batch hash: %v", err)
	}
	if len(batchHash) == 0 {
		t.Fatal("Expected batch hash to not be empty")
	}
}

func TestSequencer_VerifyBatch(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	db := ds.NewMapDatastore()
	defer func() {
		err := db.Close()
		require.NoError(err, "Failed to close datastore")
	}()

	rollupId := []byte("test-rollup")
	namespace := []byte("test-namespace")
	batchData := [][]byte{[]byte("batch1"), []byte("batch2")}
	proofs := [][]byte{[]byte("proof1"), []byte("proof2")}

	t.Run("Proposer Mode", func(t *testing.T) {
		mockDA := damocks.NewDA(t)
		mockDALC := coreda.NewDummyClient(mockDA, namespace)

		seq := &Sequencer{
			logger:           log.NewNopLogger(),
			rollupId:         rollupId,
			proposer:         true,
			dalc:             mockDALC,
			da:               mockDA,
			queue:            NewBatchQueue(db, "proposer_queue"),
			daSubmissionChan: make(chan coresequencer.Batch, 1),
		}

		res, err := seq.VerifyBatch(context.Background(), coresequencer.VerifyBatchRequest{RollupId: seq.rollupId, BatchData: batchData})
		assert.NoError(err)
		assert.NotNil(res)
		assert.True(res.Status, "Expected status to be true in proposer mode")

		mockDA.AssertNotCalled(t, "GetProofs", mock.Anything, mock.Anything, mock.Anything)
		mockDA.AssertNotCalled(t, "Validate", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("Non-Proposer Mode", func(t *testing.T) {
		t.Run("Valid Proofs", func(t *testing.T) {
			mockDA := damocks.NewDA(t)
			mockDALC := coreda.NewDummyClient(mockDA, namespace)
			seq := &Sequencer{
				logger:           log.NewNopLogger(),
				rollupId:         rollupId,
				proposer:         false,
				dalc:             mockDALC,
				da:               mockDA,
				queue:            NewBatchQueue(db, "valid_proofs_queue"),
				daSubmissionChan: make(chan coresequencer.Batch, 1),
			}

			mockDA.On("GetProofs", mock.Anything, batchData, namespace).Return(proofs, nil).Once()
			mockDA.On("Validate", mock.Anything, batchData, proofs, namespace).Return([]bool{true, true}, nil).Once()

			res, err := seq.VerifyBatch(context.Background(), coresequencer.VerifyBatchRequest{RollupId: seq.rollupId, BatchData: batchData})
			assert.NoError(err)
			assert.NotNil(res)
			assert.True(res.Status, "Expected status to be true for valid proofs")
			mockDA.AssertExpectations(t)
		})

		t.Run("Invalid Proof", func(t *testing.T) {
			mockDA := damocks.NewDA(t)
			mockDALC := coreda.NewDummyClient(mockDA, namespace)
			seq := &Sequencer{
				logger:           log.NewNopLogger(),
				rollupId:         rollupId,
				proposer:         false,
				dalc:             mockDALC,
				da:               mockDA,
				queue:            NewBatchQueue(db, "invalid_proof_queue"),
				daSubmissionChan: make(chan coresequencer.Batch, 1),
			}

			mockDA.On("GetProofs", mock.Anything, batchData, namespace).Return(proofs, nil).Once()
			mockDA.On("Validate", mock.Anything, batchData, proofs, namespace).Return([]bool{true, false}, nil).Once()

			res, err := seq.VerifyBatch(context.Background(), coresequencer.VerifyBatchRequest{RollupId: seq.rollupId, BatchData: batchData})
			assert.NoError(err)
			assert.NotNil(res)
			assert.False(res.Status, "Expected status to be false for invalid proof")
			mockDA.AssertExpectations(t)
		})

		t.Run("GetProofs Error", func(t *testing.T) {
			mockDA := damocks.NewDA(t)
			mockDALC := coreda.NewDummyClient(mockDA, namespace)
			seq := &Sequencer{
				logger:           log.NewNopLogger(),
				rollupId:         rollupId,
				proposer:         false,
				dalc:             mockDALC,
				da:               mockDA,
				queue:            NewBatchQueue(db, "getproofs_err_queue"),
				daSubmissionChan: make(chan coresequencer.Batch, 1),
			}
			expectedErr := errors.New("get proofs failed")

			mockDA.On("GetProofs", mock.Anything, batchData, namespace).Return(nil, expectedErr).Once()

			res, err := seq.VerifyBatch(context.Background(), coresequencer.VerifyBatchRequest{RollupId: seq.rollupId, BatchData: batchData})
			assert.Error(err)
			assert.Nil(res)
			assert.Contains(err.Error(), expectedErr.Error())
			mockDA.AssertExpectations(t)
			mockDA.AssertNotCalled(t, "Validate", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})

		t.Run("Validate Error", func(t *testing.T) {
			mockDA := damocks.NewDA(t)
			mockDALC := coreda.NewDummyClient(mockDA, namespace)
			seq := &Sequencer{
				logger:           log.NewNopLogger(),
				rollupId:         rollupId,
				proposer:         false,
				dalc:             mockDALC,
				da:               mockDA,
				queue:            NewBatchQueue(db, "validate_err_queue"),
				daSubmissionChan: make(chan coresequencer.Batch, 1),
			}
			expectedErr := errors.New("validate failed")

			mockDA.On("GetProofs", mock.Anything, batchData, namespace).Return(proofs, nil).Once()
			mockDA.On("Validate", mock.Anything, batchData, proofs, namespace).Return(nil, expectedErr).Once()

			res, err := seq.VerifyBatch(context.Background(), coresequencer.VerifyBatchRequest{RollupId: seq.rollupId, BatchData: batchData})
			assert.Error(err)
			assert.Nil(res)
			assert.Contains(err.Error(), expectedErr.Error())
			mockDA.AssertExpectations(t)
		})

		t.Run("Invalid Rollup ID", func(t *testing.T) {
			mockDA := damocks.NewDA(t)
			mockDALC := coreda.NewDummyClient(mockDA, namespace)
			seq := &Sequencer{
				logger:           log.NewNopLogger(),
				rollupId:         rollupId,
				proposer:         false,
				dalc:             mockDALC,
				da:               mockDA,
				queue:            NewBatchQueue(db, "invalid_rollup_queue"),
				daSubmissionChan: make(chan coresequencer.Batch, 1),
			}

			invalidRollupId := []byte("invalidRollup")
			res, err := seq.VerifyBatch(context.Background(), coresequencer.VerifyBatchRequest{RollupId: invalidRollupId, BatchData: batchData})
			assert.Error(err)
			assert.Nil(res)
			assert.ErrorIs(err, ErrInvalidRollupId)

			mockDA.AssertNotCalled(t, "GetProofs", mock.Anything, mock.Anything, mock.Anything)
			mockDA.AssertNotCalled(t, "Validate", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	})
}

func TestSequencer_GetNextBatch_BeforeDASubmission(t *testing.T) {
	// Initialize a new sequencer with mock DA
	metrics, _ := NopMetrics()
	mockDA := &damocks.DA{}
	db := ds.NewMapDatastore()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	seq, err := NewSequencer(ctx, log.NewNopLogger(), db, mockDA, []byte("namespace"), []byte("rollup1"), 1*time.Second, metrics, false)
	if err != nil {
		t.Fatalf("Failed to create sequencer: %v", err)
	}
	defer func() {
		err := db.Close()
		if err != nil {
			t.Fatalf("Failed to close sequencer: %v", err)
		}
	}()

	// Set up mock expectations
	mockDA.On("MaxBlobSize", mock.Anything).Return(uint64(100_000_000), nil)
	mockDA.On("GasPrice", mock.Anything).Return(float64(0), nil)
	mockDA.On("GasMultiplier", mock.Anything).Return(float64(0), nil)
	mockDA.On("SubmitWithOptions", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("mock DA always rejects submissions"))

	// Submit a batch
	rollupId := []byte("rollup1")
	tx := []byte("transaction1")
	res, err := seq.SubmitRollupBatchTxs(context.Background(), coresequencer.SubmitRollupBatchTxsRequest{
		RollupId: rollupId,
		Batch:    &coresequencer.Batch{Transactions: [][]byte{tx}},
	})
	if err != nil {
		t.Fatalf("Failed to submit rollup transaction: %v", err)
	}
	if res == nil {
		t.Fatal("Expected response to not be nil")
	}
	time.Sleep(100 * time.Millisecond)

	// Try to get the batch before DA submission
	nextBatchResp, err := seq.GetNextBatch(context.Background(), coresequencer.GetNextBatchRequest{RollupId: rollupId})
	if err != nil {
		t.Fatalf("Failed to get next batch: %v", err)
	}
	if len(nextBatchResp.Batch.Transactions) != 1 {
		t.Fatalf("Expected 1 transaction, got %d", len(nextBatchResp.Batch.Transactions))
	}
	if !bytes.Equal(nextBatchResp.Batch.Transactions[0], tx) {
		t.Fatal("Expected transaction to match submitted transaction")
	}

	// Verify all mock expectations were met
	mockDA.AssertExpectations(t)
}
