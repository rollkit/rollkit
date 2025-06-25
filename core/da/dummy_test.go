package da

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestDummyDA(t *testing.T) {
	testDABlockTime := 100 * time.Millisecond
	// Create a new DummyDA instance with a max blob size of 1024 bytes
	dummyDA := NewDummyDA(1024, 0, 0, testDABlockTime)
	dummyDA.StartHeightTicker()
	defer dummyDA.StopHeightTicker()
	// Height is always 0
	ctx := context.Background()

	// Test MaxBlobSize
	maxSize, err := dummyDA.MaxBlobSize(ctx)
	if err != nil {
		t.Fatalf("MaxBlobSize failed: %v", err)
	}
	if maxSize != 1024 {
		t.Errorf("Expected max blob size 1024, got %d", maxSize)
	}

	// Test Submit
	blobs := []Blob{
		[]byte("test blob 1"),
		[]byte("test blob 2"),
	}
	ids, err := dummyDA.Submit(ctx, blobs, nil)
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}
	err = waitForFirstDAHeight(ctx, dummyDA) // Wait for height to increment
	if err != nil {
		t.Fatalf("waitForFirstDAHeight failed: %v", err)
	}
	if len(ids) != len(blobs) {
		t.Errorf("Expected %d IDs, got %d", len(blobs), len(ids))
	}

	// Test Get
	retrievedBlobs, err := dummyDA.Get(ctx, ids)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if len(retrievedBlobs) != len(blobs) {
		t.Errorf("Expected %d blobs, got %d", len(blobs), len(retrievedBlobs))
	}
	for i, blob := range blobs {
		if string(retrievedBlobs[i]) != string(blob) {
			t.Errorf("Expected blob %q, got %q", string(blob), string(retrievedBlobs[i]))
		}
	}

	// Test GetIDs
	result, err := dummyDA.GetIDs(ctx, 1)
	if err != nil {
		t.Fatalf("GetIDs failed: %v", err)
	}
	if len(result.IDs) != len(ids) {
		t.Errorf("Expected %d IDs, got %d", len(ids), len(result.IDs))
	}

	// Test Commit
	commitments, err := dummyDA.Commit(ctx, blobs)
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}
	if len(commitments) != len(blobs) {
		t.Errorf("Expected %d commitments, got %d", len(blobs), len(commitments))
	}

	// Test GetProofs
	proofs, err := dummyDA.GetProofs(ctx, ids)
	if err != nil {
		t.Fatalf("GetProofs failed: %v", err)
	}
	if len(proofs) != len(ids) {
		t.Errorf("Expected %d proofs, got %d", len(ids), len(proofs))
	}

	// Test Validate
	validations, err := dummyDA.Validate(ctx, ids, proofs)
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	if len(validations) != len(ids) {
		t.Errorf("Expected %d validations, got %d", len(ids), len(validations))
	}
	for _, valid := range validations {
		if !valid {
			t.Errorf("Expected validation to be true")
		}
	}

	// Test error case: blob size exceeds maximum
	largeBlob := make([]byte, 2048) // Larger than our max of 1024
	_, err = dummyDA.Submit(ctx, []Blob{largeBlob}, nil)
	if err == nil {
		t.Errorf("Expected error for blob exceeding max size, got nil")
	}
}

func waitForFirstDAHeight(ctx context.Context, da *DummyDA) error {
	return waitForAtLeastDAHeight(ctx, da, 1)
}

// waitForAtLeastDAHeight waits for the DummyDA to reach at least the given height
func waitForAtLeastDAHeight(ctx context.Context, da *DummyDA, targetHeight uint64) error {
	// Read current height at the start
	da.mu.RLock()
	current := da.height
	da.mu.RUnlock()

	if current >= targetHeight {
		return nil
	}

	delta := targetHeight - current

	// Dynamically set pollInterval and timeout based on delta
	pollInterval := da.blockTime / 2
	timeout := da.blockTime * time.Duration(delta+2)

	deadline := time.Now().Add(timeout)
	for {
		da.mu.RLock()
		current = da.height
		da.mu.RUnlock()
		if current >= targetHeight {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for DA height %d, current %d", targetHeight, current)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}
