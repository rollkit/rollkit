package da

import (
	"context"
	"errors"
	"math/rand"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/cometbft/cometbft/libs/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/rollkit/go-da"
	damock "github.com/rollkit/go-da/mocks"
	proxygrpc "github.com/rollkit/go-da/proxy/grpc"
	proxyjsonrpc "github.com/rollkit/go-da/proxy/jsonrpc"
	goDATest "github.com/rollkit/go-da/test"
	testServer "github.com/rollkit/rollkit/test/server"
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

// TestMain starts the mock gRPC and JSONRPC DA services
// gRPC service listens on MockDAAddress
// JSONRPC service listens on MockDAAddressHTTP
// Ports were chosen to be sufficiently different from defaults (26650, 26658)
// Static ports are used to keep client configuration simple
// NOTE: this should be unique per test package to avoid
// "bind: listen address already in use" because multiple packages
// are tested in parallel
func TestMain(m *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	jsonrpcSrv := testServer.StartMockDAServJSONRPC(ctx, MockDAAddressHTTP)
	if jsonrpcSrv == nil {
		os.Exit(1)
	}
	grpcSrv := testServer.StartMockDAServGRPC(MockDAAddress)
	exitCode := m.Run()

	// teardown servers
	// nolint:errcheck,gosec
	jsonrpcSrv.Stop(context.Background())
	grpcSrv.Stop()

	os.Exit(exitCode)
}

func TestMockDAErrors(t *testing.T) {
	t.Run("submit_timeout", func(t *testing.T) {
		mockDA := &damock.MockDA{}
		dalc := NewDAClient(mockDA, -1, -1, nil, nil, log.TestingLogger())
		header, _ := types.GetRandomBlock(1, 0)
		headers := []*types.SignedHeader{header}
		var blobs []da.Blob
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
			Return(nil, context.DeadlineExceeded)
		doTestSubmitTimeout(t, dalc, headers)
	})
	t.Run("max_blob_size_error", func(t *testing.T) {
		mockDA := &damock.MockDA{}
		dalc := NewDAClient(mockDA, -1, -1, nil, nil, log.TestingLogger())
		// Set up the mock to return an error for MaxBlobSize
		mockDA.On("MaxBlobSize", mock.Anything).Return(uint64(0), errors.New("unable to get DA max blob size"))
		doTestMaxBlockSizeError(t, dalc)
	})
	t.Run("tx_too_large", func(t *testing.T) {
		mockDA := &damock.MockDA{}
		dalc := NewDAClient(mockDA, -1, -1, nil, nil, log.TestingLogger())
		header, _ := types.GetRandomBlock(1, 0)
		headers := []*types.SignedHeader{header}
		var blobs []da.Blob
		for _, header := range headers {
			headerBytes, err := header.MarshalBinary()
			require.NoError(t, err)
			blobs = append(blobs, headerBytes)
		}
		// Set up the mock to throw tx too large
		mockDA.On("MaxBlobSize", mock.Anything).Return(uint64(1234), nil)
		mockDA.
			On("Submit", mock.Anything, blobs, float64(-1), []byte(nil)).
			Return([]da.ID{}, errors.New("tx too large"))
		doTestTxTooLargeError(t, dalc, headers)
	})
}

func TestSubmitRetrieve(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	dummyClient := NewDAClient(goDATest.NewDummyDA(), -1, -1, nil, nil, log.TestingLogger())
	jsonrpcClient, err := startMockDAClientJSONRPC(ctx)
	require.NoError(t, err)
	grpcClient := startMockDAClientGRPC()
	require.NoError(t, err)
	clients := map[string]*DAClient{
		"dummy":   dummyClient,
		"jsonrpc": jsonrpcClient,
		"grpc":    grpcClient,
	}
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
	for name, dalc := range clients {
		for _, tc := range tests {
			t.Run(name+"_"+tc.name, func(t *testing.T) {
				tc.f(t, dalc)
			})
		}
	}
}

func startMockDAClientGRPC() *DAClient {
	client := proxygrpc.NewClient()
	addr, _ := url.Parse(MockDAAddress)
	if err := client.Start(addr.Host, grpc.WithTransportCredentials(insecure.NewCredentials())); err != nil {
		panic(err)
	}
	return NewDAClient(client, -1, -1, nil, nil, log.TestingLogger())
}

func startMockDAClientJSONRPC(ctx context.Context) (*DAClient, error) {
	client, err := proxyjsonrpc.NewClient(ctx, MockDAAddressHTTP, "")
	if err != nil {
		return nil, err
	}
	return NewDAClient(&client.DA, -1, -1, nil, nil, log.TestingLogger()), nil
}

func doTestSubmitTimeout(t *testing.T, dalc *DAClient, headers []*types.SignedHeader) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	maxBlobSize, err := dalc.DA.MaxBlobSize(ctx)
	require.NoError(t, err)

	assert := assert.New(t)
	dalc.SubmitTimeout = submitTimeout
	resp := dalc.SubmitHeaders(ctx, headers, maxBlobSize, -1)
	assert.Contains(resp.Message, "context deadline exceeded", "should return context timeout error")
}

