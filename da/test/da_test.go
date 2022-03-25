package test

import (
	"math/rand"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/celestiaorg/optimint/da"
	grpcda "github.com/celestiaorg/optimint/da/grpc"
	"github.com/celestiaorg/optimint/da/grpc/mockserv"
	"github.com/celestiaorg/optimint/da/mock"
	"github.com/celestiaorg/optimint/da/registry"
	"github.com/celestiaorg/optimint/log/test"
	"github.com/celestiaorg/optimint/store"
	"github.com/celestiaorg/optimint/types"
)

const mockDaBlockTime = 100 * time.Millisecond

func TestLifecycle(t *testing.T) {
	srv := startMockServ(t)
	defer srv.GracefulStop()
	for _, dalc := range registry.RegisteredClients() {
		t.Run(dalc, func(t *testing.T) {
			doTestLifecycle(t, registry.GetClient(dalc))
		})
	}
}

func doTestLifecycle(t *testing.T, dalc da.DataAvailabilityLayerClient) {
	require := require.New(t)

	err := dalc.Init([]byte{}, nil, &test.TestLogger{T: t})
	require.NoError(err)

	err = dalc.Start()
	require.NoError(err)

	err = dalc.Stop()
	require.NoError(err)
}

func TestDALC(t *testing.T) {
	srv := startMockServ(t)
	defer srv.GracefulStop()
	for _, dalc := range registry.RegisteredClients() {
		t.Run(dalc, func(t *testing.T) {
			doTestDALC(t, registry.GetClient(dalc))
		})
	}
}

func doTestDALC(t *testing.T, dalc da.DataAvailabilityLayerClient) {
	require := require.New(t)
	assert := assert.New(t)

	// mock DALC will advance block height every 100ms
	conf := []byte{}
	if _, ok := dalc.(*mock.MockDataAvailabilityLayerClient); ok {
		conf = []byte(mockDaBlockTime.String())
	}
	err := dalc.Init(conf, store.NewDefaultInMemoryKVStore(), &test.TestLogger{T: t})
	require.NoError(err)

	err = dalc.Start()
	require.NoError(err)

	// wait a bit more than mockDaBlockTime, so mock can "produce" some blocks
	time.Sleep(mockDaBlockTime + 20*time.Millisecond)

	// only blocks b1 and b2 will be submitted to DA
	b1 := getRandomBlock(1, 10)
	b2 := getRandomBlock(2, 10)

	resp := dalc.SubmitBlock(b1)
	h1 := resp.DataLayerHeight
	assert.Equal(da.StatusSuccess, resp.Code)

	resp = dalc.SubmitBlock(b2)
	h2 := resp.DataLayerHeight
	assert.Equal(da.StatusSuccess, resp.Code)

	// wait a bit more than mockDaBlockTime, so optimint blocks can be "included" in mock block
	time.Sleep(mockDaBlockTime + 20*time.Millisecond)

	check := dalc.CheckBlockAvailability(h1)
	assert.Equal(da.StatusSuccess, check.Code)
	assert.True(check.DataAvailable)

	check = dalc.CheckBlockAvailability(h2)
	assert.Equal(da.StatusSuccess, check.Code)
	assert.True(check.DataAvailable)

	// this height should not be used by DALC
	check = dalc.CheckBlockAvailability(h1 - 1)
	assert.Equal(da.StatusSuccess, check.Code)
	assert.False(check.DataAvailable)
}

func TestRetrieve(t *testing.T) {
	srv := startMockServ(t)
	defer srv.GracefulStop()
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

func startMockServ(t *testing.T) *grpc.Server {
	conf := grpcda.DefaultConfig
	srv := mockserv.GetServer(store.NewDefaultInMemoryKVStore(), conf, []byte(mockDaBlockTime.String()))
	lis, err := net.Listen("tcp", conf.Host+":"+strconv.Itoa(conf.Port))
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = srv.Serve(lis)
	}()
	return srv
}

func doTestRetrieve(t *testing.T, dalc da.DataAvailabilityLayerClient) {
	require := require.New(t)
	assert := assert.New(t)

	// mock DALC will advance block height every 100ms
	conf := []byte{}
	if _, ok := dalc.(*mock.MockDataAvailabilityLayerClient); ok {
		conf = []byte(mockDaBlockTime.String())
	}
	err := dalc.Init(conf, store.NewDefaultInMemoryKVStore(), &test.TestLogger{T: t})
	require.NoError(err)

	err = dalc.Start()
	require.NoError(err)

	// wait a bit more than mockDaBlockTime, so mock can "produce" some blocks
	time.Sleep(mockDaBlockTime + 20*time.Millisecond)

	retriever := dalc.(da.BlockRetriever)
	countAtHeight := make(map[uint64]int)
	blocks := make(map[*types.Block]uint64)

	for i := uint64(0); i < 100; i++ {
		b := getRandomBlock(i, rand.Int()%20)
		resp := dalc.SubmitBlock(b)
		assert.Equal(da.StatusSuccess, resp.Code)
		time.Sleep(time.Duration(rand.Int63() % mockDaBlockTime.Milliseconds()))

		countAtHeight[resp.DataLayerHeight]++
		blocks[b] = resp.DataLayerHeight
	}

	// wait a bit more than mockDaBlockTime, so mock can "produce" last blocks
	time.Sleep(mockDaBlockTime + 20*time.Millisecond)

	for h, cnt := range countAtHeight {
		ret := retriever.RetrieveBlocks(h)
		assert.Equal(da.StatusSuccess, ret.Code)
		require.NotEmpty(ret.Blocks)
		assert.Len(ret.Blocks, cnt)
	}

	for b, h := range blocks {
		ret := retriever.RetrieveBlocks(h)
		assert.Equal(da.StatusSuccess, ret.Code)
		require.NotEmpty(ret.Blocks)
		assert.Contains(ret.Blocks, b)
	}
}

// copy-pasted from store/store_test.go
func getRandomBlock(height uint64, nTxs int) *types.Block {
	block := &types.Block{
		Header: types.Header{
			Height: height,
		},
		Data: types.Data{
			Txs: make(types.Txs, nTxs),
			IntermediateStateRoots: types.IntermediateStateRoots{
				RawRootsList: make([][]byte, nTxs),
			},
		},
	}
	copy(block.Header.AppHash[:], getRandomBytes(32))

	for i := 0; i < nTxs; i++ {
		block.Data.Txs[i] = getRandomTx()
		block.Data.IntermediateStateRoots.RawRootsList[i] = getRandomBytes(32)
	}

	// TODO(tzdybal): see https://github.com/celestiaorg/optimint/issues/143
	if nTxs == 0 {
		block.Data.Txs = nil
		block.Data.IntermediateStateRoots.RawRootsList = nil
	}

	return block
}

func getRandomTx() types.Tx {
	size := rand.Int()%100 + 100
	return types.Tx(getRandomBytes(size))
}

func getRandomBytes(n int) []byte {
	data := make([]byte, n)
	_, _ = rand.Read(data)
	return data
}
