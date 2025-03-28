package block

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	"github.com/rollkit/rollkit/pkg/queue"
)

func TestBatchDataConversion(t *testing.T) {
	testCases := []struct {
		name      string
		batchData [][]byte
	}{
		{
			name:      "empty batch data",
			batchData: [][]byte{},
		},
		{
			name:      "single item",
			batchData: [][]byte{[]byte("test")},
		},
		{
			name:      "multiple items",
			batchData: [][]byte{[]byte("test1"), []byte("test2"), []byte("test3")},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Convert to bytes
			bytes := convertBatchDataToBytes(tc.batchData)

			// Convert back to batch data
			recoveredData, err := bytesToBatchData(bytes)
			require.NoError(t, err)

			// Verify original and recovered data match
			require.Equal(t, tc.batchData, recoveredData)
		})
	}
}

func TestBytesToBatchDataErrors(t *testing.T) {
	testCases := []struct {
		name          string
		data          []byte
		expectedError string
	}{
		{
			name:          "corrupted length prefix",
			data:          []byte{0, 0, 0}, // Missing one byte for length prefix
			expectedError: "corrupted data: insufficient bytes for length prefix at offset 0",
		},
		{
			name:          "insufficient data",
			data:          []byte{0, 0, 0, 10, 1, 2, 3}, // Length says 10 but only 3 bytes present
			expectedError: "corrupted data: insufficient bytes for entry of length 10 at offset 4",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := bytesToBatchData(tc.data)
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.expectedError)
		})
	}
}

func TestGetTxsFromBatch(t *testing.T) {
	m := &Manager{
		bq: queue.New[BatchData](),
	}

	// Test when batch queue is empty
	_, err := m.getTxsFromBatch()
	require.Error(t, err)
	require.Equal(t, ErrNoBatch, err)

	// Test when batch queue has data
	testBatch := &coresequencer.Batch{
		Transactions: [][]byte{[]byte("tx1"), []byte("tx2")},
	}
	testTime := time.Now()
	testBatchData := [][]byte{[]byte("data1")}

	m.bq.Add(BatchData{
		Batch: testBatch,
		Time:  testTime,
		Data:  testBatchData,
	})

	result, err := m.getTxsFromBatch()
	require.NoError(t, err)
	require.Equal(t, testBatch, result.Batch)
	require.Equal(t, testTime, result.Time)
	require.Equal(t, testBatchData, result.Data)
}
