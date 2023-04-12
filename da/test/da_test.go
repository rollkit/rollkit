package test

import (
	"context"
	"encoding/json"
	"math/rand"
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	tmlog "github.com/tendermint/tendermint/libs/log"

	"github.com/rollkit/rollkit/da"
	"github.com/rollkit/rollkit/da/celestia"
	cmock "github.com/rollkit/rollkit/da/celestia/mock"
	grpcda "github.com/rollkit/rollkit/da/grpc"
	"github.com/rollkit/rollkit/da/grpc/mockserv"
	"github.com/rollkit/rollkit/da/mock"
	"github.com/rollkit/rollkit/da/registry"
	"github.com/rollkit/rollkit/log/test"
	"github.com/rollkit/rollkit/store"
	"github.com/rollkit/rollkit/types"
)

const mockDaBlockTime = 100 * time.Millisecond

var testHeaderNamespaceID = types.NamespaceID{0, 1, 2, 3, 4, 5, 6, 7}
var testDataNamespaceID = types.NamespaceID{7, 6, 5, 4, 3, 2, 1, 0}

type DAHeights struct {
	header uint64
	data   uint64
}

func TestLifecycle(t *testing.T) {
	srv := startMockGRPCServ(t)
	defer srv.GracefulStop()

	for _, dalc := range registry.RegisteredClients() {
		t.Run(dalc, func(t *testing.T) {
			doTestLifecycle(t, registry.GetClient(dalc))
		})
	}
}

func doTestLifecycle(t *testing.T, dalc da.DataAvailabilityLayerClient) {
	require := require.New(t)

	err := dalc.Init(testHeaderNamespaceID, testDataNamespaceID, []byte{}, nil, test.NewLogger(t))
	require.NoError(err)

	err = dalc.Start()
	require.NoError(err)

	err = dalc.Stop()
	require.NoError(err)
}

func TestDALC(t *testing.T) {
	grpcServer := startMockGRPCServ(t)
	defer grpcServer.GracefulStop()

	httpServer := startMockCelestiaNodeServer(t)
	defer httpServer.Stop()

	for _, dalc := range registry.RegisteredClients() {
		t.Run(dalc, func(t *testing.T) {
			doTestDALC(t, registry.GetClient(dalc))
		})
	}
}

func doTestDALC(t *testing.T, dalc da.DataAvailabilityLayerClient) {
	require := require.New(t)
	assert := assert.New(t)
	ctx := context.Background()

	// mock DALC will advance block height every 100ms
	conf := []byte{}
	if _, ok := dalc.(*mock.DataAvailabilityLayerClient); ok {
		conf = []byte(mockDaBlockTime.String())
	}
	if _, ok := dalc.(*celestia.DataAvailabilityLayerClient); ok {
		config := celestia.Config{
			BaseURL:  "http://localhost:26658",
			Timeout:  30 * time.Second,
			GasLimit: 3000000,
		}
		conf, _ = json.Marshal(config)
	}
	kvStore, _ := store.NewDefaultInMemoryKVStore()
	err := dalc.Init(testHeaderNamespaceID, testDataNamespaceID, conf, kvStore, test.NewLogger(t))
	require.NoError(err)

	err = dalc.Start()
	require.NoError(err)

	// wait a bit more than mockDaBlockTime, so mock can "produce" some blocks
	time.Sleep(mockDaBlockTime + 20*time.Millisecond)

	// only blocks b1 and b2 will be submitted to DA
	b1 := getRandomBlock(1, 10)
	b2 := getRandomBlock(2, 10)

	resp := dalc.SubmitBlockHeader(ctx, &b1.SignedHeader)
	h11 := resp.DAHeight
	assert.Equal(da.StatusSuccess, resp.Code)

	resp = dalc.SubmitBlockData(ctx, &b1.Data)
	h12 := resp.DAHeight
	assert.Equal(da.StatusSuccess, resp.Code)

	resp = dalc.SubmitBlockHeader(ctx, &b2.SignedHeader)
	h21 := resp.DAHeight
	assert.Equal(da.StatusSuccess, resp.Code)

	resp = dalc.SubmitBlockData(ctx, &b2.Data)
	h22 := resp.DAHeight
	assert.Equal(da.StatusSuccess, resp.Code)

	// wait a bit more than mockDaBlockTime, so Rollkit blocks can be "included" in mock block
	time.Sleep(mockDaBlockTime + 20*time.Millisecond)

	check := dalc.CheckBlockHeaderAvailability(ctx, h11)
	assert.Equal(da.StatusSuccess, check.Code)
	assert.True(check.DataAvailable)

	check = dalc.CheckBlockDataAvailability(ctx, h12)
	assert.Equal(da.StatusSuccess, check.Code)
	assert.True(check.DataAvailable)

	check = dalc.CheckBlockHeaderAvailability(ctx, h21)
	assert.Equal(da.StatusSuccess, check.Code)
	assert.True(check.DataAvailable)

	check = dalc.CheckBlockDataAvailability(ctx, h22)
	assert.Equal(da.StatusSuccess, check.Code)
	assert.True(check.DataAvailable)

	// this height should not be used by DALC
	check = dalc.CheckBlockHeaderAvailability(ctx, h11-1)
	assert.Equal(da.StatusSuccess, check.Code)
	assert.False(check.DataAvailable)

	check = dalc.CheckBlockDataAvailability(ctx, h12-1)
	assert.Equal(da.StatusSuccess, check.Code)
	assert.False(check.DataAvailable)
}

