package test

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	cmlog "github.com/cometbft/cometbft/libs/log"

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

var (
	testNamespaceID = types.NamespaceID{0, 1, 2, 3, 4, 5, 6, 7}

	testConfig = celestia.Config{
		BaseURL:  "http://localhost:26658",
		Timeout:  30 * time.Second,
		GasLimit: 3000000,
	}
)

func TestMain(m *testing.M) {
	srv := startMockGRPCServ()
	if srv == nil {
		os.Exit(1)
	}

	httpServer := startMockCelestiaNodeServer()
	if httpServer == nil {
		os.Exit(1)
	}

	exitCode := m.Run()

	// teardown servers
	srv.GracefulStop()
	httpServer.Stop()

	os.Exit(exitCode)
}

func TestLifecycle(t *testing.T) {
	for _, dalc := range registry.RegisteredClients() {
		t.Run(dalc, func(t *testing.T) {
			doTestLifecycle(t, registry.GetClient(dalc))
		})
	}
}

func doTestLifecycle(t *testing.T, dalc da.DataAvailabilityLayerClient) {
	require := require.New(t)

	conf := []byte{}
	if _, ok := dalc.(*mock.DataAvailabilityLayerClient); ok {
		conf = []byte(mockDaBlockTime.String())
	}
	if _, ok := dalc.(*celestia.DataAvailabilityLayerClient); ok {
		conf, _ = json.Marshal(testConfig)
	}
	err := dalc.Init(testNamespaceID, conf, nil, test.NewLogger(t))
	require.NoError(err)

	err = dalc.Start()
	require.NoError(err)

	err = dalc.Stop()
	require.NoError(err)
}

func TestDALC(t *testing.T) {
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
		conf, _ = json.Marshal(testConfig)
	}
	kvStore, _ := store.NewDefaultInMemoryKVStore()
	err := dalc.Init(testNamespaceID, conf, kvStore, test.NewLogger(t))
	require.NoError(err)

	err = dalc.Start()
	require.NoError(err)

	// wait a bit more than mockDaBlockTime, so mock can "produce" some blocks
	time.Sleep(mockDaBlockTime + 20*time.Millisecond)

	// only blocks b1 and b2 will be submitted to DA
	b1 := getRandomBlock(1, 10)
	b2 := getRandomBlock(2, 10)

	resp := dalc.SubmitBlock(ctx, b1)
	h1 := resp.DAHeight
	assert.Equal(da.StatusSuccess, resp.Code)

	resp = dalc.SubmitBlock(ctx, b2)
	h2 := resp.DAHeight
	assert.Equal(da.StatusSuccess, resp.Code)

	// wait a bit more than mockDaBlockTime, so Rollkit blocks can be "included" in mock block
	time.Sleep(mockDaBlockTime + 20*time.Millisecond)

	check := dalc.CheckBlockAvailability(ctx, h1)
	assert.Equal(da.StatusSuccess, check.Code)
	assert.True(check.DataAvailable)

	check = dalc.CheckBlockAvailability(ctx, h2)
	assert.Equal(da.StatusSuccess, check.Code)
	assert.True(check.DataAvailable)

	// this height should not be used by DALC
	check = dalc.CheckBlockAvailability(ctx, h1-1)
	assert.Equal(da.StatusSuccess, check.Code)
	assert.False(check.DataAvailable)
}

func TestRetrieve(t *testing.T) {
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

func startMockGRPCServ() *grpc.Server {
	conf := grpcda.DefaultConfig
	logger := cmlog.NewTMLogger(os.Stdout)

	kvStore, _ := store.NewDefaultInMemoryKVStore()
	srv := mockserv.GetServer(kvStore, conf, []byte(mockDaBlockTime.String()), logger)
	lis, err := net.Listen("tcp", conf.Host+":"+strconv.Itoa(conf.Port))
	if err != nil {
		fmt.Println(err)
		return nil
	}
	go func() {
		_ = srv.Serve(lis)
	}()
	return srv
}

func startMockCelestiaNodeServer() *cmock.Server {
	httpSrv := cmock.NewServer(mockDaBlockTime, cmlog.NewTMLogger(os.Stdout))
	l, err := net.Listen("tcp4", "127.0.0.1:26658")
	if err != nil {
		fmt.Println("failed to create listener for mock celestia-node RPC server, error: %w", err)
		return nil
	}
	err = httpSrv.Start(l)
	if err != nil {
		fmt.Println("can't start mock celestia-node RPC server")
		return nil
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
		conf, _ = json.Marshal(testConfig)
	}
	kvStore, _ := store.NewDefaultInMemoryKVStore()
	err := dalc.Init(testNamespaceID, conf, kvStore, test.NewLogger(t))
	require.NoError(err)

	err = dalc.Start()
	require.NoError(err)

	// wait a bit more than mockDaBlockTime, so mock can "produce" some blocks
	time.Sleep(mockDaBlockTime + 20*time.Millisecond)

	retriever := dalc.(da.BlockRetriever)
	countAtHeight := make(map[uint64]int)
	blocks := make(map[*types.Block]uint64)

	for i := uint64(0); i < 100; i++ {
		b := getRandomBlock(i, rand.Int()%20) //nolint:gosec
		resp := dalc.SubmitBlock(ctx, b)
		assert.Equal(da.StatusSuccess, resp.Code, resp.Message)
		time.Sleep(time.Duration(rand.Int63() % mockDaBlockTime.Milliseconds())) //nolint:gosec

		countAtHeight[resp.DAHeight]++
		blocks[b] = resp.DAHeight
	}

	// wait a bit more than mockDaBlockTime, so mock can "produce" last blocks
	time.Sleep(mockDaBlockTime + 20*time.Millisecond)

	for h, cnt := range countAtHeight {
		t.Log("Retrieving block, DA Height", h)
		ret := retriever.RetrieveBlocks(ctx, h)
		assert.Equal(da.StatusSuccess, ret.Code, ret.Message)
		require.NotEmpty(ret.Blocks, h)
		assert.Len(ret.Blocks, cnt, h)
	}

	for b, h := range blocks {
		ret := retriever.RetrieveBlocks(ctx, h)
		assert.Equal(da.StatusSuccess, ret.Code, h)
		require.NotEmpty(ret.Blocks, h)
		assert.Contains(ret.Blocks, b, h)
	}
}
