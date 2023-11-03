package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"os"
	"reflect"
	"strconv"
	"strings"
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
	"github.com/rollkit/rollkit/store"
	test "github.com/rollkit/rollkit/test/log"
	"github.com/rollkit/rollkit/types"
)

var (
	testNamespaceID = types.NamespaceID{0, 1, 2, 3, 4, 5, 6, 7}

	testConfig = celestia.Config{
		Timeout:  15 * time.Second,
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
	err := dalc.Init(testNamespaceID, conf, nil, test.NewFileLoggerCustom(t, test.TempLogFileName(t, "dalc")))
	require.NoError(err)

	err = dalc.Start()
	require.NoError(err)

	defer func() {
		require.NoError(dalc.Stop())
	}()
}

func TestRetrieveRedo(t *testing.T) {
	require := require.New(t)
	dalc := registry.GetClient("mock")
	//ctx, cancel := context.WithCancel(context.Background())
	conf := []byte(mockDaBlockTime.String())
	kvStore, _ := store.NewDefaultInMemoryKVStore()
	err := dalc.Init(testNamespaceID, conf, kvStore, test.NewFileLoggerCustom(t, test.TempLogFileName(t, "dalc")))
	require.NoError(err)
	err = dalc.Start()
	require.NoError(err)
	defer func() {
		require.NoError(dalc.Stop())
	}()
	fmt.Println("staring to sleep...")
	time.Sleep(mockDaBlockTime + 20*time.Millisecond)
	fmt.Println("done")
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
	url, err := httpSrv.Start()
	if err != nil {
		fmt.Println("can't start mock celestia-node RPC server")
		return nil
	}
	testConfig.BaseURL = url
	return httpSrv
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
	err := dalc.Init(testNamespaceID, conf, kvStore, test.NewFileLoggerCustom(t, test.TempLogFileName(t, "dalc")))
	require.NoError(err)

	err = dalc.Start()
	require.NoError(err)
	defer func() {
		require.NoError(dalc.Stop())
	}()

	// wait a bit more than mockDaBlockTime, so mock can "produce" some blocks
	time.Sleep(mockDaBlockTime + 20*time.Millisecond)

	retriever := dalc.(da.BlockRetriever)
	//countAtHeight := make(map[uint64]int)
	//blockToDAHeight := make(map[*types.Block]uint64)
	//blockhashToDAHeight := make(map[[32]byte]uint64)
	//numBatches := uint64(10)
	batchSize := 10

	blocks := make([]*types.Block, batchSize)
	for j := 0; j < batchSize; j++ {
		blocks[j] = getRandomBlock( /*i*uint64(batchSize)**/ uint64(j), rand.Int()%20) //nolint:gosec
		fmt.Printf("submitting block with hash %02x\n", [32]byte(blocks[j].SignedHeader.AppHash))
	}
	resp := dalc.SubmitBlocks(ctx, blocks)
	assert.Equal(da.StatusSuccess, resp.Code, resp.Message)
	//time.Sleep(time.Duration(rand.Int63() % mockDaBlockTime.Milliseconds())) //nolint:gosec
	time.Sleep(time.Second * 1)

	ret := retriever.RetrieveBlocks(ctx, resp.DAHeight)
	assert.Equal(da.StatusSuccess, ret.Code, resp.DAHeight)
	require.NotEmpty(ret.Blocks, resp.DAHeight)
	fmt.Println("got blocks!")
	for _, block := range ret.Blocks {
		fmt.Printf("got block with hash %02x\n", [32]byte(block.SignedHeader.AppHash))
	}
	return

	/*for i := uint64(0); i < numBatches; i++ {
		blocks := make([]*types.Block, batchSize)
		for j := 0; j < batchSize; j++ {
			blocks[j] = getRandomBlock(i*uint64(batchSize)+uint64(j), rand.Int()%20) //nolint:gosec
			fmt.Printf("submitting block with hash %02x\n", [32]byte(blocks[j].SignedHeader.AppHash))
		}
		resp := dalc.SubmitBlocks(ctx, blocks)
		assert.Equal(da.StatusSuccess, resp.Code, resp.Message)
		time.Sleep(time.Duration(rand.Int63() % mockDaBlockTime.Milliseconds())) //nolint:gosec

		for _, b := range blocks {
			blockToDAHeight[b] = resp.DAHeight
			blockhashToDAHeight[[32]byte(b.SignedHeader.AppHash)] = resp.DAHeight
			countAtHeight[resp.DAHeight]++
		}
		fmt.Println("The height was: ", resp.DAHeight)
	}*/

	// wait a bit more than mockDaBlockTime, so mock can "produce" last blocks
	time.Sleep(mockDaBlockTime + 20*time.Millisecond)

	/*for h, cnt := range countAtHeight {
		t.Log("Retrieving block, DA Height", h)
		ret := retriever.RetrieveBlocks(ctx, h)
		assert.Equal(da.StatusSuccess, ret.Code, ret.Message)
		require.NotEmpty(ret.Blocks, h)
		assert.Equal(cnt%batchSize, 0)
		assert.Len(ret.Blocks, cnt, h)
	}*/

	/*for b, h := range blockhashToDAHeight {
		fmt.Println("CHECKING:")
		fmt.Printf("b: %02x h: %d\n", b, h)
		ret := retriever.RetrieveBlocks(ctx, h)
		for _, block := range ret.Blocks {
			fmt.Println(block)
			fmt.Printf("blockhash: %02x\n", [32]byte(block.SignedHeader.AppHash))
		}
	}*/

	/*got := 0
	missed := 0
	for b, h := range blockToDAHeight {
		ret := retriever.RetrieveBlocks(ctx, h)
		assert.Equal(da.StatusSuccess, ret.Code, h)
		require.NotEmpty(ret.Blocks, h)
		//assert.Contains(ret.Blocks, b, h)
		if _, found := containsElement(ret.Blocks, b); found {
			got++
		} else {
			missed++
		}
	}
	fmt.Println("got ", got, " missed ", missed)*/

}

// containsElement try loop over the list check if the list includes the element.
// return (false, false) if impossible.
// return (true, false) if element was not found.
// return (true, true) if element was found.
func containsElement(list interface{}, element interface{}) (ok, found bool) {

	listValue := reflect.ValueOf(list)
	listType := reflect.TypeOf(list)
	if listType == nil {
		return false, false
	}
	listKind := listType.Kind()
	defer func() {
		if e := recover(); e != nil {
			ok = false
			found = false
		}
	}()

	if listKind == reflect.String {
		elementValue := reflect.ValueOf(element)
		return true, strings.Contains(listValue.String(), elementValue.String())
	}

	if listKind == reflect.Map {
		mapKeys := listValue.MapKeys()
		for i := 0; i < len(mapKeys); i++ {
			if ObjectsAreEqual(mapKeys[i].Interface(), element) {
				return true, true
			}
		}
		return true, false
	}

	for i := 0; i < listValue.Len(); i++ {
		if ObjectsAreEqual(listValue.Index(i).Interface(), element) {
			return true, true
		}
	}
	return true, false

}

// ObjectsAreEqual determines if two objects are considered equal.
//
// This function does no assertion of any kind.
func ObjectsAreEqual(expected, actual interface{}) bool {
	if expected == nil || actual == nil {
		return expected == actual
	}

	exp, ok := expected.([]byte)
	if !ok {
		return reflect.DeepEqual(expected, actual)
	}

	act, ok := actual.([]byte)
	if !ok {
		return false
	}
	if exp == nil || act == nil {
		return exp == nil && act == nil
	}
	return bytes.Equal(exp, act)
}
