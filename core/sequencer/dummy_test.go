package sequencer

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestNewDummySequencer(t *testing.T) {
	seq := NewDummySequencer()
	if seq == nil {
		t.Fatal("NewDummySequencer should return a non-nil sequencer")
	}

	// Type assertion to ensure it's the correct type
	_, ok := seq.(*dummySequencer)
	if !ok {
		t.Fatal("NewDummySequencer should return a *dummySequencer")
	}
}

func TestDummySequencer_SubmitRollupBatchTxs(t *testing.T) {
	seq := NewDummySequencer()
	ctx := context.Background()

	// Create a test batch
	batch := &Batch{
		Transactions: [][]byte{
			[]byte("tx1"),
			[]byte("tx2"),
			[]byte("tx3"),
		},
	}

	rollupID := []byte("test-rollup-1")

	// Submit the batch
	resp, err := seq.SubmitRollupBatchTxs(ctx, SubmitRollupBatchTxsRequest{
		RollupId: rollupID,
		Batch:    batch,
	})

	// Verify response
	if err != nil {
		t.Fatalf("SubmitRollupBatchTxs should not return an error: %v", err)
	}
	if resp == nil {
		t.Fatal("SubmitRollupBatchTxs should return a non-nil response")
	}

	// Verify the batch was stored by retrieving it
	getResp, err := seq.GetNextBatch(ctx, GetNextBatchRequest{
		RollupId: rollupID,
	})

	if err != nil {
		t.Fatalf("GetNextBatch should not return an error after submission: %v", err)
	}
	if !reflect.DeepEqual(batch, getResp.Batch) {
		t.Fatal("Retrieved batch should match submitted batch")
	}
}

func TestDummySequencer_GetNextBatch(t *testing.T) {
	seq := NewDummySequencer()
	ctx := context.Background()

	t.Run("non-existent rollup ID", func(t *testing.T) {
		// Try to get a batch for a non-existent rollup ID
		nonExistentRollupID := []byte("non-existent-rollup")
		_, err := seq.GetNextBatch(ctx, GetNextBatchRequest{
			RollupId: nonExistentRollupID,
		})

		// Should return an error
		if err == nil {
			t.Fatal("GetNextBatch should return an error for non-existent rollup ID")
		}
		if err.Error() == "" || !contains(err.Error(), "no batch found for rollup ID") {
			t.Fatalf("Error message should indicate the rollup ID was not found, got: %v", err)
		}
	})

	t.Run("existing rollup ID", func(t *testing.T) {
		// Create and submit a test batch
		rollupID := []byte("test-rollup-2")
		batch := &Batch{
			Transactions: [][]byte{
				[]byte("tx1"),
				[]byte("tx2"),
			},
		}

		// Submit the batch
		_, err := seq.SubmitRollupBatchTxs(ctx, SubmitRollupBatchTxsRequest{
			RollupId: rollupID,
			Batch:    batch,
		})
		if err != nil {
			t.Fatalf("SubmitRollupBatchTxs should not return an error: %v", err)
		}

		// Get the batch
		getResp, err := seq.GetNextBatch(ctx, GetNextBatchRequest{
			RollupId: rollupID,
		})

		// Verify response
		if err != nil {
			t.Fatalf("GetNextBatch should not return an error for existing rollup ID: %v", err)
		}
		if getResp == nil {
			t.Fatal("GetNextBatch should return a non-nil response")
		}
		if !reflect.DeepEqual(batch, getResp.Batch) {
			t.Fatal("Retrieved batch should match submitted batch")
		}
		if getResp.Timestamp.IsZero() {
			t.Fatal("Timestamp should be set")
		}
		if time.Since(getResp.Timestamp) > 2*time.Second {
			t.Fatal("Timestamp should be recent")
		}
	})

	// Note: The dummy implementation ignores LastBatchHash and MaxBytes parameters
}

func TestDummySequencer_VerifyBatch(t *testing.T) {
	seq := NewDummySequencer()
	ctx := context.Background()

	// The dummy implementation always returns true regardless of input
	rollupID := []byte("test-rollup")
	batchData := [][]byte{[]byte("tx1"), []byte("tx2")}

	resp, err := seq.VerifyBatch(ctx, VerifyBatchRequest{
		RollupId:  rollupID,
		BatchData: batchData,
	})

	// Verify response
	if err != nil {
		t.Fatalf("VerifyBatch should not return an error: %v", err)
	}
	if resp == nil {
		t.Fatal("VerifyBatch should return a non-nil response")
	}
	if !resp.Status {
		t.Fatal("VerifyBatch should always return true for dummy implementation")
	}
}

