package types_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"cosmossdk.io/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	coreda "github.com/rollkit/rollkit/core/da"
	"github.com/rollkit/rollkit/test/mocks"
	"github.com/rollkit/rollkit/types"
)

func TestSubmitWithHelpers(t *testing.T) {
	logger := log.NewNopLogger()

	testCases := []struct {
		name           string
		data           [][]byte
		gasPrice       float64
		options        []byte
		submitErr      error
		submitIDs      [][]byte
		expectedCode   coreda.StatusCode
		expectedErrMsg string
		expectedIDs    [][]byte
		expectedCount  uint64
	}{
		{
			name:          "successful submission",
			data:          [][]byte{[]byte("blob1"), []byte("blob2")},
			gasPrice:      1.0,
			options:       []byte("opts"),
			submitIDs:     [][]byte{[]byte("id1"), []byte("id2")},
			expectedCode:  coreda.StatusSuccess,
			expectedIDs:   [][]byte{[]byte("id1"), []byte("id2")},
			expectedCount: 2,
		},
		{
			name:           "context canceled error",
			data:           [][]byte{[]byte("blob1")},
			gasPrice:       1.0,
			options:        []byte("opts"),
			submitErr:      context.Canceled,
			expectedCode:   coreda.StatusContextCanceled,
			expectedErrMsg: "submission canceled",
		},
		{
			name:           "tx timed out error",
			data:           [][]byte{[]byte("blob1")},
			gasPrice:       1.0,
			options:        []byte("opts"),
			submitErr:      coreda.ErrTxTimedOut,
			expectedCode:   coreda.StatusNotIncludedInBlock,
			expectedErrMsg: "failed to submit blobs: " + coreda.ErrTxTimedOut.Error(),
		},
		{
			name:           "tx already in mempool error",
			data:           [][]byte{[]byte("blob1")},
			gasPrice:       1.0,
			options:        []byte("opts"),
			submitErr:      coreda.ErrTxAlreadyInMempool,
			expectedCode:   coreda.StatusAlreadyInMempool,
			expectedErrMsg: "failed to submit blobs: " + coreda.ErrTxAlreadyInMempool.Error(),
		},
		{
			name:           "incorrect account sequence error",
			data:           [][]byte{[]byte("blob1")},
			gasPrice:       1.0,
			options:        []byte("opts"),
			submitErr:      coreda.ErrTxIncorrectAccountSequence,
			expectedCode:   coreda.StatusIncorrectAccountSequence,
			expectedErrMsg: "failed to submit blobs: " + coreda.ErrTxIncorrectAccountSequence.Error(),
		},
		{
			name:           "blob size over limit error",
			data:           [][]byte{[]byte("blob1")},
			gasPrice:       1.0,
			options:        []byte("opts"),
			submitErr:      coreda.ErrBlobSizeOverLimit,
			expectedCode:   coreda.StatusTooBig,
			expectedErrMsg: "failed to submit blobs: " + coreda.ErrBlobSizeOverLimit.Error(),
		},
		{
			name:           "context deadline error",
			data:           [][]byte{[]byte("blob1")},
			gasPrice:       1.0,
			options:        []byte("opts"),
			submitErr:      coreda.ErrContextDeadline,
			expectedCode:   coreda.StatusContextDeadline,
			expectedErrMsg: "failed to submit blobs: " + coreda.ErrContextDeadline.Error(),
		},
		{
			name:           "generic submission error",
			data:           [][]byte{[]byte("blob1")},
			gasPrice:       1.0,
			options:        []byte("opts"),
			submitErr:      errors.New("some generic error"),
			expectedCode:   coreda.StatusError,
			expectedErrMsg: "failed to submit blobs: some generic error",
		},
		{
			name:           "no IDs returned for non-empty data",
			data:           [][]byte{[]byte("blob1")},
			gasPrice:       1.0,
			options:        []byte("opts"),
			submitIDs:      [][]byte{},
			expectedCode:   coreda.StatusError,
			expectedErrMsg: "failed to submit blobs: no IDs returned despite non-empty input",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockDA := mocks.NewMockDA(t)
			mockDA.On("SubmitWithOptions", mock.Anything, tc.data, tc.gasPrice, mock.Anything, tc.options).Return(tc.submitIDs, tc.submitErr)

			result := types.SubmitWithHelpers(context.Background(), mockDA, logger, tc.data, tc.gasPrice, tc.options)

			assert.Equal(t, tc.expectedCode, result.Code)
			if tc.expectedErrMsg != "" {
				assert.Contains(t, result.Message, tc.expectedErrMsg)
			}
			if tc.expectedIDs != nil {
				assert.Equal(t, tc.expectedIDs, result.IDs)
			}
			if tc.expectedCount != 0 {
				assert.Equal(t, tc.expectedCount, result.SubmittedCount)
			}
			mockDA.AssertExpectations(t)
		})
	}
}

