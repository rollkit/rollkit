package server

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	logging "github.com/ipfs/go-log/v2"
	ds "github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/rollkit/rollkit/pkg/p2p"
	"github.com/rollkit/rollkit/pkg/store"
	"github.com/rollkit/rollkit/test/mocks"
	"github.com/rollkit/rollkit/types"
	pb "github.com/rollkit/rollkit/types/pb/rollkit/v1"
)

func TestGetBlock(t *testing.T) {
	// Create a mock store
	mockStore := mocks.NewMockStore(t)

	// Create test data
	height := uint64(10)
	// Ensure the header has the correct height for key generation
	header := &types.SignedHeader{Header: types.Header{BaseHeader: types.BaseHeader{Height: height}}}
	data := &types.Data{}
	expectedHeaderDAHeight := uint64(100)
	expectedDataDAHeight := uint64(101)

	headerDAHeightBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(headerDAHeightBytes, expectedHeaderDAHeight)
	dataDAHeightBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(dataDAHeightBytes, expectedDataDAHeight)

	// Create server with mock store
	logger := logging.Logger("test")
	_ = logging.SetLogLevel("test", "FATAL") 
	server := NewStoreServer(mockStore, logger)

	// Test GetBlock with height - success case
	t.Run("by height with DA heights", func(t *testing.T) {
		// Setup mock expectations
		mockStore.On("GetBlockData", mock.Anything, height).Return(header, data, nil).Once()
		mockStore.On("GetMetadata", mock.Anything, fmt.Sprintf("%s/%d/h", store.RollkitHeightToDAHeightKey, height)).Return(headerDAHeightBytes, nil).Once()
		mockStore.On("GetMetadata", mock.Anything, fmt.Sprintf("%s/%d/d", store.RollkitHeightToDAHeightKey, height)).Return(dataDAHeightBytes, nil).Once()

		req := connect.NewRequest(&pb.GetBlockRequest{
			Identifier: &pb.GetBlockRequest_Height{
				Height: height,
			},
		})
		resp, err := server.GetBlock(context.Background(), req)

		// Assert expectations
		require.NoError(t, err)
		require.NotNil(t, resp.Msg.Block)
		require.Equal(t, expectedHeaderDAHeight, resp.Msg.HeaderDaHeight)
		require.Equal(t, expectedDataDAHeight, resp.Msg.DataDaHeight)
		mockStore.AssertExpectations(t)
	})

	// Test GetBlock with height - metadata not found
	t.Run("by height DA heights not found", func(t *testing.T) {
		mockStore.On("GetBlockData", mock.Anything, height).Return(header, data, nil).Once()
		mockStore.On("GetMetadata", mock.Anything, fmt.Sprintf("%s/%d/h", store.RollkitHeightToDAHeightKey, height)).Return(nil, ds.ErrNotFound).Once()
		mockStore.On("GetMetadata", mock.Anything, fmt.Sprintf("%s/%d/d", store.RollkitHeightToDAHeightKey, height)).Return(nil, ds.ErrNotFound).Once()

		req := connect.NewRequest(&pb.GetBlockRequest{
			Identifier: &pb.GetBlockRequest_Height{
				Height: height,
			},
		})
		resp, err := server.GetBlock(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, resp.Msg.Block)
		require.Equal(t, uint64(0), resp.Msg.HeaderDaHeight) // Should default to 0
		require.Equal(t, uint64(0), resp.Msg.DataDaHeight)   // Should default to 0
		mockStore.AssertExpectations(t)
	})

	// Test GetBlock with hash - success case
	t.Run("by hash with DA heights", func(t *testing.T) {
		hashBytes := []byte("test_hash")
		// Important: The header returned by GetBlockByHash must also have its height set for DA height lookup
		headerForHash := &types.SignedHeader{Header: types.Header{BaseHeader: types.BaseHeader{Height: height}}}
		mockStore.On("GetBlockByHash", mock.Anything, hashBytes).Return(headerForHash, data, nil).Once()
		mockStore.On("GetMetadata", mock.Anything, fmt.Sprintf("%s/%d/h", store.RollkitHeightToDAHeightKey, height)).Return(headerDAHeightBytes, nil).Once()
		mockStore.On("GetMetadata", mock.Anything, fmt.Sprintf("%s/%d/d", store.RollkitHeightToDAHeightKey, height)).Return(dataDAHeightBytes, nil).Once()

		req := connect.NewRequest(&pb.GetBlockRequest{
			Identifier: &pb.GetBlockRequest_Hash{
				Hash: hashBytes,
			},
		})
		resp, err := server.GetBlock(context.Background(), req)

		require.NoError(t, err)
		require.NotNil(t, resp.Msg.Block)
		require.Equal(t, expectedHeaderDAHeight, resp.Msg.HeaderDaHeight)
		require.Equal(t, expectedDataDAHeight, resp.Msg.DataDaHeight)
		mockStore.AssertExpectations(t)
	})

	// Test GetBlock with genesis height (0), DA heights should be 0 as per current server logic
	t.Run("by height genesis (height 0)", func(t *testing.T) {
		genesisHeight := uint64(0)                                              // Requesting latest, and store.Height will return 0
		mockStore.On("Height", mock.Anything).Return(genesisHeight, nil).Once() // Simulate store being at genesis (current height is 0)

		req := connect.NewRequest(&pb.GetBlockRequest{
			Identifier: &pb.GetBlockRequest_Height{
				Height: genesisHeight, // Requesting block 0 (interpreted as "latest")
			},
		})
		resp, err := server.GetBlock(context.Background(), req)

		require.Error(t, err)
		require.Nil(t, resp)
		var connectErr *connect.Error
		require.ErrorAs(t, err, &connectErr)
		require.Equal(t, connect.CodeNotFound, connectErr.Code())
		require.Contains(t, connectErr.Message(), "store is empty, no latest block available")
		mockStore.AssertExpectations(t)
	})
}

