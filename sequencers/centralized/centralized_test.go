package centralized

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cosmossdk.io/log"
	"github.com/ipfs/go-datastore"
	ds "github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"

	coreda "github.com/rollkit/rollkit/core/da"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	dac "github.com/rollkit/rollkit/da"
)

func TestNewSequencer(t *testing.T) {
	// Create a new sequencer with mock DA client
	dummyDA := coreda.NewDummyDA(100_000_000)
	metrics, _ := NopMetrics()
	db := ds.NewMapDatastore()
	seq, err := NewSequencer(context.Background(), log.NewNopLogger(), db, dummyDA, []byte("namespace"), []byte("rollup1"), 10*time.Second, metrics)
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

	if seq.bq == nil {
		t.Fatal("Expected batch queue to not be nil")
	}
	if seq.dalc == nil {
		t.Fatal("Expected DA client to not be nil")
	}
}

func TestSequencer_SubmitRollupBatchTxs(t *testing.T) {
	// Initialize a new sequencer
	metrics, _ := NopMetrics()
	dummyDA := coreda.NewDummyDA(100_000_000)
	db := ds.NewMapDatastore()
	seq, err := NewSequencer(context.Background(), log.NewNopLogger(), db, dummyDA, []byte("namespace"), []byte("rollup1"), 10*time.Second, metrics)
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
	rollupId := []byte("rollup1")
	tx := []byte("transaction1")

	res, err := seq.SubmitRollupBatchTxs(context.Background(), coresequencer.SubmitRollupBatchTxsRequest{RollupId: rollupId, Batch: &coresequencer.Batch{Transactions: [][]byte{tx}}})
	if err != nil {
		t.Fatalf("Failed to submit rollup transaction: %v", err)
	}
	if res == nil {
		t.Fatal("Expected response to not be nil")
	}

	// Wait for the transaction to be processed
	time.Sleep(2 * time.Second)

	// Verify the transaction was added
	nextBatchresp, err := seq.GetNextBatch(context.Background(), coresequencer.GetNextBatchRequest{RollupId: rollupId, LastBatchHash: nil})
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
		bq:          NewBatchQueue(db),
		seenBatches: sync.Map{},
		rollupId:    []byte("rollup"),
	}
	defer func() {
		err := db.Close()
		if err != nil {
			t.Fatalf("Failed to close sequencer: %v", err)
		}
	}()

	// Test case where lastBatchHash and seq.lastBatchHash are both nil
	res, err := seq.GetNextBatch(context.Background(), coresequencer.GetNextBatchRequest{RollupId: seq.rollupId, LastBatchHash: nil})
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
		bq:          NewBatchQueue(db),
		seenBatches: sync.Map{},
		rollupId:    []byte("rollup"),
	}
	defer func() {
		err := db.Close()
		if err != nil {
			t.Fatalf("Failed to close sequencer: %v", err)
		}
	}()

	// Add mock batch to the BatchQueue
	err := seq.bq.AddBatch(context.Background(), *mockBatch)
	if err != nil {
		t.Fatalf("Failed to add batch: %v", err)
	}

	// Test success case with no previous lastBatchHash
	res, err := seq.GetNextBatch(context.Background(), coresequencer.GetNextBatchRequest{RollupId: seq.rollupId, LastBatchHash: nil})
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
	// Ensure the batch hash was added to seenBatches
	_, exists := seq.seenBatches.Load(hex.EncodeToString(batchHash))
	if !exists {
		t.Fatal("Expected seenBatches to not be empty")
	}
}

func TestSequencer_VerifyBatch(t *testing.T) {
	db := ds.NewMapDatastore()
	// Initialize a new sequencer with a seen batch
	seq := &Sequencer{
		bq:          NewBatchQueue(db),
		seenBatches: sync.Map{},
		rollupId:    []byte("rollup"),
	}
	defer func() {
		err := db.Close()
		if err != nil {
			t.Fatalf("Failed to close sequencer: %v", err)
		}
	}()

	// Simulate adding a batch hash
	batchHash := []byte("validHash")
	seq.seenBatches.Store(hex.EncodeToString(batchHash), struct{}{})

	// Test that VerifyBatch returns true for an existing batch
	res, err := seq.VerifyBatch(context.Background(), coresequencer.VerifyBatchRequest{RollupId: seq.rollupId, BatchHash: batchHash})
	if err != nil {
		t.Fatalf("Failed to verify batch: %v", err)
	}
	if !res.Status {
		t.Fatal("Expected status to be true for valid batch hash")
	}

	// Test that VerifyBatch returns false for a non-existing batch
	res, err = seq.VerifyBatch(context.Background(), coresequencer.VerifyBatchRequest{RollupId: seq.rollupId, BatchHash: []byte("invalidHash")})
	if err != nil {
		t.Fatalf("Failed to verify batch: %v", err)
	}
	if res.Status {
		t.Fatal("Expected status to be false for invalid batch hash")
	}
}

