package da

import (
	"context"
	"testing"
)

func TestDummyClient(t *testing.T) {
	// Create a new DummyDA with a max blob size of 1024 bytes
	dummyDA := NewDummyDA(1024, 0, 0)

	// Create a new DummyClient with the DummyDA
	namespace := []byte("test-namespace")
	client := NewDummyClient(dummyDA, namespace)

	// Test MaxBlobSize
	ctx := context.Background()
	maxBlobSize, err := client.MaxBlobSize(ctx)
	if err != nil {
		t.Errorf("Unexpected error from MaxBlobSize: %v", err)
	}
	if maxBlobSize != 1024 {
		t.Errorf("Expected max blob size 1024, got %v", maxBlobSize)
	}

	// Create some test headers
	headers := [][]byte{
		[]byte("header1"),
		[]byte("header2"),
		[]byte("header3"),
	}

	// Test SubmitHeaders
	submitResult := client.SubmitHeaders(ctx, headers, maxBlobSize, 1.0)

	// Check the result
	if submitResult.Code != StatusSuccess {
		t.Errorf("Expected StatusSuccess, got %v", submitResult.Code)
	}
	if submitResult.SubmittedCount != 3 {
		t.Errorf("Expected 3 submitted headers, got %v", submitResult.SubmittedCount)
	}

	// Test RetrieveHeaders
	retrieveResult := client.RetrieveHeaders(ctx, 1) // Use height 1 as set in the dummy implementation

	// Check the result
	if retrieveResult.Code != StatusSuccess {
		t.Errorf("Expected StatusSuccess, got %v", retrieveResult.Code)
	}
	if len(retrieveResult.Headers) != 3 {
		t.Errorf("Expected 3 retrieved headers, got %v", len(retrieveResult.Headers))
	}

	// Check that the retrieved headers match the submitted ones
	for i, header := range headers {
		if string(retrieveResult.Headers[i]) != string(header) {
			t.Errorf("Expected header %v, got %v", string(header), string(retrieveResult.Headers[i]))
		}
	}

	// Test retrieving headers from a non-existent height
	nonExistentResult := client.RetrieveHeaders(ctx, 999)

	// Check the result
	if nonExistentResult.Code != StatusSuccess {
		t.Errorf("Expected StatusSuccess, got %v", nonExistentResult.Code)
	}
	if len(nonExistentResult.Headers) != 0 {
		t.Errorf("Expected 0 retrieved headers, got %v", len(nonExistentResult.Headers))
	}
}