func doTestMaxBlockSizeError(t *testing.T, dalc *DAClient) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	_, err := dalc.DA.MaxBlobSize(ctx)
	assert.ErrorContains(err, "unable to get DA max blob size", "should return max blob size error")
}

func doTestSubmitRetrieve(t *testing.T, dalc *DAClient) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require := require.New(t)
	assert := assert.New(t)

	const numBatches = 10
	const numHeaders = 10

	headerToDAHeight := make(map[*types.SignedHeader]uint64)
	countAtHeight := make(map[uint64]int)

	maxBlobSize, err := dalc.DA.MaxBlobSize(ctx)
	require.NoError(err)

	submitAndRecordHeaders := func(headers []*types.SignedHeader) {
		for len(headers) > 0 {
			resp := dalc.SubmitHeaders(ctx, headers, maxBlobSize, -1)
			assert.Equal(StatusSuccess, resp.Code, resp.Message)

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
			headers[i], _ = types.GetRandomBlock(batch*numBatches+uint64(i), rand.Int()%20) //nolint:gosec
		}
		submitAndRecordHeaders(headers)
		time.Sleep(time.Duration(rand.Int63() % MockDABlockTime.Milliseconds())) //nolint:gosec
	}

	validateBlockRetrieval := func(height uint64, expectedCount int) {
		t.Log("Retrieving block, DA Height", height)
		ret := dalc.RetrieveHeaders(ctx, height)
		assert.Equal(StatusSuccess, ret.Code, ret.Message)
		require.NotEmpty(ret.Headers, height)
		assert.Len(ret.Headers, expectedCount, height)
	}

	for height, count := range countAtHeight {
		validateBlockRetrieval(height, count)
	}

	for header, height := range headerToDAHeight {
		ret := dalc.RetrieveHeaders(ctx, height)
		assert.Equal(StatusSuccess, ret.Code, height)
		require.NotEmpty(ret.Headers, height)
		assert.Contains(ret.Headers, header, height)
	}
}

func doTestTxTooLargeError(t *testing.T, dalc *DAClient, headers []*types.SignedHeader) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	maxBlobSize, err := dalc.DA.MaxBlobSize(ctx)
	require.NoError(t, err)

	assert := assert.New(t)
	resp := dalc.SubmitHeaders(ctx, headers, maxBlobSize, -1)
	assert.Contains(resp.Message, "tx too large", "should return tx too large error")
	assert.Equal(resp.Code, StatusTooBig)
}

func doTestSubmitEmptyBlocks(t *testing.T, dalc *DAClient) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	maxBlobSize, err := dalc.DA.MaxBlobSize(ctx)
	require.NoError(t, err)

	assert := assert.New(t)

	header1, _ := types.GetRandomBlock(1, 0)
	header2, _ := types.GetRandomBlock(1, 0)
	resp := dalc.SubmitHeaders(ctx, []*types.SignedHeader{header1, header2}, maxBlobSize, -1)
	assert.Equal(StatusSuccess, resp.Code, "empty blocks should submit")
	assert.EqualValues(resp.SubmittedCount, 2, "empty blocks should batch")
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

	assert := assert.New(t)

	header1, _ := types.GetRandomBlock(1, 1)
	header2, _ := types.GetRandomBlock(1, 2)
	resp := dalc.SubmitHeaders(ctx, []*types.SignedHeader{header1, header2}, maxBlobSize, -1)
	assert.Equal(StatusSuccess, resp.Code, "small blocks should submit")
	assert.EqualValues(resp.SubmittedCount, 2, "small blocks should batch")
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

	assert := assert.New(t)
	result := dalc.RetrieveHeaders(ctx, 123)
	// Namespaces don't work on dummy da right now (https://github.com/rollkit/go-da/issues/94),
	// when namespaces are implemented, this should be uncommented
	// assert.Equal(StatusNotFound, result.Code)
	// assert.Contains(result.Message, ErrBlobNotFound.Error())
	assert.Equal(StatusError, result.Code)
}

func TestSubmitWithOptions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	dummyClient := NewDAClient(goDATest.NewDummyDA(), -1, -1, nil, []byte("option=value"), log.TestingLogger())
	jsonrpcClient, err := startMockDAClientJSONRPC(ctx)
	require.NoError(t, err)
	grpcClient := startMockDAClientGRPC()
	require.NoError(t, err)
	clients := map[string]*DAClient{
		"dummy":   dummyClient,
		"jsonrpc": jsonrpcClient,
		"grpc":    grpcClient,
	}
	tests := []struct {
		name string
		f    func(t *testing.T, dalc *DAClient)
	}{
		{"submit_retrieve", doTestSubmitRetrieve},
	}
	for name, dalc := range clients {
		for _, tc := range tests {
			t.Run(name+"_"+tc.name, func(t *testing.T) {
				tc.f(t, dalc)
			})
		}
	}
}
