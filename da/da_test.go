package da

import (
	"context"
	"errors"
	"math/rand"
	"testing"
	"time"

	"cosmossdk.io/log"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	coreda "github.com/rollkit/rollkit/core/da"
	"github.com/rollkit/rollkit/da/mocks"
	"github.com/rollkit/rollkit/types"
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
	chainID := "TestMockDAErrors"
	t.Run("submit_timeout", func(t *testing.T) {
		mockDA := &mocks.DA{}
		dalc := NewDAClient(mockDA, 0, 0, nil, log.NewTestLogger(t), nil)
		header, _ := types.GetRandomBlock(1, 0, chainID)
		headers := []*types.SignedHeader{header}
		var blobs []coreda.Blob
		for _, header := range headers {
			headerBytes, err := header.MarshalBinary()
			require.NoError(t, err)
			blobs = append(blobs, headerBytes)
		}
		// Set up the mock to throw context deadline exceeded
		mockDA.On("MaxBlobSize", mock.Anything).Return(uint64(1234), nil)
		mockDA.
			On("Submit", mock.Anything, blobs, float64(-1), []byte(nil)).
			After(submitTimeout).
			Return(nil, &ErrContextDeadline{})

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		maxBlobSize, err := dalc.DA.MaxBlobSize(ctx)
		require.NoError(t, err)

		dalc.SubmitTimeout = submitTimeout
		resp := dalc.SubmitHeaders(ctx, headers, maxBlobSize, -1)
		require.Contains(t, resp.Message, (&ErrContextDeadline{}).Error(), "should return context timeout error")

	})
	t.Run("max_blob_size_error", func(t *testing.T) {
		mockDA := &mocks.DA{}
		dalc := NewDAClient(mockDA, 0, 0, nil, log.NewTestLogger(t), nil)
		// Set up the mock to return an error for MaxBlobSize
		mockDA.On("MaxBlobSize", mock.Anything).Return(uint64(0), errors.New("unable to get DA max blob size"))

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		_, err := dalc.DA.MaxBlobSize(ctx)
		require.ErrorContains(t, err, "unable to get DA max blob size", "should return max blob size error")

	})
	t.Run("tx_too_large", func(t *testing.T) {
		mockDA := &mocks.DA{}
		dalc := NewDAClient(mockDA, 0, 0, nil, log.NewTestLogger(t), nil)
		header, _ := types.GetRandomBlock(1, 0, chainID)
		headers := []*types.SignedHeader{header}
		var blobs []coreda.Blob
		for _, header := range headers {
			headerBytes, err := header.MarshalBinary()
			require.NoError(t, err)
			blobs = append(blobs, headerBytes)
		}
		// Set up the mock to throw tx too large
		mockDA.On("MaxBlobSize", mock.Anything).Return(uint64(1234), nil)
		mockDA.
			On("Submit", mock.Anything, blobs, float64(-1), []byte(nil)).
			After(submitTimeout).
			Return(nil, &ErrTxTooLarge{})

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		maxBlobSize, err := dalc.DA.MaxBlobSize(ctx)
		require.NoError(t, err)
		require.New(t)
		resp := dalc.SubmitHeaders(ctx, headers, maxBlobSize, -1)
		require.Contains(t, resp.Message, (&ErrTxTooLarge{}).Error(), "should return tx too large error")
		require.Equal(t, resp.Code, StatusTooBig)
	})
}

func TestSubmitRetrieve(t *testing.T) {
	dummyClient := NewDAClient(coreda.NewDummyDA(100_000), 0, 0, nil, log.NewTestLogger(t), nil)
	tests := []struct {
		name string
		f    func(t *testing.T, dalc *DAClient)
	}{
		{"submit_retrieve", doTestSubmitRetrieve},
		{"submit_empty_blocks", doTestSubmitEmptyBlocks},
		// {"submit_over_sized_block", doTestSubmitOversizedBlock},
		{"submit_small_blocks_batch", doTestSubmitSmallBlocksBatch},
		// {"submit_large_blocks_overflow", doTestSubmitLargeBlocksOverflow},
		{"retrieve_no_blocks_found", doTestRetrieveNoBlocksFound},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.f(t, dummyClient)
		})
	}
}

func doTestSubmitRetrieve(t *testing.T, dalc *DAClient) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.New(t)

	const numBatches = 10
	const numHeaders = 10

	headerToDAHeight := make(map[*types.SignedHeader]uint64)
	countAtHeight := make(map[uint64]int)

	maxBlobSize, err := dalc.DA.MaxBlobSize(ctx)
	require.NoError(t, err)

	submitAndRecordHeaders := func(headers []*types.SignedHeader) {
		for len(headers) > 0 {
			resp := dalc.SubmitHeaders(ctx, headers, maxBlobSize, 0)
			require.Equal(t, StatusSuccess, resp.Code, resp.Message)

			for _, block := range headers[:resp.SubmittedCount] {
				headerToDAHeight[block] = resp.DAHeight
				countAtHeight[resp.DAHeight]++
			}
			headers = headers[resp.SubmittedCount:]
		}
	}

	for batch := uint64(0); batch < numBatches; batch++ {
		headers := make([]*types.SignedHeader, numHeaders)
		for i := range headers {
			headers[i], _ = types.GetRandomBlock(batch*numBatches+uint64(i), rand.Int()%20, "doTestSubmitRetrieve") //nolint:gosec
		}
		submitAndRecordHeaders(headers)
		time.Sleep(time.Duration(rand.Int63() % MockDABlockTime.Milliseconds())) //nolint:gosec
	}

	validateBlockRetrieval := func(height uint64, expectedCount int) {
		t.Log("Retrieving block, DA Height", height)
		ret := dalc.RetrieveHeaders(ctx, height)
		require.Equal(t, StatusSuccess, ret.Code, ret.Message)
		require.NotEmpty(t, ret.Headers, height)
		require.Len(t, ret.Headers, expectedCount, height)
	}

	for height, count := range countAtHeight {
		validateBlockRetrieval(height, count)
	}

	for header, height := range headerToDAHeight {
		ret := dalc.RetrieveHeaders(ctx, height)
		require.Equal(t, StatusSuccess, ret.Code, height)
		require.NotEmpty(t, ret.Headers, height)
		require.Contains(t, ret.Headers, header, height)
	}
}