func TestRetrieveWithHelpers(t *testing.T) {
	logger := log.NewNopLogger()
	dataLayerHeight := uint64(100)
	mockIDs := [][]byte{[]byte("id1"), []byte("id2")}
	mockBlobs := [][]byte{[]byte("blobA"), []byte("blobB")}
	mockTimestamp := time.Now()

	testCases := []struct {
		name           string
		getIDsResult   *coreda.GetIDsResult
		getIDsErr      error
		getBlobsErr    error
		expectedCode   coreda.StatusCode
		expectedErrMsg string
		expectedIDs    [][]byte
		expectedData   [][]byte
		expectedHeight uint64
	}{
		{
			name: "successful retrieval",
			getIDsResult: &coreda.GetIDsResult{
				IDs:       mockIDs,
				Timestamp: mockTimestamp,
			},
			expectedCode:   coreda.StatusSuccess,
			expectedIDs:    mockIDs,
			expectedData:   mockBlobs,
			expectedHeight: dataLayerHeight,
		},
		{
			name:           "blob not found error during GetIDs",
			getIDsErr:      coreda.ErrBlobNotFound,
			expectedCode:   coreda.StatusNotFound,
			expectedErrMsg: coreda.ErrBlobNotFound.Error(),
			expectedHeight: dataLayerHeight,
		},
		{
			name:           "height from future error during GetIDs",
			getIDsErr:      coreda.ErrHeightFromFuture,
			expectedCode:   coreda.StatusHeightFromFuture,
			expectedErrMsg: coreda.ErrHeightFromFuture.Error(),
			expectedHeight: dataLayerHeight,
		},
		{
			name:           "generic error during GetIDs",
			getIDsErr:      errors.New("failed to connect to DA"),
			expectedCode:   coreda.StatusError,
			expectedErrMsg: "failed to get IDs: failed to connect to DA",
			expectedHeight: dataLayerHeight,
		},
		{
			name:           "GetIDs returns nil result",
			getIDsResult:   nil,
			expectedCode:   coreda.StatusNotFound,
			expectedErrMsg: coreda.ErrBlobNotFound.Error(),
			expectedHeight: dataLayerHeight,
		},
		{
			name: "GetIDs returns empty IDs",
			getIDsResult: &coreda.GetIDsResult{
				IDs:       [][]byte{},
				Timestamp: mockTimestamp,
			},
			expectedCode:   coreda.StatusNotFound,
			expectedErrMsg: coreda.ErrBlobNotFound.Error(),
			expectedHeight: dataLayerHeight,
		},
		{
			name: "error during Get (blobs retrieval)",
			getIDsResult: &coreda.GetIDsResult{
				IDs:       mockIDs,
				Timestamp: mockTimestamp,
			},
			getBlobsErr:    errors.New("network error during blob retrieval"),
			expectedCode:   coreda.StatusError,
			expectedErrMsg: "failed to get blobs: network error during blob retrieval",
			expectedHeight: dataLayerHeight,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockDA := mocks.NewMockDA(t)

			mockDA.On("GetIDs", mock.Anything, dataLayerHeight, mock.Anything).Return(tc.getIDsResult, tc.getIDsErr)

			if tc.getIDsErr == nil && tc.getIDsResult != nil && len(tc.getIDsResult.IDs) > 0 {
				mockDA.On("Get", mock.Anything, tc.getIDsResult.IDs, mock.Anything).Return(mockBlobs, tc.getBlobsErr)
			}

			result := types.RetrieveWithHelpers(context.Background(), mockDA, logger, dataLayerHeight, []byte("test-namespace"))

			assert.Equal(t, tc.expectedCode, result.Code)
			assert.Equal(t, tc.expectedHeight, result.Height)
			if tc.expectedErrMsg != "" {
				assert.Contains(t, result.Message, tc.expectedErrMsg)
			}
			if tc.expectedIDs != nil {
				assert.Equal(t, tc.expectedIDs, result.IDs)
			}
			if tc.expectedData != nil {
				assert.Equal(t, tc.expectedData, result.Data)
			}
			mockDA.AssertExpectations(t)
		})
	}
}
