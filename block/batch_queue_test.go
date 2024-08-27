package block

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/rollkit/go-sequencing"
)

// MockBatch is a mock implementation of sequencing.Batch for testing purposes
var (
	batch1 = sequencing.Batch{
		Transactions: []sequencing.Tx{
			sequencing.Tx("batch1"),
		},
	}
	batch2 = sequencing.Batch{
		Transactions: []sequencing.Tx{
			sequencing.Tx("batch2"),
		},
	}
)

func TestNewBatchQueue(t *testing.T) {
	// Test creating a new BatchQueue
	bq := NewBatchQueue()
	require.NotNil(t, bq, "NewBatchQueue should return a non-nil instance")
	require.Empty(t, bq.queue, "New BatchQueue should have an empty queue")
}

func TestBatchQueue_AddBatch(t *testing.T) {
	// Create a new BatchQueue
	bq := NewBatchQueue()

	// Add the first batch and check
	bq.AddBatch(batch1)
	require.Len(t, bq.queue, 1, "BatchQueue should have 1 batch after adding")
	require.Equal(t, batch1, bq.queue[0], "The first batch should match the one added")

	// Add the second batch and check
	bq.AddBatch(batch2)
	require.Len(t, bq.queue, 2, "BatchQueue should have 2 batches after adding another")
	require.Equal(t, batch2, bq.queue[1], "The second batch should match the one added")
}

func TestBatchQueue_Next(t *testing.T) {
	// Create a new BatchQueue
	bq := NewBatchQueue()

	// Test with empty queue
	require.Nil(t, bq.Next(), "Next should return nil when the queue is empty")

	// Add batches
	bq.AddBatch(batch1)
	bq.AddBatch(batch2)

	// Retrieve the first batch
	nextBatch := bq.Next()
	require.NotNil(t, nextBatch, "Next should return the first batch when called")
	require.Equal(t, batch1, *nextBatch, "Next should return the first batch added")
	require.Len(t, bq.queue, 1, "BatchQueue should have 1 batch after retrieving the first")

	// Retrieve the second batch
	nextBatch = bq.Next()
	require.NotNil(t, nextBatch, "Next should return the second batch when called")
	require.Equal(t, batch2, *nextBatch, "Next should return the second batch added")
	require.Empty(t, bq.queue, "BatchQueue should be empty after retrieving all batches")
}