func TestSubmitBatchToDA(t *testing.T) {
	// Create a sample batch for testing
	sampleBatch := coresequencer.Batch{
		Transactions: [][]byte{
			[]byte("tx1"),
			[]byte("tx2"),
		},
	}

	tests := []struct {
		name             string
		batch            coresequencer.Batch
		maxBlobSize      uint64
		simulateTooBig   bool
		simulateDAError  bool
		expectedError    bool
		expectedErrorMsg string
		expectedAttempts int
		// New fields for expanded test cases
		temporaryErrors int             // Number of temporary errors before success
		simulateTimeout bool            // Simulate context timeout
		simulateBackoff bool            // Track if backoff was properly applied
		backoffDelays   []time.Duration // Expected backoff delays
	}{
		{
			name:             "successful submission",
			batch:            sampleBatch,
			maxBlobSize:      1024 * 1024,
			expectedError:    false,
			expectedAttempts: 1,
		},
		{
			name:             "blob too big",
			batch:            sampleBatch,
			maxBlobSize:      10, // Very small max blob size to trigger StatusTooBig
			simulateTooBig:   true,
			expectedError:    false, // Should succeed after reducing blob size
			expectedAttempts: 2,
		},
		{
			name:             "DA error",
			batch:            sampleBatch,
			maxBlobSize:      1024 * 1024,
			simulateDAError:  true,
			expectedError:    true,
			expectedErrorMsg: "failed to submit all blocks to DA layer",
			expectedAttempts: maxSubmitAttempts,
		},
		{
			name:             "temporary errors with exponential backoff",
			batch:            sampleBatch,
			maxBlobSize:      1024 * 1024,
			temporaryErrors:  3, // Fail 3 times then succeed
			expectedError:    false,
			expectedAttempts: 4, // 3 failures + 1 success
			simulateBackoff:  true,
		},
		{
			name:             "context timeout during retry",
			batch:            sampleBatch,
			maxBlobSize:      1024 * 1024,
			simulateTimeout:  true,
			expectedError:    true,
			expectedErrorMsg: "context deadline exceeded",
			expectedAttempts: 1, // Only one attempt before timeout
		},
		{
			name:             "empty batch submission",
			batch:            coresequencer.Batch{Transactions: [][]byte{}},
			maxBlobSize:      1024 * 1024,
			expectedError:    false,
			expectedAttempts: 1,
		},
		{
			name:             "partial success then failure",
			batch:            sampleBatch,
			maxBlobSize:      1024 * 1024,
			temporaryErrors:  maxSubmitAttempts - 1, // Almost reach max attempts
			expectedError:    true,
			expectedErrorMsg: "failed to submit all blocks to DA layer",
			expectedAttempts: maxSubmitAttempts,
			simulateBackoff:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a context with timeout if simulating timeout
			var ctx context.Context
			var cancel context.CancelFunc
			if tt.simulateTimeout {
				ctx, cancel = context.WithTimeout(context.Background(), 10*time.Millisecond)
			} else {
				ctx, cancel = context.WithCancel(context.Background())
			}
			defer cancel()

			// Create a DummyDA with the specified max blob size
			dummyDA := coreda.NewDummyDA(tt.maxBlobSize)

			// Create a wrapper around DummyDA to intercept calls and simulate errors if needed
			daWrapper := &dummyDAWrapper{
				DummyDA:         dummyDA,
				simulateTooBig:  tt.simulateTooBig,
				simulateDAError: tt.simulateDAError,
				callCount:       0,
				temporaryErrors: tt.temporaryErrors,
				simulateBackoff: tt.simulateBackoff,
				backoffTimes:    make([]time.Time, 0),
			}

			// Create a DAClient with the wrapper
			dalc := dac.NewDAClient(daWrapper, 1.0, 2.0, coreda.Namespace("test"), log.NewTestLogger(t), nil)

			// Create a sequencer with the DA client
			sequencer := &Sequencer{
				logger:      log.NewTestLogger(t),
				dalc:        dalc,
				batchTime:   time.Second,
				maxBlobSize: &atomic.Uint64{},
				rollupId:    []byte("test-rollup"),
				bq:          NewBatchQueue(dssync.MutexWrap(datastore.NewMapDatastore())),
				seenBatches: sync.Map{},
				metrics:     nil,
			}

			// Set the initial max blob size
			sequencer.maxBlobSize.Store(tt.maxBlobSize)

			// If simulating timeout, add a small delay to ensure timeout happens during retry
			if tt.simulateTimeout {
				daWrapper.addDelay = 50 * time.Millisecond
			}

			// Call the method under test
			err := sequencer.submitBatchToDA(ctx, tt.batch)

			// Verify the results
			if tt.expectedError {
				// rewrite to not use testify
				if err == nil {
					t.Fatalf("Expected error, got nil")
				}
				if tt.expectedErrorMsg != "" {
					if !strings.Contains(err.Error(), tt.expectedErrorMsg) {
						t.Fatalf("Expected error message to contain %q, got %q", tt.expectedErrorMsg, err.Error())
					}
				}
			} else {
				if err != nil {
					t.Fatalf("Expected no error, got %v", err)
				}
			}

			// Verify the number of attempts
			if tt.expectedAttempts != daWrapper.callCount {
				t.Fatalf("Expected %d attempts, got %d", tt.expectedAttempts, daWrapper.callCount)
			}

			// Verify backoff behavior if applicable
			if tt.simulateBackoff && daWrapper.callCount > 1 {
				// Check that time between retries increases (exponential backoff)
				for i := 1; i < len(daWrapper.backoffTimes); i++ {
					currentInterval := daWrapper.backoffTimes[i].Sub(daWrapper.backoffTimes[i-1])
					if i > 1 {
						previousInterval := daWrapper.backoffTimes[i-1].Sub(daWrapper.backoffTimes[i-2])
						// Allow some flexibility due to scheduling variations
						if currentInterval.Milliseconds() < previousInterval.Milliseconds() {
							t.Fatalf("Expected exponential backoff between retry attempts, got %v and %v", currentInterval, previousInterval)
						}
					}
				}
			}
		})
	}
}

