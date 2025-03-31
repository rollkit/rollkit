package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/rollkit/rollkit/pkg/rpc/server"
	"github.com/rollkit/rollkit/test/mocks"
	"github.com/rollkit/rollkit/types"
	rpc "github.com/rollkit/rollkit/types/pb/rollkit/v1/v1connect"
)

// setupTestServer creates a test server with a mock store
func setupTestServer(t *testing.T, mockStore *mocks.Store) (*httptest.Server, *Client) {
	// Create a new HTTP test server
	mux := http.NewServeMux()

	// Create a new store server
	storeServer := server.NewStoreServer(mockStore)

	// Register the store service
	path, handler := rpc.NewStoreServiceHandler(storeServer)
	mux.Handle(path, handler)

	// Create an HTTP server with h2c for HTTP/2 support
	testServer := httptest.NewServer(h2c.NewHandler(mux, &http2.Server{}))

	// Create a client that connects to the test server
	client := NewClient(testServer.URL)

	return testServer, client
}

func TestClientGetState(t *testing.T) {
	// Create a mock store
	mockStore := mocks.NewStore(t)

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
	testServer, client := setupTestServer(t, mockStore)
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
	// Create a mock store
	mockStore := mocks.NewStore(t)

	// Create test data
	key := "test_key"
	value := []byte("test_value")

	// Setup mock expectations
	mockStore.On("GetMetadata", mock.Anything, key).Return(value, nil)

	// Setup test server and client
	testServer, client := setupTestServer(t, mockStore)
	defer testServer.Close()

	// Call GetMetadata
	resultValue, err := client.GetMetadata(context.Background(), key)

	// Assert expectations
	require.NoError(t, err)
	require.Equal(t, value, resultValue)
	mockStore.AssertExpectations(t)
}

func TestClientGetBlockByHeight(t *testing.T) {
	// Create a mock store
	mockStore := mocks.NewStore(t)

	// Create test data
	height := uint64(10)
	header := &types.SignedHeader{}
	data := &types.Data{}

	// Setup mock expectations
	mockStore.On("GetBlockData", mock.Anything, height).Return(header, data, nil)

	// Setup test server and client
	testServer, client := setupTestServer(t, mockStore)
	defer testServer.Close()

	// Call GetBlockByHeight
	block, err := client.GetBlockByHeight(context.Background(), height)

	// Assert expectations
	require.NoError(t, err)
	require.NotNil(t, block)
	mockStore.AssertExpectations(t)
}

func TestClientGetBlockByHash(t *testing.T) {
	// Create a mock store
	mockStore := mocks.NewStore(t)

	// Create test data
	hash := []byte("block_hash")
	header := &types.SignedHeader{}
	data := &types.Data{}

	// Setup mock expectations
	mockStore.On("GetBlockByHash", mock.Anything, hash).Return(header, data, nil)

	// Setup test server and client
	testServer, client := setupTestServer(t, mockStore)
	defer testServer.Close()

	// Call GetBlockByHash
	block, err := client.GetBlockByHash(context.Background(), hash)

	// Assert expectations
	require.NoError(t, err)
	require.NotNil(t, block)
	mockStore.AssertExpectations(t)
}