func TestGetBlock_Latest(t *testing.T) {

	mockStore := mocks.NewMockStore(t)
	logger := logging.Logger("test")
	_ = logging.SetLogLevel("test", "FATAL") 
	server := NewStoreServer(mockStore, logger)

	latestHeight := uint64(20)
	header := &types.SignedHeader{Header: types.Header{BaseHeader: types.BaseHeader{Height: latestHeight}}}
	data := &types.Data{}
	expectedHeaderDAHeight := uint64(200)
	expectedDataDAHeight := uint64(201)

	headerDAHeightBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(headerDAHeightBytes, expectedHeaderDAHeight)
	dataDAHeightBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(dataDAHeightBytes, expectedDataDAHeight)

	// Expectation for Height (which should be called by GetLatestBlockHeight)
	mockStore.On("Height", context.Background()).Return(latestHeight, nil).Once()
	// Expectation for GetBlockData with the latest height
	mockStore.On("GetBlockData", context.Background(), latestHeight).Return(header, data, nil).Once()
	// Expectation for DA height metadata
	mockStore.On("GetMetadata", mock.Anything, fmt.Sprintf("%s/%d/h", store.RollkitHeightToDAHeightKey, latestHeight)).Return(headerDAHeightBytes, nil).Once()
	mockStore.On("GetMetadata", mock.Anything, fmt.Sprintf("%s/%d/d", store.RollkitHeightToDAHeightKey, latestHeight)).Return(dataDAHeightBytes, nil).Once()

	req := connect.NewRequest(&pb.GetBlockRequest{
		Identifier: &pb.GetBlockRequest_Height{
			Height: 0, // Indicates latest block
		},
	})
	resp, err := server.GetBlock(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Block)
	require.Equal(t, expectedHeaderDAHeight, resp.Msg.HeaderDaHeight)
	require.Equal(t, expectedDataDAHeight, resp.Msg.DataDaHeight)
	mockStore.AssertExpectations(t)
}

func TestGetState(t *testing.T) {
	// Create a mock store
	mockStore := mocks.NewMockStore(t)

	// Create test data
	state := types.State{
		AppHash:         []byte("app_hash"),
		InitialHeight:   10,
		LastBlockHeight: 10,
		LastBlockTime:   time.Now(),
		ChainID:         "test-chain",
		Version: types.Version{
			Block: 1,
			App:   1,
		},
	}

	// Setup mock expectations
	mockStore.On("GetState", mock.Anything).Return(state, nil)

	// Create server with mock store
	logger := logging.Logger("test")
	_ = logging.SetLogLevel("test", "FATAL") 
	server := NewStoreServer(mockStore, logger)

	// Call GetState
	req := connect.NewRequest(&emptypb.Empty{})
	resp, err := server.GetState(context.Background(), req)

	// Assert expectations
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.State)
	require.Equal(t, state.AppHash, resp.Msg.State.AppHash)
	require.Equal(t, state.InitialHeight, resp.Msg.State.InitialHeight)
	require.Equal(t, state.LastBlockHeight, resp.Msg.State.LastBlockHeight)
	require.Equal(t, state.LastBlockTime.UTC(), resp.Msg.State.LastBlockTime.AsTime())
	require.Equal(t, state.ChainID, resp.Msg.State.ChainId)
	require.Equal(t, state.Version.Block, resp.Msg.State.Version.Block)
	require.Equal(t, state.Version.App, resp.Msg.State.Version.App)
	mockStore.AssertExpectations(t)
}

func TestGetState_Error(t *testing.T) {
	mockStore := mocks.NewMockStore(t)
	mockStore.On("GetState", mock.Anything).Return(types.State{}, fmt.Errorf("state error"))
	logger := logging.Logger("test")
	_ = logging.SetLogLevel("test", "FATAL") 
	server := NewStoreServer(mockStore, logger)
	resp, err := server.GetState(context.Background(), connect.NewRequest(&emptypb.Empty{}))
	require.Error(t, err)
	require.Nil(t, resp)
}