// dummyDAWrapper wraps a DummyDA to simulate different error conditions
type dummyDAWrapper struct {
	*coreda.DummyDA
	simulateTooBig  bool
	simulateDAError bool
	callCount       int
	temporaryErrors int           // Number of temporary errors to simulate before success
	simulateBackoff bool          // Whether to track backoff times
	backoffTimes    []time.Time   // Records when each call was made to check backoff
	addDelay        time.Duration // Additional delay to add (for timeout tests)
}

// Submit overrides the Submit method to simulate errors
func (w *dummyDAWrapper) Submit(ctx context.Context, blobs []coreda.Blob, gasPrice float64, namespace coreda.Namespace) ([]coreda.ID, error) {
	w.callCount++

	// Record time for backoff verification
	if w.simulateBackoff {
		w.backoffTimes = append(w.backoffTimes, time.Now())
	}

	// Add artificial delay if needed (for timeout tests)
	if w.addDelay > 0 {
		time.Sleep(w.addDelay)
	}

	// Check for context cancellation
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Simulate temporary errors
	if w.temporaryErrors > 0 {
		w.temporaryErrors--
		return nil, errors.New("temporary DA error")
	}

	// Simulate permanent DA error
	if w.simulateDAError {
		return nil, errors.New("simulated DA error")
	}

	// Simulate "blob too big" error on first attempt
	if w.simulateTooBig && w.callCount == 1 {
		return nil, errors.New("blob size exceeds maximum")
	}

	return w.DummyDA.Submit(ctx, blobs, gasPrice, namespace)
}

// SubmitWithOptions overrides the SubmitWithOptions method to simulate errors
func (w *dummyDAWrapper) SubmitWithOptions(ctx context.Context, blobs []coreda.Blob, gasPrice float64, namespace coreda.Namespace, options []byte) ([]coreda.ID, error) {
	w.callCount++

	// Record time for backoff verification
	if w.simulateBackoff {
		w.backoffTimes = append(w.backoffTimes, time.Now())
	}

	// Add artificial delay if needed (for timeout tests)
	if w.addDelay > 0 {
		time.Sleep(w.addDelay)
	}

	// Check for context cancellation
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Simulate temporary errors
	if w.temporaryErrors > 0 {
		w.temporaryErrors--
		return nil, errors.New("temporary DA error")
	}

	// Simulate permanent DA error
	if w.simulateDAError {
		return nil, errors.New("simulated DA error")
	}

	// Simulate "blob too big" error on first attempt
	if w.simulateTooBig && w.callCount == 1 {
		return nil, errors.New("blob size exceeds maximum")
	}

	return w.DummyDA.SubmitWithOptions(ctx, blobs, gasPrice, namespace, options)
}
