package da

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/cometbft/cometbft/libs/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/rollkit/go-da/proxy"
	goDATest "github.com/rollkit/go-da/test"
	"github.com/rollkit/rollkit/types"
	"google.golang.org/grpc/credentials/insecure"
)

const mockDaBlockTime = 100 * time.Millisecond

func TestMain(m *testing.M) {
	srv := startMockGRPCServ()
	if srv == nil {
		os.Exit(1)
	}
	exitCode := m.Run()

	// teardown servers
	srv.GracefulStop()

	os.Exit(exitCode)
}

func TestRetrieve(t *testing.T) {
	dummyClient := &DAClient{DA: goDATest.NewDummyDA(), Logger: log.TestingLogger()}
	grpcClient, err := startMockGRPCClient()
	require.NoError(t, err)
	clients := map[string]*DAClient{
		"dummy": dummyClient,
		"grpc":  grpcClient,
	}
	for name, dalc := range clients {
		t.Run(name, func(t *testing.T) {
			doTestRetrieve(t, dalc)
		})
	}
}

func startMockGRPCServ() *grpc.Server {
	srv := proxy.NewServer(goDATest.NewDummyDA(), grpc.Creds(insecure.NewCredentials()))
	lis, err := net.Listen("tcp", "127.0.0.1"+":"+strconv.Itoa(7980))
	if err != nil {
		fmt.Println(err)
		return nil
	}
	go func() {
		_ = srv.Serve(lis)
	}()
	return srv
}

func startMockGRPCClient() (*DAClient, error) {
	client := proxy.NewClient()
	lis, err := net.Listen("tcp", "127.0.0.1"+":"+strconv.Itoa(7980))
	if err != nil {
		return nil, err
	}
	err = client.Start(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &DAClient{DA: client, Logger: log.TestingLogger()}, nil
}

func doTestRetrieve(t *testing.T, dalc *da.DAClient) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require := require.New(t)
	assert := assert.New(t)

	// wait a bit more than mockDaBlockTime, so mock can "produce" some blocks
	time.Sleep(mockDaBlockTime + 20*time.Millisecond)

	retriever := dalc.(da.BlockRetriever)
	countAtHeight := make(map[uint64]int)
	blockToDAHeight := make(map[*types.Block]uint64)
	numBatches := uint64(10)
	blocksSubmittedPerBatch := 10

	for i := uint64(0); i < numBatches; i++ {
		blocks := make([]*types.Block, blocksSubmittedPerBatch)
		for j := 0; j < len(blocks); j++ {
			blocks[j] = types.GetRandomBlock(i*numBatches+uint64(j), rand.Int()%20) //nolint:gosec
		}
		resp := dalc.SubmitBlocks(ctx, blocks)
		assert.Equal(da.StatusSuccess, resp.Code, resp.Message)
		time.Sleep(time.Duration(rand.Int63() % mockDaBlockTime.Milliseconds())) //nolint:gosec

		for _, b := range blocks {
			blockToDAHeight[b] = resp.DAHeight
			countAtHeight[resp.DAHeight]++
		}
	}

	// wait a bit more than mockDaBlockTime, so mock can "produce" last blocks
	time.Sleep(mockDaBlockTime + 20*time.Millisecond)

	for h, cnt := range countAtHeight {
		t.Log("Retrieving block, DA Height", h)
		ret := retriever.RetrieveBlocks(ctx, h)
		assert.Equal(da.StatusSuccess, ret.Code, ret.Message)
		require.NotEmpty(ret.Blocks, h)
		assert.Equal(cnt%blocksSubmittedPerBatch, 0)
		assert.Len(ret.Blocks, cnt, h)
	}

	for b, h := range blockToDAHeight {
		ret := retriever.RetrieveBlocks(ctx, h)
		assert.Equal(da.StatusSuccess, ret.Code, h)
		require.NotEmpty(ret.Blocks, h)
		assert.Contains(ret.Blocks, b, h)
	}
}
