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
	damocks "github.com/rollkit/rollkit/da/mocks"
	"github.com/stretchr/testify/mock"
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

	// Test with initial rollup ID
	tx := []byte("transaction1")

	res, err := seq.SubmitRollupBatchTxs(context.Background(), coresequencer.SubmitRollupBatchTxsRequest{RollupId: rollupId, Batch: &coresequencer.Batch{Transactions: [][]byte{tx}}})
	if err != nil {
		t.Fatalf("Failed to submit rollup transaction: %v", err)
	}
	if res == nil {
		t.Fatal("Expected response to not be nil")
	}

	err = seq.publishBatch(t.Context())
	if err != nil {
		t.Fatalf("Failed to publish batch: %v", err)
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
	// Initialize a new sequencer with a seen batch
	seq := &Sequencer{
		logger:           log.NewNopLogger(),
		queue:            NewBatchQueue(db, "batches"),
		daSubmissionChan: make(chan coresequencer.Batch, 100),
		rollupId:         []byte("rollup"),
		proposer:         true,
	}
	defer func() {
		err := db.Close()
		require.NoError(err, "Failed to close datastore")
	}()

	rollupId := []byte("rollup")
	namespace := []byte("test-namespace")
	batchData := [][]byte{[]byte("batch1"), []byte("batch2")} // Example batch data (IDs)
	proofs := [][]byte{[]byte("proof1"), []byte("proof2")}    // Example proofs
	// Simulate adding a batch hash
	batchHash := []byte("validHash")

	// Test Case 1: Proposer=true should always return true
	t.Run("Proposer Mode", func(t *testing.T) {
		mockDA := mocks.NewDA(t) // Mock DA, though it shouldn't be called
		dummyClient := coreda.NewDummyClient(mockDA, namespace)
		seq := &Sequencer{
			bq:          NewBatchQueue(db, "pending_proposer"),
			sbq:         NewBatchQueue(db, "submitted_proposer"),
			seenBatches: sync.Map{},
			rollupId:    rollupId,
			proposer:    true, // Set proposer mode
			dalc:        dummyClient,
			da:          mockDA,
		}

		res, err := seq.VerifyBatch(context.Background(), coresequencer.VerifyBatchRequest{RollupId: seq.rollupId, BatchData: batchData})
		assert.NoError(err)
		assert.NotNil(res)
		assert.True(res.Status, "Expected status to be true in proposer mode")
		// Ensure no DA methods were called
		mockDA.AssertNotCalled(t, "GetProofs", mock.Anything, mock.Anything, mock.Anything)
		mockDA.AssertNotCalled(t, "Validate", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})

	// Test Cases for Non-Proposer Mode
	t.Run("Non-Proposer Mode", func(t *testing.T) {
		// Sub-test 2.1: Valid proofs
		t.Run("Valid Proofs", func(t *testing.T) {
			mockDA := mocks.NewDA(t)
			dummyClient := coreda.NewDummyClient(mockDA, namespace)
			seq := &Sequencer{
				bq:          NewBatchQueue(db, "pending_valid"),
				sbq:         NewBatchQueue(db, "submitted_valid"),
				seenBatches: sync.Map{},
				rollupId:    rollupId,
				proposer:    false,
				dalc:        dummyClient,
				da:          mockDA,
			}

			// Setup mock expectations
			mockDA.On("GetProofs", mock.Anything, batchData, namespace).Return(proofs, nil).Once()
			mockDA.On("Validate", mock.Anything, batchData, proofs, namespace).Return([]bool{true, true}, nil).Once()

			res, err := seq.VerifyBatch(context.Background(), coresequencer.VerifyBatchRequest{RollupId: seq.rollupId, BatchData: batchData})
			assert.NoError(err)
			assert.NotNil(res)
			assert.True(res.Status, "Expected status to be true for valid proofs")
			mockDA.AssertExpectations(t)
		})

		// Sub-test 2.2: Invalid proof
		t.Run("Invalid Proof", func(t *testing.T) {
			mockDA := mocks.NewDA(t)
			dummyClient := coreda.NewDummyClient(mockDA, namespace)
			seq := &Sequencer{
				bq:          NewBatchQueue(db, "pending_invalid"),
				sbq:         NewBatchQueue(db, "submitted_invalid"),
				seenBatches: sync.Map{},
				rollupId:    rollupId,
				proposer:    false,
				dalc:        dummyClient,
				da:          mockDA,
			}

			// Setup mock expectations (Validate returns false)
			mockDA.On("GetProofs", mock.Anything, batchData, namespace).Return(proofs, nil).Once()
			mockDA.On("Validate", mock.Anything, batchData, proofs, namespace).Return([]bool{true, false}, nil).Once() // One proof is invalid

			res, err := seq.VerifyBatch(context.Background(), coresequencer.VerifyBatchRequest{RollupId: seq.rollupId, BatchData: batchData})
			assert.NoError(err)
			assert.NotNil(res)
			assert.False(res.Status, "Expected status to be false for invalid proof")
			mockDA.AssertExpectations(t)
		})

		// Sub-test 2.3: GetProofs error
		t.Run("GetProofs Error", func(t *testing.T) {
			mockDA := mocks.NewDA(t)
			dummyClient := coreda.NewDummyClient(mockDA, namespace)
			seq := &Sequencer{
				bq:          NewBatchQueue(db, "pending_getproofs_err"),
				sbq:         NewBatchQueue(db, "submitted_getproofs_err"),
				seenBatches: sync.Map{},
				rollupId:    rollupId,
				proposer:    false,
				dalc:        dummyClient,
				da:          mockDA,
			}
			expectedErr := errors.New("get proofs failed")

			// Setup mock expectations
			mockDA.On("GetProofs", mock.Anything, batchData, namespace).Return(nil, expectedErr).Once()

			res, err := seq.VerifyBatch(context.Background(), coresequencer.VerifyBatchRequest{RollupId: seq.rollupId, BatchData: batchData})
			assert.Error(err)
			assert.Nil(res)
			assert.Contains(err.Error(), expectedErr.Error())
			mockDA.AssertExpectations(t)
			mockDA.AssertNotCalled(t, "Validate", mock.Anything, mock.Anything, mock.Anything, mock.Anything) // Validate should not be called
		})

		// Sub-test 2.4: Validate error
		t.Run("Validate Error", func(t *testing.T) {
			mockDA := mocks.NewDA(t)
			dummyClient := coreda.NewDummyClient(mockDA, namespace)
			seq := &Sequencer{
				bq:          NewBatchQueue(db, "pending_validate_err"),
				sbq:         NewBatchQueue(db, "submitted_validate_err"),
				seenBatches: sync.Map{},
				rollupId:    rollupId,
				proposer:    false,
				dalc:        dummyClient,
				da:          mockDA,
			}
			expectedErr := errors.New("validate failed")

			// Setup mock expectations
			mockDA.On("GetProofs", mock.Anything, batchData, namespace).Return(proofs, nil).Once()
			mockDA.On("Validate", mock.Anything, batchData, proofs, namespace).Return(nil, expectedErr).Once()

			res, err := seq.VerifyBatch(context.Background(), coresequencer.VerifyBatchRequest{RollupId: seq.rollupId, BatchData: batchData})
			assert.Error(err)
			assert.Nil(res)
			assert.Contains(err.Error(), expectedErr.Error())
			mockDA.AssertExpectations(t)
		})

		// Sub-test 2.5: Invalid Rollup ID
		t.Run("Invalid Rollup ID", func(t *testing.T) {
			mockDA := mocks.NewDA(t) // Mock DA, though it shouldn't be called
			dummyClient := coreda.NewDummyClient(mockDA, namespace)
			seq := &Sequencer{
				bq:          NewBatchQueue(db, "pending_invalid_rollup"),
				sbq:         NewBatchQueue(db, "submitted_invalid_rollup"),
				seenBatches: sync.Map{},
				rollupId:    rollupId,
				proposer:    false,
				dalc:        dummyClient,
				da:          mockDA,
			}

			invalidRollupId := []byte("invalidRollup")
			res, err := seq.VerifyBatch(context.Background(), coresequencer.VerifyBatchRequest{RollupId: invalidRollupId, BatchData: batchData})
			assert.Error(err)
			assert.Nil(res)
			assert.ErrorIs(err, ErrInvalidRollupId)
			// Ensure no DA methods were called
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
	mockDA.On("Submit", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
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