func doTestSubmitEmptyBlocks(t *testing.T, dalc *DAClient) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	maxBlobSize, err := dalc.DA.MaxBlobSize(ctx)
	require.NoError(t, err)
	require.New(t)

	chainID := "doTestSubmitEmptyBlocks"
	header1, _ := types.GetRandomBlock(1, 0, chainID)
	header2, _ := types.GetRandomBlock(1, 0, chainID)
	resp := dalc.SubmitHeaders(ctx, []*types.SignedHeader{header1, header2}, maxBlobSize, 0)
	require.Equal(t, StatusSuccess, resp.Code, "empty blocks should submit")
	require.EqualValues(t, resp.SubmittedCount, 2, "empty blocks should batch")
}

// func doTestSubmitOversizedBlock(t *testing.T, dalc *DAClient) {
// 	ctx, cancel := context.WithCancel(context.Background())
// 	defer cancel()

// 	require := require.New(t)
// 	assert := assert.New(t)

// 	limit, err := dalc.DA.MaxBlobSize(ctx)
// 	require.NoError(err)
// 	oversizedHeader, _ := types.GetRandomBlock(1, int(limit)) //nolint:gosec
// 	resp := dalc.SubmitHeaders(ctx, []*types.SignedHeader{oversizedHeader}, limit, -1)
// 	assert.Equal(StatusError, resp.Code, "oversized block should throw error")
// 	assert.Contains(resp.Message, "failed to submit blocks: no blobs generated blob: over size limit")
// }

func doTestSubmitSmallBlocksBatch(t *testing.T, dalc *DAClient) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	maxBlobSize, err := dalc.DA.MaxBlobSize(ctx)
	require.NoError(t, err)

	require.New(t)

	chainID := "doTestSubmitSmallBlocksBatch"
	header1, _ := types.GetRandomBlock(1, 1, chainID)
	header2, _ := types.GetRandomBlock(1, 2, chainID)
	resp := dalc.SubmitHeaders(ctx, []*types.SignedHeader{header1, header2}, maxBlobSize, 0)
	require.Equal(t, StatusSuccess, resp.Code, "small blocks should submit")
	require.EqualValues(t, resp.SubmittedCount, 2, "small blocks should batch")
}

// func doTestSubmitLargeBlocksOverflow(t *testing.T, dalc *DAClient) {
// 	ctx, cancel := context.WithCancel(context.Background())
// 	defer cancel()

// 	require := require.New(t)
// 	assert := assert.New(t)

// 	limit, err := dalc.DA.MaxBlobSize(ctx)
// 	require.NoError(err)

// 	// two large blocks, over blob limit to force partial submit
// 	var header1, header2 *types.SignedHeader
// 	for i := 0; ; i += 10 {
// 		header1, _ = types.GetRandomBlock(1, i)
// 		blob1, err := header1.MarshalBinary()
// 		require.NoError(err)

// 		header2, _ = types.GetRandomBlock(1, i)
// 		blob2, err := header2.MarshalBinary()
// 		require.NoError(err)

// 		if uint64(len(blob1)+len(blob2)) > limit {
// 			break
// 		}
// 	}

// 	// overflowing blocks submit partially
// 	resp := dalc.SubmitHeaders(ctx, []*types.SignedHeader{header1, header2}, limit, -1)
// 	assert.Equal(StatusSuccess, resp.Code, "overflowing blocks should submit partially")
// 	assert.EqualValues(1, resp.SubmittedCount, "submitted count should be partial")

// 	// retry remaining blocks
// 	resp = dalc.SubmitHeaders(ctx, []*types.SignedHeader{header2}, limit, -1)
// 	assert.Equal(StatusSuccess, resp.Code, "remaining blocks should submit")
// 	assert.EqualValues(resp.SubmittedCount, 1, "submitted count should match")
// }

func doTestRetrieveNoBlocksFound(t *testing.T, dalc *DAClient) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.New(t)
	result := dalc.RetrieveHeaders(ctx, 123)
	// Namespaces don't work on dummy da right now (https://github.com/rollkit/go-da/issues/94),
	// when namespaces are implemented, this should be uncommented
	// require.Equal(t, StatusNotFound, result.Code)
	// require.Contains(t, result.Message, ErrBlobNotFound.Error())
	require.Equal(t, StatusError, result.Code)
}

func TestSubmitWithOptions(t *testing.T) {
	dummyClient := NewDAClient(coreda.NewDummyDA(100_000), 0, 0, nil, log.NewTestLogger(t), []byte("option=value"))
	tests := []struct {
		name string
		f    func(t *testing.T, dalc *DAClient)
	}{
		{"submit_retrieve", doTestSubmitRetrieve},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.f(t, dummyClient)
		})
	}
}
