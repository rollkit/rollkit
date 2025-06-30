package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sdklog "cosmossdk.io/log"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/rollkit/rollkit/pkg/p2p"
	"github.com/rollkit/rollkit/pkg/rpc/server"
	"github.com/rollkit/rollkit/test/mocks"
	"github.com/rollkit/rollkit/types"
	rpc "github.com/rollkit/rollkit/types/pb/rollkit/v1/v1connect"
)

// setupTestServer creates a test server with mock store and mock p2p manager
func setupTestServer(t *testing.T, mockStore *mocks.MockStore, mockP2P *mocks.MockP2PRPC) (*httptest.Server, *Client) {
	// Create a new HTTP test server
	mux := http.NewServeMux()

	// Create the servers
	storeServer := server.NewStoreServer(mockStore, sdklog.NewTestLogger(t))
	p2pServer := server.NewP2PServer(mockP2P)

	// Register the store service
	storePath, storeHandler := rpc.NewStoreServiceHandler(storeServer)
	mux.Handle(storePath, storeHandler)

	// Register the p2p service
	p2pPath, p2pHandler := rpc.NewP2PServiceHandler(p2pServer)
	mux.Handle(p2pPath, p2pHandler)

	// Create an HTTP server with h2c for HTTP/2 support
	testServer := httptest.NewServer(h2c.NewHandler(mux, &http2.Server{}))

	// Create a client that connects to the test server
	client := NewClient(testServer.URL)

	return testServer, client
}

func TestClientGetState(t *testing.T) {
	// Create mocks
	mockStore := mocks.NewMockStore(t)
	mockP2P := mocks.NewMockP2PRPC(t)

	// Create test data
	state := types.State{
		AppHash:         []byte("app_hash"),
		InitialHeight:   10,
		LastBlockHeight: 10,
		LastBlockTime:   time.Now(),
	}

	// Setup mock expectations
	mockStore.On("GetState", mock.Anything).Return(state, nil)

	// Setup test server and client
	testServer, client := setupTestServer(t, mockStore, mockP2P)
	defer testServer.Close()

	// Call GetState
	resultState, err := client.GetState(context.Background())

	// Assert expectations
	require.NoError(t, err)
	require.Equal(t, state.AppHash, resultState.AppHash)
	require.Equal(t, state.InitialHeight, resultState.InitialHeight)
	require.Equal(t, state.LastBlockHeight, resultState.LastBlockHeight)
	require.Equal(t, state.LastBlockTime.UTC(), resultState.LastBlockTime.AsTime())
	mockStore.AssertExpectations(t)
}

func TestClientGetMetadata(t *testing.T) {
	// Create mocks
	mockStore := mocks.NewMockStore(t)
	mockP2P := mocks.NewMockP2PRPC(t)

	// Create test data
	key := "test_key"
	value := []byte("test_value")

	// Setup mock expectations
	mockStore.On("GetMetadata", mock.Anything, key).Return(value, nil)

	// Setup test server and client
	testServer, client := setupTestServer(t, mockStore, mockP2P)
	defer testServer.Close()

	// Call GetMetadata
	resultValue, err := client.GetMetadata(context.Background(), key)

	// Assert expectations
	require.NoError(t, err)
	require.Equal(t, value, resultValue)
	mockStore.AssertExpectations(t)
}

func TestClientGetBlockByHeight(t *testing.T) {
	// Create mocks
	mockStore := mocks.NewMockStore(t)
	mockP2P := mocks.NewMockP2PRPC(t)

	// Create test data
	height := uint64(10)
	header := &types.SignedHeader{}
	data := &types.Data{}

	// Setup mock expectations
	mockStore.On("GetBlockData", mock.Anything, height).Return(header, data, nil)

	// Setup test server and client
	testServer, client := setupTestServer(t, mockStore, mockP2P)
	defer testServer.Close()

	// Call GetBlockByHeight
	block, err := client.GetBlockByHeight(context.Background(), height)

	// Assert expectations
	require.NoError(t, err)
	require.NotNil(t, block)
	mockStore.AssertExpectations(t)
}

func TestClientGetBlockByHash(t *testing.T) {
	// Create mocks
	mockStore := mocks.NewMockStore(t)
	mockP2P := mocks.NewMockP2PRPC(t)

	// Create test data
	hash := []byte("block_hash")
	header := &types.SignedHeader{}
	data := &types.Data{}

	// Setup mock expectations
	mockStore.On("GetBlockByHash", mock.Anything, hash).Return(header, data, nil)

	// Setup test server and client
	testServer, client := setupTestServer(t, mockStore, mockP2P)
	defer testServer.Close()

	// Call GetBlockByHash
	block, err := client.GetBlockByHash(context.Background(), hash)

	// Assert expectations
	require.NoError(t, err)
	require.NotNil(t, block)
	mockStore.AssertExpectations(t)
}

func TestClientGetPeerInfo(t *testing.T) {
	// Create mocks
	mockStore := mocks.NewMockStore(t)
	mockP2P := mocks.NewMockP2PRPC(t)

	addr, err := multiaddr.NewMultiaddr("/ip4/0.0.0.0/tcp/8000")
	require.NoError(t, err)

	// Create test data
	peers := []peer.AddrInfo{
		{
			ID:    "3bM8hezDN5",
			Addrs: []multiaddr.Multiaddr{addr},
		},
		{
			ID:    "3tSMH9AUGpeoe4",
			Addrs: []multiaddr.Multiaddr{addr},
		},
	}

	// Setup mock expectations
	mockP2P.On("GetPeers").Return(peers, nil)

	// Setup test server and client
	testServer, client := setupTestServer(t, mockStore, mockP2P)
	defer testServer.Close()

	// Call GetPeerInfo
	resultPeers, err := client.GetPeerInfo(context.Background())

	// Assert expectations
	require.NoError(t, err)
	require.Len(t, resultPeers, 2)
	require.Equal(t, "3tSMH9AUGpeoe4", resultPeers[0].Id)
	require.Equal(t, "{3tSMH9AUGpeoe4: [/ip4/0.0.0.0/tcp/8000]}", resultPeers[0].Address)
	require.Equal(t, "Kv9im1EaxaZ2KEviHvT", resultPeers[1].Id)
	require.Equal(t, "{Kv9im1EaxaZ2KEviHvT: [/ip4/0.0.0.0/tcp/8000]}", resultPeers[1].Address)
	mockP2P.AssertExpectations(t)
}

func TestClientGetNetInfo(t *testing.T) {
	// Create mocks
	mockStore := mocks.NewMockStore(t)
	mockP2P := mocks.NewMockP2PRPC(t)

	// Create test data
	netInfo := p2p.NetworkInfo{
		ID:            "node1",
		ListenAddress: []string{"0.0.0.0:26656"},
	}

	// Setup mock expectations
	mockP2P.On("GetNetworkInfo").Return(netInfo, nil)

	// Setup test server and client
	testServer, client := setupTestServer(t, mockStore, mockP2P)
	defer testServer.Close()

	// Call GetNetInfo
	resultNetInfo, err := client.GetNetInfo(context.Background())

	// Assert expectations
	require.NoError(t, err)
	require.Equal(t, "node1", resultNetInfo.Id)
	require.Equal(t, "0.0.0.0:26656", resultNetInfo.ListenAddresses[0])
	mockP2P.AssertExpectations(t)
}