func TestGetMetadata(t *testing.T) {
	// Create a mock store
	mockStore := mocks.NewMockStore(t)

	// Create test data
	key := "test_key"
	value := []byte("test_value")

	// Setup mock expectations
	mockStore.On("GetMetadata", mock.Anything, key).Return(value, nil)

	// Create server with mock store
	logger := logging.Logger("test")
	_ = logging.SetLogLevel("test", "FATAL") 
	server := NewStoreServer(mockStore, logger)

	// Call GetMetadata
	req := connect.NewRequest(&pb.GetMetadataRequest{
		Key: key,
	})
	resp, err := server.GetMetadata(context.Background(), req)

	// Assert expectations
	require.NoError(t, err)
	require.Equal(t, value, resp.Msg.Value)
	mockStore.AssertExpectations(t)
}

func TestGetMetadata_Error(t *testing.T) {
	mockStore := mocks.NewMockStore(t)
	mockStore.On("GetMetadata", mock.Anything, "bad").Return(nil, fmt.Errorf("meta error"))
	logger := logging.Logger("test")
	_ = logging.SetLogLevel("test", "FATAL") 
	server := NewStoreServer(mockStore, logger)
	resp, err := server.GetMetadata(context.Background(), connect.NewRequest(&pb.GetMetadataRequest{Key: "bad"}))
	require.Error(t, err)
	require.Nil(t, resp)
}

func TestP2PServer_GetPeerInfo(t *testing.T) {
	mockP2P := &mocks.MockP2PRPC{}
	addr, err := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	require.NoError(t, err)
	mockP2P.On("GetPeers").Return([]peer.AddrInfo{{ID: "id1", Addrs: []multiaddr.Multiaddr{addr}}}, nil)
	server := NewP2PServer(mockP2P)
	resp, err := server.GetPeerInfo(context.Background(), connect.NewRequest(&emptypb.Empty{}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Peers, 1)
	mockP2P.AssertExpectations(t)

	// Error case
	mockP2P2 := &mocks.MockP2PRPC{}
	mockP2P2.On("GetPeers").Return(nil, fmt.Errorf("p2p error"))
	server2 := NewP2PServer(mockP2P2)
	resp2, err2 := server2.GetPeerInfo(context.Background(), connect.NewRequest(&emptypb.Empty{}))
	require.Error(t, err2)
	require.Nil(t, resp2)
}

func TestP2PServer_GetNetInfo(t *testing.T) {
	mockP2P := &mocks.MockP2PRPC{}
	netInfo := p2p.NetworkInfo{ID: "nid", ListenAddress: []string{"addr1"}}
	mockP2P.On("GetNetworkInfo").Return(netInfo, nil)
	server := NewP2PServer(mockP2P)
	resp, err := server.GetNetInfo(context.Background(), connect.NewRequest(&emptypb.Empty{}))
	require.NoError(t, err)
	require.Equal(t, netInfo.ID, resp.Msg.NetInfo.Id)
	mockP2P.AssertExpectations(t)

	// Error case
	mockP2P2 := &mocks.MockP2PRPC{}
	mockP2P2.On("GetNetworkInfo").Return(p2p.NetworkInfo{}, fmt.Errorf("netinfo error"))
	server2 := NewP2PServer(mockP2P2)
	resp2, err2 := server2.GetNetInfo(context.Background(), connect.NewRequest(&emptypb.Empty{}))
	require.Error(t, err2)
	require.Nil(t, resp2)
}

func TestHealthServer_Livez(t *testing.T) {
	h := NewHealthServer()
	resp, err := h.Livez(context.Background(), connect.NewRequest(&emptypb.Empty{}))
	require.NoError(t, err)
	require.Equal(t, pb.HealthStatus_PASS, resp.Msg.Status)
}

func TestHealthLiveEndpoint(t *testing.T) {
	assert := require.New(t)

	// Create mock dependencies
	mockStore := mocks.NewMockStore(t)
	mockP2PManager := &mocks.MockP2PRPC{} // Assuming this mock is sufficient or can be adapted

	// Create the service handler
	logger := logging.Logger("test")
	_ = logging.SetLogLevel("test", "FATAL") 
	handler, err := NewServiceHandler(mockStore, mockP2PManager, logger)
	assert.NoError(err)
	assert.NotNil(handler)

	// Create a new HTTP test server
	server := httptest.NewServer(handler)
	defer server.Close()

	// Make a GET request to the /health/live endpoint
	resp, err := http.Get(server.URL + "/health/live")
	assert.NoError(err)
	defer resp.Body.Close()

	// Check the status code
	assert.Equal(http.StatusOK, resp.StatusCode)

	// Check the response body
	body, err := io.ReadAll(resp.Body)
	assert.NoError(err)
	assert.Equal("OK\n", string(body)) // fmt.Fprintln adds a newline
}
