package da

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/cometbft/cometbft/libs/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rollkit/go-da"
	"github.com/rollkit/go-da/proxy-jsonrpc"
	goDATest "github.com/rollkit/go-da/test"
	"github.com/rollkit/rollkit/da/mock"
	"github.com/rollkit/rollkit/types"
)

const mockDaBlockTime = 100 * time.Millisecond

func TestMain(m *testing.M) {
	srv := startMockDAServ()
	if srv == nil {
		os.Exit(1)
	}
	exitCode := m.Run()

	// teardown servers
	err := srv.Stop(context.TODO())
	if err != nil {
		fmt.Println(err)
	}

	os.Exit(exitCode)
}

func TestMockDAErrors(t *testing.T) {
	t.Run("submit_timeout", func(t *testing.T) {
		mockDA := &mock.MockDA{}
		dalc := &DAClient{DA: mockDA, GasPrice: -1, GasMultiplier: -1, Logger: log.TestingLogger()}
		blocks := []*types.Block{types.GetRandomBlock(1, 0)}
		var blobs []da.Blob
		for _, block := range blocks {
			blockBytes, err := block.MarshalBinary()
			require.NoError(t, err)
			blobs = append(blobs, blockBytes)
		}
		// Set up the mock to throw context deadline exceeded
		mockDA.On("MaxBlobSize").Return(uint64(1234), nil)
		mockDA.
			On("Submit", blobs, float64(-1), []byte(nil)).
			After(100*time.Millisecond).
			Return([]da.ID{bytes.Repeat([]byte{0x00}, 8)}, nil)
		doTestSubmitTimeout(t, dalc, blocks)
	})
	t.Run("max_blob_size_error", func(t *testing.T) {
		mockDA := &mock.MockDA{}
		dalc := &DAClient{DA: mockDA, GasPrice: -1, GasMultiplier: -1, Logger: log.TestingLogger()}
		// Set up the mock to return an error for MaxBlobSize
		mockDA.On("MaxBlobSize").Return(uint64(0), errors.New("unable to get DA max blob size"))
		doTestMaxBlockSizeError(t, dalc)
	})
}

func TestSubmitRetrieve(t *testing.T) {
	dummyClient := &DAClient{DA: goDATest.NewDummyDA(), GasPrice: -1, Logger: log.TestingLogger()}
	rpcClient, err := startMockDAClient()
	require.NoError(t, err)
	clients := map[string]*DAClient{
		"dummy": dummyClient,
		"rpc":   rpcClient,
	}
	tests := []struct {
		name string
		f    func(t *testing.T, dalc *DAClient)
	}{
		{"submit_retrieve", doTestSubmitRetrieve},
		{"submit_empty_blocks", doTestSubmitEmptyBlocks},
		{"submit_over_sized_block", doTestSubmitOversizedBlock},
		{"submit_small_blocks_batch", doTestSubmitSmallBlocksBatch},
		{"submit_large_blocks_overflow", doTestSubmitLargeBlocksOverflow},
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

func startMockDAServ() *proxy.Server {
	srv := proxy.NewServer("localhost", "7980", goDATest.NewDummyDA())
	err := srv.Start(context.TODO())
	if err != nil {
		fmt.Println(err)
		return nil
	}
	return srv
}

func startMockDAClient() (*DAClient, error) {
	client, err := proxy.NewClient(context.TODO(), "http://localhost:7980", "")
	if err != nil {
		return nil, err
	}
	return &DAClient{DA: &client.DA, GasPrice: -1, GasMultiplier: -1, Logger: log.TestingLogger()}, nil
}

func doTestSubmitTimeout(t *testing.T, dalc *DAClient, blocks []*types.Block) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	maxBlobSize, err := dalc.DA.MaxBlobSize(ctx)
	require.NoError(t, err)

	assert := assert.New(t)
	submitTimeout = 50 * time.Millisecond
	resp := dalc.SubmitBlocks(ctx, blocks, maxBlobSize, -1)
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
	const numBlocks = 10

	blockToDAHeight := make(map[*types.Block]uint64)
	countAtHeight := make(map[uint64]int)

	maxBlobSize, err := dalc.DA.MaxBlobSize(ctx)
	require.NoError(err)

	submitAndRecordBlocks := func(blocks []*types.Block) {
		for len(blocks) > 0 {
			resp := dalc.SubmitBlocks(ctx, blocks, maxBlobSize, -1)
			assert.Equal(StatusSuccess, resp.Code, resp.Message)

			for _, block := range blocks[:resp.SubmittedCount] {
				blockToDAHeight[block] = resp.DAHeight
				countAtHeight[resp.DAHeight]++
			}
			blocks = blocks[resp.SubmittedCount:]
		}
	}

	for batch := uint64(0); batch < numBatches; batch++ {
		blocks := make([]*types.Block, numBlocks)
		for i := range blocks {
			blocks[i] = types.GetRandomBlock(batch*numBatches+uint64(i), rand.Int()%20) //nolint:gosec
		}
		submitAndRecordBlocks(blocks)
		time.Sleep(time.Duration(rand.Int63() % mockDaBlockTime.Milliseconds())) //nolint:gosec
	}

	validateBlockRetrieval := func(height uint64, expectedCount int) {
		t.Log("Retrieving block, DA Height", height)
		ret := dalc.RetrieveBlocks(ctx, height)
		assert.Equal(StatusSuccess, ret.Code, ret.Message)
		require.NotEmpty(ret.Blocks, height)
		assert.Len(ret.Blocks, expectedCount, height)
	}

	for height, count := range countAtHeight {
		validateBlockRetrieval(height, count)
	}

	for block, height := range blockToDAHeight {
		ret := dalc.RetrieveBlocks(ctx, height)
		assert.Equal(StatusSuccess, ret.Code, height)
		require.NotEmpty(ret.Blocks, height)
		assert.Contains(ret.Blocks, block, height)
	}
}

func doTestSubmitEmptyBlocks(t *testing.T, dalc *DAClient) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	maxBlobSize, err := dalc.DA.MaxBlobSize(ctx)
	require.NoError(t, err)

	assert := assert.New(t)

	block1 := types.GetRandomBlock(1, 0)
	block2 := types.GetRandomBlock(1, 0)
	resp := dalc.SubmitBlocks(ctx, []*types.Block{block1, block2}, maxBlobSize, -1)
	assert.Equal(StatusSuccess, resp.Code, "empty blocks should submit")
	assert.EqualValues(resp.SubmittedCount, 2, "empty blocks should batch")
}

