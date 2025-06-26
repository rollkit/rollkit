package single

import (
	"bytes"
	"context"
	"errors"
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
	dummyDA := coreda.NewDummyDA(100_000_000, 0, 0, 10*time.Second)
	metrics, _ := NopMetrics()
	db := ds.NewMapDatastore()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	seq, err := NewSequencer(ctx, log.NewNopLogger(), db, dummyDA, []byte("test1"), 10*time.Second, metrics, false)
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
	if seq.da == nil {
		t.Fatal("Expected DA client to not be nil")
	}
}

func TestSequencer_SubmitBatchTxs(t *testing.T) {
	// Initialize a new sequencer
	metrics, _ := NopMetrics()
	dummyDA := coreda.NewDummyDA(100_000_000, 0, 0, 10*time.Second)
	db := ds.NewMapDatastore()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	Id := []byte("test1")
	seq, err := NewSequencer(ctx, log.NewNopLogger(), db, dummyDA, Id, 10*time.Second, metrics, false)
	if err != nil {
		t.Fatalf("Failed to create sequencer: %v", err)
	}
	defer func() {
		err := db.Close()
		if err != nil {
			t.Fatalf("Failed to close sequencer: %v", err)
		}
	}()

	// Test with initial ID
	tx := []byte("transaction1")

	res, err := seq.SubmitBatchTxs(context.Background(), coresequencer.SubmitBatchTxsRequest{Id: Id, Batch: &coresequencer.Batch{Transactions: [][]byte{tx}}})
	if err != nil {
		t.Fatalf("Failed to submit  transaction: %v", err)
	}
	if res == nil {
		t.Fatal("Expected response to not be nil")
	}

	// Verify the transaction was added
	nextBatchresp, err := seq.GetNextBatch(context.Background(), coresequencer.GetNextBatchRequest{Id: Id})
	if err != nil {
		t.Fatalf("Failed to get next batch: %v", err)
	}
	if len(nextBatchresp.Batch.Transactions) != 1 {
		t.Fatalf("Expected 1 transaction, got %d", len(nextBatchresp.Batch.Transactions))
	}

	// Test with a different  ID (expecting an error due to mismatch)
	res, err = seq.SubmitBatchTxs(context.Background(), coresequencer.SubmitBatchTxsRequest{Id: []byte("test2"), Batch: &coresequencer.Batch{Transactions: [][]byte{tx}}})
	if err == nil {
		t.Fatal("Expected error for invalid  ID, got nil")
	}
	if !errors.Is(err, ErrInvalidId) {
		t.Fatalf("Expected ErrInvalidId, got %v", err)
	}
	if res != nil {
		t.Fatal("Expected nil response for error case")
	}
}

func TestSequencer_SubmitBatchTxs_EmptyBatch(t *testing.T) {
	// Initialize a new sequencer
	metrics, _ := NopMetrics()
	dummyDA := coreda.NewDummyDA(100_000_000, 0, 0, 10*time.Second)
	db := ds.NewMapDatastore()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	Id := []byte("test1")
	seq, err := NewSequencer(ctx, log.NewNopLogger(), db, dummyDA, Id, 10*time.Second, metrics, false)
	require.NoError(t, err, "Failed to create sequencer")
	defer func() {
		err := db.Close()
		require.NoError(t, err, "Failed to close sequencer")
	}()

	// Test submitting an empty batch
	res, err := seq.SubmitBatchTxs(context.Background(), coresequencer.SubmitBatchTxsRequest{
		Id:    Id,
		Batch: &coresequencer.Batch{Transactions: [][]byte{}}, // Empty transactions
	})
	require.NoError(t, err, "Submitting empty batch should not return an error")
	require.NotNil(t, res, "Response should not be nil for empty batch submission")

	// Verify that no batch was added to the queue
	nextBatchResp, err := seq.GetNextBatch(context.Background(), coresequencer.GetNextBatchRequest{Id: Id})
	require.NoError(t, err, "Getting next batch after empty submission failed")
	require.NotNil(t, nextBatchResp, "GetNextBatch response should not be nil")
	require.Empty(t, nextBatchResp.Batch.Transactions, "Queue should be empty after submitting an empty batch")

	// Test submitting a nil batch
	res, err = seq.SubmitBatchTxs(context.Background(), coresequencer.SubmitBatchTxsRequest{
		Id:    Id,
		Batch: nil, // Nil batch
	})
	require.NoError(t, err, "Submitting nil batch should not return an error")
	require.NotNil(t, res, "Response should not be nil for nil batch submission")

	// Verify again that no batch was added to the queue
	nextBatchResp, err = seq.GetNextBatch(context.Background(), coresequencer.GetNextBatchRequest{Id: Id})
	require.NoError(t, err, "Getting next batch after nil submission failed")
	require.NotNil(t, nextBatchResp, "GetNextBatch response should not be nil")
	require.Empty(t, nextBatchResp.Batch.Transactions, "Queue should still be empty after submitting a nil batch")
}

