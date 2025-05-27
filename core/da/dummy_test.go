package da

import (
	"context"
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
	ids, err := dummyDA.Submit(ctx, blobs, 0, nil)
	time.Sleep(testDABlockTime)
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}
	if len(ids) != len(blobs) {
		t.Errorf("Expected %d IDs, got %d", len(blobs), len(ids))
	}

	// Test Get
	retrievedBlobs, err := dummyDA.Get(ctx, ids, nil)
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
	result, err := dummyDA.GetIDs(ctx, 1, nil)
	if err != nil {
		t.Fatalf("GetIDs failed: %v", err)
	}
	if len(result.IDs) != len(ids) {
		t.Errorf("Expected %d IDs, got %d", len(ids), len(result.IDs))
	}

	// Test Commit
	commitments, err := dummyDA.Commit(ctx, blobs, nil)
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}
	if len(commitments) != len(blobs) {
		t.Errorf("Expected %d commitments, got %d", len(blobs), len(commitments))
	}

	// Test GetProofs
	proofs, err := dummyDA.GetProofs(ctx, ids, nil)
	if err != nil {
		t.Fatalf("GetProofs failed: %v", err)
	}
	if len(proofs) != len(ids) {
		t.Errorf("Expected %d proofs, got %d", len(ids), len(proofs))
	}

	// Test Validate
	validations, err := dummyDA.Validate(ctx, ids, proofs, nil)
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
	_, err = dummyDA.Submit(ctx, []Blob{largeBlob}, 0, nil)
	if err == nil {
		t.Errorf("Expected error for blob exceeding max size, got nil")
	}
}