func TestDummySequencer_Concurrency(t *testing.T) {
	seq := NewDummySequencer()
	ctx := context.Background()

	// Test concurrent submissions and retrievals
	const numGoroutines = 10
	const numOperationsPerGoroutine = 5

	// Create a wait group to wait for all goroutines to finish
	done := make(chan struct{})
	errors := make(chan error, numGoroutines*numOperationsPerGoroutine)

	for i := 0; i < numGoroutines; i++ {
		go func(routineID int) {
			for j := 0; j < numOperationsPerGoroutine; j++ {
				// Create a unique rollup ID for this operation
				rollupID := []byte(fmt.Sprintf("rollup-%d-%d", routineID, j))

				// Create a batch
				batch := &Batch{
					Transactions: [][]byte{
						[]byte(fmt.Sprintf("tx-%d-%d-1", routineID, j)),
						[]byte(fmt.Sprintf("tx-%d-%d-2", routineID, j)),
					},
				}

				// Submit the batch
				_, err := seq.SubmitRollupBatchTxs(ctx, SubmitRollupBatchTxsRequest{
					RollupId: rollupID,
					Batch:    batch,
				})
				if err != nil {
					errors <- fmt.Errorf("error submitting batch: %w", err)
					continue
				}

				// Get the batch
				getResp, err := seq.GetNextBatch(ctx, GetNextBatchRequest{
					RollupId: rollupID,
				})
				if err != nil {
					errors <- fmt.Errorf("error getting batch: %w", err)
					continue
				}

				// Verify the batch
				if !reflect.DeepEqual(batch, getResp.Batch) {
					errors <- fmt.Errorf("retrieved batch does not match submitted batch")
				}
			}

			done <- struct{}{}
		}(i)
	}

	// Wait for all goroutines to finish
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Check if there were any errors
	close(errors)
	var errs []error
	for err := range errors {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		t.Fatalf("There should be no errors in concurrent operations, got: %v", errs)
	}
}

func TestDummySequencer_MultipleRollups(t *testing.T) {
	seq := NewDummySequencer()
	ctx := context.Background()

	// Create multiple rollup IDs and batches
	rollupIDs := [][]byte{
		[]byte("rollup-1"),
		[]byte("rollup-2"),
		[]byte("rollup-3"),
	}

	batches := []*Batch{
		{Transactions: [][]byte{[]byte("tx1-1"), []byte("tx1-2")}},
		{Transactions: [][]byte{[]byte("tx2-1"), []byte("tx2-2")}},
		{Transactions: [][]byte{[]byte("tx3-1"), []byte("tx3-2")}},
	}

	// Submit batches for each rollup
	for i, rollupID := range rollupIDs {
		_, err := seq.SubmitRollupBatchTxs(ctx, SubmitRollupBatchTxsRequest{
			RollupId: rollupID,
			Batch:    batches[i],
		})
		if err != nil {
			t.Fatalf("SubmitRollupBatchTxs should not return an error: %v", err)
		}
	}

	// Retrieve and verify batches for each rollup
	for i, rollupID := range rollupIDs {
		getResp, err := seq.GetNextBatch(ctx, GetNextBatchRequest{
			RollupId: rollupID,
		})

		if err != nil {
			t.Fatalf("GetNextBatch should not return an error: %v", err)
		}
		if !reflect.DeepEqual(batches[i], getResp.Batch) {
			t.Fatalf("Retrieved batch should match submitted batch for rollup %s", rollupID)
		}
	}
}

func TestDummySequencer_BatchOverwrite(t *testing.T) {
	seq := NewDummySequencer()
	ctx := context.Background()

	rollupID := []byte("test-rollup")

	// Create and submit first batch
	batch1 := &Batch{
		Transactions: [][]byte{
			[]byte("batch1-tx1"),
			[]byte("batch1-tx2"),
		},
	}

	_, err := seq.SubmitRollupBatchTxs(ctx, SubmitRollupBatchTxsRequest{
		RollupId: rollupID,
		Batch:    batch1,
	})
	if err != nil {
		t.Fatalf("SubmitRollupBatchTxs should not return an error: %v", err)
	}

	// Create and submit second batch for the same rollup ID
	batch2 := &Batch{
		Transactions: [][]byte{
			[]byte("batch2-tx1"),
			[]byte("batch2-tx2"),
			[]byte("batch2-tx3"),
		},
	}

	_, err = seq.SubmitRollupBatchTxs(ctx, SubmitRollupBatchTxsRequest{
		RollupId: rollupID,
		Batch:    batch2,
	})
	if err != nil {
		t.Fatalf("SubmitRollupBatchTxs should not return an error: %v", err)
	}

	// Get the batch and verify it's the second one
	getResp, err := seq.GetNextBatch(ctx, GetNextBatchRequest{
		RollupId: rollupID,
	})

	if err != nil {
		t.Fatalf("GetNextBatch should not return an error: %v", err)
	}
	if !reflect.DeepEqual(batch2, getResp.Batch) {
		t.Fatal("Retrieved batch should be the most recently submitted one")
	}
	if reflect.DeepEqual(batch1, getResp.Batch) {
		t.Fatal("Retrieved batch should not be the first submitted one")
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