func doTestSubmitOversizedBlock(t *testing.T, dalc *DAClient) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require := require.New(t)
	assert := assert.New(t)

	limit, err := dalc.DA.MaxBlobSize(ctx)
	require.NoError(err)
	oversizedBlock := types.GetRandomBlock(1, int(limit))
	resp := dalc.SubmitBlocks(ctx, []*types.Block{oversizedBlock}, limit, -1)
	assert.Equal(StatusError, resp.Code, "oversized block should throw error")
	assert.Contains(resp.Message, "failed to submit blocks: oversized block: blob: over size limit")
}

func doTestSubmitSmallBlocksBatch(t *testing.T, dalc *DAClient) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	maxBlobSize, err := dalc.DA.MaxBlobSize(ctx)
	require.NoError(t, err)

	assert := assert.New(t)

	block1 := types.GetRandomBlock(1, 1)
	block2 := types.GetRandomBlock(1, 2)
	resp := dalc.SubmitBlocks(ctx, []*types.Block{block1, block2}, maxBlobSize, -1)
	assert.Equal(StatusSuccess, resp.Code, "small blocks should submit")
	assert.EqualValues(resp.SubmittedCount, 2, "small blocks should batch")
}

func doTestSubmitLargeBlocksOverflow(t *testing.T, dalc *DAClient) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require := require.New(t)
	assert := assert.New(t)

	limit, err := dalc.DA.MaxBlobSize(ctx)
	require.NoError(err)

	// two large blocks, over blob limit to force partial submit
	var block1, block2 *types.Block
	for i := 0; ; i += 10 {
		block1 = types.GetRandomBlock(1, i)
		blob1, err := block1.MarshalBinary()
		require.NoError(err)

		block2 = types.GetRandomBlock(1, i)
		blob2, err := block2.MarshalBinary()
		require.NoError(err)

		if uint64(len(blob1)+len(blob2)) > limit {
			break
		}
	}

	// overflowing blocks submit partially
	resp := dalc.SubmitBlocks(ctx, []*types.Block{block1, block2}, limit, -1)
	assert.Equal(StatusSuccess, resp.Code, "overflowing blocks should submit partially")
	assert.EqualValues(1, resp.SubmittedCount, "submitted count should be partial")

	// retry remaining blocks
	resp = dalc.SubmitBlocks(ctx, []*types.Block{block2}, limit, -1)
	assert.Equal(StatusSuccess, resp.Code, "remaining blocks should submit")
	assert.EqualValues(resp.SubmittedCount, 1, "submitted count should match")
}

func doTestRetrieveNoBlocksFound(t *testing.T, dalc *DAClient) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	result := dalc.RetrieveBlocks(ctx, 123)
	assert.Equal(StatusNotFound, result.Code)
	assert.Contains(result.Message, ErrBlobNotFound.Error())
}
