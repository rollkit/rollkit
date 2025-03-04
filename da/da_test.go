package da

import (
	"context"
	"errors"
	"math/rand"
	"testing"
	"time"

	"cosmossdk.io/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	coreda "github.com/rollkit/rollkit/core/da"
	"github.com/rollkit/rollkit/da/mocks"
)

const (
	// MockDABlockTime is the mock da block time
	MockDABlockTime = 100 * time.Millisecond

	// MockDAAddress is the mock address for the gRPC server
	MockDAAddress = "grpc://localhost:7980"

	// MockDAAddressHTTP is mock address for the JSONRPC server
	MockDAAddressHTTP = "http://localhost:7988"

	// MockDANamespace is the mock namespace
	MockDANamespace = "00000000000000000000000000000000000000000000000000deadbeef"

	submitTimeout = 50 * time.Millisecond
)

func TestMockDAErrors(t *testing.T) {
	t.Run("submit_timeout", func(t *testing.T) {
		mockDA := &mocks.DA{}
		dalc := NewDAClient(mockDA, -1, -1, nil, nil, log.NewTestLogger(t))
		blobs := make([]coreda.Blob, 1)
		blobs[0] = make([]byte, 1234)
		// Set up the mock to throw context deadline exceeded
		mockDA.On("MaxBlobSize", mock.Anything).Return(uint64(1234), nil)
		mockDA.
			On("Submit", mock.Anything, blobs, float64(-1), []byte(nil)).
			After(submitTimeout).
			Return(nil, uint64(0), ErrContextDeadline)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		maxBlobSize, err := dalc.MaxBlobSize(ctx)
		require.NoError(t, err)

		assert := assert.New(t)

		ctx, cancel = context.WithTimeout(ctx, submitTimeout)
		defer cancel()

		resp := dalc.SubmitHeaders(ctx, blobs, maxBlobSize, -1)
		assert.Contains(resp.Message, ErrContextDeadline.Error(), "should return context timeout error")
	})
	t.Run("max_blob_size_error", func(t *testing.T) {
		mockDA := &mocks.DA{}
		dalc := NewDAClient(mockDA, -1, -1, nil, nil, log.NewTestLogger(t))
		// Set up the mock to return an error for MaxBlobSize
		mockDA.On("MaxBlobSize", mock.Anything).Return(uint64(0), errors.New("unable to get DA max blob size"))
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		assert := assert.New(t)
		_, err := dalc.MaxBlobSize(ctx)
		assert.ErrorContains(err, "unable to get DA max blob size", "should return max blob size error")
	})
	t.Run("tx_too_large", func(t *testing.T) {
		mockDA := &mocks.DA{}
		dalc := NewDAClient(mockDA, -1, -1, nil, nil, log.NewTestLogger(t))
		blobs := make([]coreda.Blob, 1)
		blobs[0] = make([]byte, 1234)
		// Set up the mock to throw tx too large
		mockDA.On("MaxBlobSize", mock.Anything).Return(uint64(1234), nil)
		mockDA.
			On("Submit", mock.Anything, blobs, float64(-1), []byte(nil)).
			Return([]coreda.ID{}, uint64(0), ErrTxTooLarge)

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		maxBlobSize, err := dalc.MaxBlobSize(ctx)
		require.NoError(t, err)

		assert := assert.New(t)

		resp := dalc.SubmitHeaders(ctx, blobs, maxBlobSize, -1)
		assert.Contains(resp.Message, ErrTxTooLarge.Error(), "should return tx too large error")
		assert.Equal(resp.Code, coreda.StatusTooBig)
	})
}

func TestSubmitRetrieve(t *testing.T) {
	t.Skip("skipping tests") //TODO: fix these tests
	dummyClient := NewDAClient(coreda.NewDummyDA(100_000), -1, -1, nil, nil, log.NewTestLogger(t))
	tests := []struct {
		name string
		f    func(t *testing.T, dalc coreda.Client)
	}{
		{"submit_retrieve", doTestSubmitRetrieve},
		{"submit_empty_blocks", doTestSubmitEmptyBlocks},
		{"submit_over_sized_block", doTestSubmitOversizedBlock},
		{"submit_small_blocks_batch", doTestSubmitSmallBlocksBatch},
		{"submit_large_blocks_overflow", doTestSubmitLargeBlocksOverflow}, //TODO: bring these back
		{"retrieve_no_blocks_found", doTestRetrieveNoBlocksFound},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.f(t, dummyClient)
		})
	}
}