func TestRetrieve(t *testing.T) {
	grpcServer := startMockGRPCServ(t)
	defer grpcServer.GracefulStop()

	httpServer := startMockCelestiaNodeServer(t)
	defer httpServer.Stop()

	for _, client := range registry.RegisteredClients() {
		t.Run(client, func(t *testing.T) {
			dalc := registry.GetClient(client)
			_, ok := dalc.(da.BlockRetriever)
			if ok {
				doTestRetrieve(t, dalc)
			}
		})
	}
}

func startMockGRPCServ(t *testing.T) *grpc.Server {
	t.Helper()
	conf := grpcda.DefaultConfig
	logger := tmlog.NewTMLogger(os.Stdout)

	kvStore, _ := store.NewDefaultInMemoryKVStore()
	srv := mockserv.GetServer(kvStore, conf, []byte(mockDaBlockTime.String()), logger)
	lis, err := net.Listen("tcp", conf.Host+":"+strconv.Itoa(conf.Port))
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = srv.Serve(lis)
	}()
	return srv
}

func startMockCelestiaNodeServer(t *testing.T) *cmock.Server {
	t.Helper()
	httpSrv := cmock.NewServer(mockDaBlockTime, test.NewLogger(t))
	l, err := net.Listen("tcp4", "127.0.0.1:26658")
	if err != nil {
		t.Fatal("failed to create listener for mock celestia-node RPC server", "error", err)
	}
	err = httpSrv.Start(l)
	if err != nil {
		t.Fatal("can't start mock celestia-node RPC server")
	}
	return httpSrv
}

