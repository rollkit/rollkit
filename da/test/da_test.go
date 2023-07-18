package test

import (
	"context"
	"encoding/json"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/da"
	"github.com/rollkit/rollkit/da/celestia"
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

	defer func() {
		require.NoError(dalc.Stop())
	}()
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

func doTestRetrieve(t *testing.T, dalc da.DataAvailabilityLayerClient) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
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
	defer func() {
		require.NoError(dalc.Stop())
	}()

	// wait a bit more than mockDaBlockTime, so mock can "produce" some blocks
	time.Sleep(mockDaBlockTime + 20*time.Millisecond)

	retriever := dalc.(da.BlockRetriever)
	countAtHeight := make(map[uint64]int)
	blockToDAHeight := make(map[*types.Block]uint64)

	for i := uint64(0); i < 100; i++ {
		blocks := make([]*types.Block, 10)
		for j := 0; j < len(blocks); j++ {
			blocks[j] = getRandomBlock(i, rand.Int()%20) //nolint:gosec
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
		assert.Len(ret.Blocks, cnt, h)
	}

	for b, h := range blockToDAHeight {
		ret := retriever.RetrieveBlocks(ctx, h)
		assert.Equal(da.StatusSuccess, ret.Code, h)
		require.NotEmpty(ret.Blocks, h)
		assert.Contains(ret.Blocks, b, h)
	}
}