func doTestSubmitRetrieve(t *testing.T, dalc coreda.Client) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require := require.New(t)
	assert := assert.New(t)

	const numBatches = 10
	const numHeaders = 10

	countAtHeight := make(map[uint64]int)

	maxBlobSize, err := dalc.MaxBlobSize(ctx)
	require.NoError(err)

	submitAndRecordHeaders := func(blobs []coreda.Blob) {
		for len(blobs) > 0 {
			resp := dalc.SubmitHeaders(ctx, blobs, maxBlobSize, -1)
			assert.Equal(coreda.StatusSuccess, resp.Code, resp.Message)

			countAtHeight[resp.DAHeight]++
			blobs = blobs[resp.SubmittedCount:]
		}
	}

	for batch := uint64(0); batch < numBatches; batch++ {
		headers := make([]coreda.Blob, numHeaders)
		for i := range headers {
			headers[i] = make([]byte, 1234)
		}
		submitAndRecordHeaders(headers)
		time.Sleep(time.Duration(rand.Int63() % MockDABlockTime.Milliseconds()))
	}

	validateBlockRetrieval := func(height uint64, expectedCount int) {
		t.Log("Retrieving block, DA Height", height)
		ret := dalc.RetrieveHeaders(ctx, height)
		assert.Equal(coreda.StatusSuccess, ret.Code, ret.Message)
		require.NotEmpty(ret.Headers, height)
		// assert.Len(ret.Headers, expectedCount, height) // TODO: fix this
	}

	for height, count := range countAtHeight {
		validateBlockRetrieval(height, count)
	}

}

func doTestSubmitEmptyBlocks(t *testing.T, dalc coreda.Client) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	maxBlobSize, err := dalc.MaxBlobSize(ctx)
	require.NoError(t, err)

	assert := assert.New(t)

	headersBz := make([]coreda.Blob, 2)
	headersBz[0] = make([]byte, 0)
	headersBz[1] = make([]byte, 0)
	resp := dalc.SubmitHeaders(ctx, headersBz, maxBlobSize, -1)
	assert.Equal(coreda.StatusSuccess, resp.Code, "empty blocks should submit")
	assert.EqualValues(resp.SubmittedCount, 2, "empty blocks should batch")
}

func doTestSubmitOversizedBlock(t *testing.T, dalc coreda.Client) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require := require.New(t)
	assert := assert.New(t)

	limit, err := dalc.MaxBlobSize(ctx)
	require.NoError(err)
	oversized := make([]coreda.Blob, 1)
	oversized[0] = make([]byte, limit+1)
	resp := dalc.SubmitHeaders(ctx, oversized, limit, -1)
	assert.Equal(coreda.StatusError, resp.Code, "oversized block should throw error")
	assert.Contains(resp.Message, "failed to submit blocks: no blobs generated blob: over size limit")
}

func doTestSubmitSmallBlocksBatch(t *testing.T, dalc coreda.Client) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	maxBlobSize, err := dalc.MaxBlobSize(ctx)
	require.NoError(t, err)

	assert := assert.New(t)

	headersBz := make([]coreda.Blob, 2)
	headersBz[0] = make([]byte, 100)
	headersBz[1] = make([]byte, 100)
	resp := dalc.SubmitHeaders(ctx, headersBz, maxBlobSize, -1)
	assert.Equal(coreda.StatusSuccess, resp.Code, "small blocks should submit")
	assert.EqualValues(resp.SubmittedCount, 2, "small blocks should batch")
}

func doTestSubmitLargeBlocksOverflow(t *testing.T, dalc coreda.Client) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require := require.New(t)
	assert := assert.New(t)

	limit, err := dalc.MaxBlobSize(ctx)
	require.NoError(err)

	// two large blocks, over blob limit to force partial submit
	var header1, header2 coreda.Blob
	for i := 0; ; i += 10 {
		header1 = make([]byte, i)
		blob1 := header1

		header2 = make([]byte, i)
		blob2 := header2

		if uint64(len(blob1)+len(blob2)) > limit {
			break
		}
	}

	// overflowing blocks submit partially
	resp := dalc.SubmitHeaders(ctx, []coreda.Blob{header1, header2}, limit, -1)
	assert.Equal(coreda.StatusSuccess, resp.Code, "overflowing blocks should submit partially")
	assert.EqualValues(1, resp.SubmittedCount, "submitted count should be partial")

	// retry remaining blocks
	resp = dalc.SubmitHeaders(ctx, []coreda.Blob{header2}, limit, -1)
	assert.Equal(coreda.StatusSuccess, resp.Code, "remaining blocks should submit")
	assert.EqualValues(resp.SubmittedCount, 1, "submitted count should match")
}

func doTestRetrieveNoBlocksFound(t *testing.T, dalc coreda.Client) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	result := dalc.RetrieveHeaders(ctx, 123)
	// Namespaces don't work on dummy da right now (https://github.com/rollkit/go-da/issues/94),
	// when namespaces are implemented, this should be uncommented
	// assert.Equal(StatusNotFound, result.Code)
	// assert.Contains(result.Message, ErrBlobNotFound.Error())
	assert.Equal(coreda.StatusError, result.Code)
}

func TestSubmitWithOptions(t *testing.T) {
	dummyClient := NewDAClient(coreda.NewDummyDA(100_000), -1, -1, nil, []byte("option=value"), log.NewTestLogger(t))
	tests := []struct {
		name string
		f    func(t *testing.T, dalc coreda.Client)
	}{
		{"submit_retrieve", doTestSubmitRetrieve},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.f(t, dummyClient)
		})
	}

}