func doTestRetrieve(t *testing.T, dalc da.DataAvailabilityLayerClient) {
	ctx := context.Background()
	require := require.New(t)
	assert := assert.New(t)

	// mock DALC will advance block height every 100ms
	conf := []byte{}
	if _, ok := dalc.(*mock.DataAvailabilityLayerClient); ok {
		conf = []byte(mockDaBlockTime.String())
	}
	if _, ok := dalc.(*celestia.DataAvailabilityLayerClient); ok {
		config := celestia.Config{
			BaseURL:  "http://localhost:26658",
			Timeout:  30 * time.Second,
			GasLimit: 3000000,
		}
		conf, _ = json.Marshal(config)
	}
	kvStore, _ := store.NewDefaultInMemoryKVStore()
	err := dalc.Init(testHeaderNamespaceID, testDataNamespaceID, conf, kvStore, test.NewLogger(t))
	require.NoError(err)

	err = dalc.Start()
	require.NoError(err)

	// wait a bit more than mockDaBlockTime, so mock can "produce" some blocks
	time.Sleep(mockDaBlockTime + 20*time.Millisecond)

	retriever := dalc.(da.BlockRetriever)
	countHeadersAtHeight := make(map[uint64]int)
	countDatasAtHeight := make(map[uint64]int)
	blocks := make(map[*types.Block]DAHeights)

	for i := uint64(0); i < 100; i++ {
		b := getRandomBlock(i, rand.Int()%20) //nolint:gosec
		heights := DAHeights{}
		resp := dalc.SubmitBlockHeader(ctx, &b.SignedHeader)
		assert.Equal(da.StatusSuccess, resp.Code, resp.Message)
		countHeadersAtHeight[resp.DAHeight]++
		heights.header = resp.DAHeight
		resp = dalc.SubmitBlockData(ctx, &b.Data)
		assert.Equal(da.StatusSuccess, resp.Code, resp.Message)
		countDatasAtHeight[resp.DAHeight]++
		heights.data = resp.DAHeight
		// if the header and data are not at the same DA height
		if heights.header != heights.data {
			countHeadersAtHeight[resp.DAHeight]++
			countDatasAtHeight[heights.header]++
		}
		time.Sleep(time.Duration(rand.Int63() % mockDaBlockTime.Milliseconds())) //nolint:gosec
		blocks[b] = heights
	}

	// wait a bit more than mockDaBlockTime, so mock can "produce" last blocks
	time.Sleep(mockDaBlockTime + 20*time.Millisecond)

	for h, cnt := range countHeadersAtHeight {
		ret := retriever.RetrieveBlockHeaders(ctx, h)
		t.Log("Count of headers at DA Height", h, "should be", len(ret.Headers), "but it is", cnt)
		assert.Equal(da.StatusSuccess, ret.Code, ret.Message)
		require.NotEmpty(ret.Headers, h)
		assert.Len(ret.Headers, cnt, h)
	}
	for h, cnt := range countDatasAtHeight {
		ret := retriever.RetrieveBlockDatas(ctx, h)
		t.Log("Count of datas at DA Height", h, "should be", len(ret.Datas), "but it is", cnt)
		assert.Equal(da.StatusSuccess, ret.Code, ret.Message)
		require.NotEmpty(ret.Datas, h)
		assert.Len(ret.Datas, cnt, h)
	}

	for b, heights := range blocks {
		ret := retriever.RetrieveBlockHeaders(ctx, heights.header)
		assert.Equal(da.StatusSuccess, ret.Code, heights.header)
		require.NotEmpty(ret.Headers, heights.header)
		assert.Contains(ret.Headers, &b.SignedHeader, heights.header)

		ret1 := retriever.RetrieveBlockDatas(ctx, heights.data)
		assert.Equal(da.StatusSuccess, ret1.Code, heights.data)
		require.NotEmpty(ret1.Datas, heights.data)
		assert.Contains(ret1.Datas, &b.Data, heights.data)
	}
}

// copy-pasted from store/store_test.go
func getRandomBlock(height uint64, nTxs int) *types.Block {
	block := &types.Block{
		SignedHeader: types.SignedHeader{
			Header: types.Header{
				BaseHeader: types.BaseHeader{
					Height: height,
				},
				AggregatorsHash: make([]byte, 32),
			}},
		Data: types.Data{
			Txs: make(types.Txs, nTxs),
			IntermediateStateRoots: types.IntermediateStateRoots{
				RawRootsList: make([][]byte, nTxs),
			},
		},
	}
	block.SignedHeader.Header.AppHash = getRandomBytes(32)

	for i := 0; i < nTxs; i++ {
		block.Data.Txs[i] = getRandomTx()
		block.Data.IntermediateStateRoots.RawRootsList[i] = getRandomBytes(32)
	}

	// TODO(tzdybal): see https://github.com/rollkit/rollkit/issues/143
	if nTxs == 0 {
		block.Data.Txs = nil
		block.Data.IntermediateStateRoots.RawRootsList = nil
	}

	return block
}

func getRandomTx() types.Tx {
	size := rand.Int()%100 + 100 //nolint:gosec
	return types.Tx(getRandomBytes(size))
}

func getRandomBytes(n int) []byte {
	data := make([]byte, n)
	_, _ = rand.Read(data) //nolint:gosec
	return data
}