func TestSequencer_GetNextBatch_NoLastBatch(t *testing.T) {
	db := ds.NewMapDatastore()

	seq := &Sequencer{
		queue: NewBatchQueue(db, "batches"),
		Id:    []byte("test"),
	}
	defer func() {
		err := db.Close()
		if err != nil {
			t.Fatalf("Failed to close sequencer: %v", err)
		}
	}()

	// Test case where lastBatchHash and seq.lastBatchHash are both nil
	res, err := seq.GetNextBatch(context.Background(), coresequencer.GetNextBatchRequest{Id: seq.Id})
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
		logger: log.NewNopLogger(),
		queue:  NewBatchQueue(db, "batches"),
		Id:     []byte("test"),
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
	res, err := seq.GetNextBatch(context.Background(), coresequencer.GetNextBatchRequest{Id: seq.Id})
	if err != nil {
		t.Fatalf("Failed to get next batch: %v", err)
	}

	// Ensure the time is approximately the same
	if res.Timestamp.Day() != time.Now().Day() {
		t.Fatalf("Expected timestamp day to be %d, got %d", time.Now().Day(), res.Timestamp.Day())
	}

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

	Id := []byte("test")
	batchData := [][]byte{[]byte("batch1"), []byte("batch2")}
	proofs := [][]byte{[]byte("proof1"), []byte("proof2")}

	t.Run("Proposer Mode", func(t *testing.T) {
		mockDA := damocks.NewMockDA(t)

		seq := &Sequencer{
			logger:   log.NewNopLogger(),
			Id:       Id,
			proposer: true,
			da:       mockDA,
			queue:    NewBatchQueue(db, "proposer_queue"),
		}

		res, err := seq.VerifyBatch(context.Background(), coresequencer.VerifyBatchRequest{Id: seq.Id, BatchData: batchData})
		assert.NoError(err)
		assert.NotNil(res)
		assert.True(res.Status, "Expected status to be true in proposer mode")

		mockDA.AssertNotCalled(t, "GetProofs", context.Background(), mock.Anything)
		mockDA.AssertNotCalled(t, "Validate", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("Non-Proposer Mode", func(t *testing.T) {
		t.Run("Valid Proofs", func(t *testing.T) {
			mockDA := damocks.NewMockDA(t)
			seq := &Sequencer{
				logger:   log.NewNopLogger(),
				Id:       Id,
				proposer: false,
				da:       mockDA,
				queue:    NewBatchQueue(db, "valid_proofs_queue"),
			}

			mockDA.On("GetProofs", context.Background(), batchData, Id).Return(proofs, nil).Once()
			mockDA.On("Validate", mock.Anything, batchData, proofs, Id).Return([]bool{true, true}, nil).Once()

			res, err := seq.VerifyBatch(context.Background(), coresequencer.VerifyBatchRequest{Id: seq.Id, BatchData: batchData})
			assert.NoError(err)
			assert.NotNil(res)
			assert.True(res.Status, "Expected status to be true for valid proofs")
			mockDA.AssertExpectations(t)
		})

		t.Run("Invalid Proof", func(t *testing.T) {
			mockDA := damocks.NewMockDA(t)
			seq := &Sequencer{
				logger:   log.NewNopLogger(),
				Id:       Id,
				proposer: false,
				da:       mockDA,
				queue:    NewBatchQueue(db, "invalid_proof_queue"),
			}

			mockDA.On("GetProofs", context.Background(), batchData, Id).Return(proofs, nil).Once()
			mockDA.On("Validate", mock.Anything, batchData, proofs, Id).Return([]bool{true, false}, nil).Once()

			res, err := seq.VerifyBatch(context.Background(), coresequencer.VerifyBatchRequest{Id: seq.Id, BatchData: batchData})
			assert.NoError(err)
			assert.NotNil(res)
			assert.False(res.Status, "Expected status to be false for invalid proof")
			mockDA.AssertExpectations(t)
		})

		t.Run("GetProofs Error", func(t *testing.T) {
			mockDA := damocks.NewMockDA(t)
			seq := &Sequencer{
				logger:   log.NewNopLogger(),
				Id:       Id,
				proposer: false,
				da:       mockDA,
				queue:    NewBatchQueue(db, "getproofs_err_queue"),
			}
			expectedErr := errors.New("get proofs failed")

			mockDA.On("GetProofs", context.Background(), batchData, Id).Return(nil, expectedErr).Once()

			res, err := seq.VerifyBatch(context.Background(), coresequencer.VerifyBatchRequest{Id: seq.Id, BatchData: batchData})
			assert.Error(err)
			assert.Nil(res)
			assert.Contains(err.Error(), expectedErr.Error())
			mockDA.AssertExpectations(t)
			mockDA.AssertNotCalled(t, "Validate", mock.Anything, mock.Anything, mock.Anything)
		})

		t.Run("Validate Error", func(t *testing.T) {
			mockDA := damocks.NewMockDA(t)
			seq := &Sequencer{
				logger:   log.NewNopLogger(),
				Id:       Id,
				proposer: false,
				da:       mockDA,
				queue:    NewBatchQueue(db, "validate_err_queue"),
			}
			expectedErr := errors.New("validate failed")

			mockDA.On("GetProofs", context.Background(), batchData, Id).Return(proofs, nil).Once()
			mockDA.On("Validate", mock.Anything, batchData, proofs, Id).Return(nil, expectedErr).Once()

			res, err := seq.VerifyBatch(context.Background(), coresequencer.VerifyBatchRequest{Id: seq.Id, BatchData: batchData})
			assert.Error(err)
			assert.Nil(res)
			assert.Contains(err.Error(), expectedErr.Error())
			mockDA.AssertExpectations(t)
		})

		t.Run("Invalid ID", func(t *testing.T) {
			mockDA := damocks.NewMockDA(t)

			seq := &Sequencer{
				logger:   log.NewNopLogger(),
				Id:       Id,
				proposer: false,
				da:       mockDA,
				queue:    NewBatchQueue(db, "invalid_queue"),
			}

			invalidId := []byte("invalid")
			res, err := seq.VerifyBatch(context.Background(), coresequencer.VerifyBatchRequest{Id: invalidId, BatchData: batchData})
			assert.Error(err)
			assert.Nil(res)
			assert.ErrorIs(err, ErrInvalidId)

			mockDA.AssertNotCalled(t, "GetProofs", context.Background(), mock.Anything)
			mockDA.AssertNotCalled(t, "Validate", mock.Anything, mock.Anything, mock.Anything)
		})
	})
}

func TestSequencer_GetNextBatch_BeforeDASubmission(t *testing.T) {
	t.Skip()
	// Initialize a new sequencer with mock DA
	metrics, _ := NopMetrics()
	mockDA := &damocks.MockDA{}
	db := ds.NewMapDatastore()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	seq, err := NewSequencer(ctx, log.NewNopLogger(), db, mockDA, []byte("test1"), 1*time.Second, metrics, false)
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
	mockDA.On("GasPrice", mock.Anything).Return(float64(0), nil)
	mockDA.On("GasMultiplier", mock.Anything).Return(float64(0), nil)
	mockDA.On("Submit", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("mock DA always rejects submissions"))

	// Submit a batch
	Id := []byte("test1")
	tx := []byte("transaction1")
	res, err := seq.SubmitBatchTxs(context.Background(), coresequencer.SubmitBatchTxsRequest{
		Id:    Id,
		Batch: &coresequencer.Batch{Transactions: [][]byte{tx}},
	})
	if err != nil {
		t.Fatalf("Failed to submit  transaction: %v", err)
	}
	if res == nil {
		t.Fatal("Expected response to not be nil")
	}
	time.Sleep(100 * time.Millisecond)

	// Try to get the batch before DA submission
	nextBatchResp, err := seq.GetNextBatch(context.Background(), coresequencer.GetNextBatchRequest{Id: Id})
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

// TestSequencer_RecordMetrics tests the RecordMetrics method to ensure it properly updates metrics.
func TestSequencer_RecordMetrics(t *testing.T) {
	t.Run("With Metrics", func(t *testing.T) {
		// Create a sequencer with metrics enabled
		metrics, err := NopMetrics()
		require.NoError(t, err)

		seq := &Sequencer{
			logger:  log.NewNopLogger(),
			metrics: metrics,
		}

		// Test values
		gasPrice := 1.5
		blobSize := uint64(1024)
		statusCode := coreda.StatusSuccess
		numPendingBlocks := uint64(5)
		includedBlockHeight := uint64(100)

		// Call RecordMetrics - should not panic or error
		seq.RecordMetrics(gasPrice, blobSize, statusCode, numPendingBlocks, includedBlockHeight)

		// Since we're using NopMetrics (discard metrics), we can't verify the actual values
		// but we can verify the method doesn't panic and completes successfully
		assert.NotNil(t, seq.metrics)
	})

	t.Run("Without Metrics", func(t *testing.T) {
		// Create a sequencer without metrics
		seq := &Sequencer{
			logger:  log.NewNopLogger(),
			metrics: nil, // No metrics
		}

		// Test values
		gasPrice := 2.0
		blobSize := uint64(2048)
		statusCode := coreda.StatusNotIncludedInBlock
		numPendingBlocks := uint64(3)
		includedBlockHeight := uint64(200)

		// Call RecordMetrics - should not panic even with nil metrics
		seq.RecordMetrics(gasPrice, blobSize, statusCode, numPendingBlocks, includedBlockHeight)

		// Verify metrics is still nil
		assert.Nil(t, seq.metrics)
	})

	t.Run("With Different Status Codes", func(t *testing.T) {
		// Create a sequencer with metrics
		metrics, err := NopMetrics()
		require.NoError(t, err)

		seq := &Sequencer{
			logger:  log.NewNopLogger(),
			metrics: metrics,
		}

		// Test different status codes
		testCases := []struct {
			name       string
			statusCode coreda.StatusCode
		}{
			{"Success", coreda.StatusSuccess},
			{"NotIncluded", coreda.StatusNotIncludedInBlock},
			{"AlreadyInMempool", coreda.StatusAlreadyInMempool},
			{"TooBig", coreda.StatusTooBig},
			{"ContextCanceled", coreda.StatusContextCanceled},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Call RecordMetrics with different status codes
				seq.RecordMetrics(1.0, 512, tc.statusCode, 2, 50)

				// Verify no panic occurred
				assert.NotNil(t, seq.metrics)
			})
		}
	})

}
