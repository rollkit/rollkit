package server

import (
	"context"
	"fmt"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/rollkit/rollkit/pkg/p2p"
	"github.com/rollkit/rollkit/test/mocks"
	"github.com/rollkit/rollkit/types"
	pb "github.com/rollkit/rollkit/types/pb/rollkit/v1"
)

func TestGetBlock(t *testing.T) {
	// Create a mock store
	mockStore := mocks.NewStore(t)

	// Create test data
	height := uint64(10)
	header := &types.SignedHeader{}
	data := &types.SignedData{}

	// Setup mock expectations
	mockStore.On("GetBlockData", mock.Anything, height).Return(header, data, nil)

	// Create server with mock store
	server := NewStoreServer(mockStore)

	// Test GetBlock with height
	t.Run("by height", func(t *testing.T) {
		req := connect.NewRequest(&pb.GetBlockRequest{
			Identifier: &pb.GetBlockRequest_Height{
				Height: height,
			},
		})
		resp, err := server.GetBlock(context.Background(), req)

		// Assert expectations
		require.NoError(t, err)
		require.NotNil(t, resp.Msg.Block)
		mockStore.AssertExpectations(t)
	})

	// Test GetBlock with hash
	t.Run("by hash", func(t *testing.T) {
		hash := []byte("test_hash")
		mockStore.On("GetBlockByHash", mock.Anything, hash).Return(header, data, nil)

		req := connect.NewRequest(&pb.GetBlockRequest{
			Identifier: &pb.GetBlockRequest_Hash{
				Hash: hash,
			},
		})
		resp, err := server.GetBlock(context.Background(), req)

		// Assert expectations
		require.NoError(t, err)
		require.NotNil(t, resp.Msg.Block)
		mockStore.AssertExpectations(t)
	})
}

func TestGetBlock_Latest(t *testing.T) {
	mockStore := mocks.NewStore(t)
	server := NewStoreServer(mockStore)

	header := &types.SignedHeader{}
	data := &types.SignedData{}

	// Expectation for GetHeight (which should be called by GetLatestBlockHeight)
	mockStore.On("Height", context.Background()).Return(uint64(20), nil).Once()
	// Expectation for GetBlockData with the latest height
	mockStore.On("GetBlockData", context.Background(), uint64(20)).Return(header, data, nil).Once()

	req := connect.NewRequest(&pb.GetBlockRequest{
		Identifier: &pb.GetBlockRequest_Height{
			Height: 0, // Indicates latest block
		},
	})
	resp, err := server.GetBlock(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.Block)
	mockStore.AssertExpectations(t)
}

func TestGetState(t *testing.T) {
	// Create a mock store
	mockStore := mocks.NewStore(t)

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
	server := NewStoreServer(mockStore)

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
	mockStore := mocks.NewStore(t)
	mockStore.On("GetState", mock.Anything).Return(types.State{}, fmt.Errorf("state error"))
	server := NewStoreServer(mockStore)
	resp, err := server.GetState(context.Background(), connect.NewRequest(&emptypb.Empty{}))
	require.Error(t, err)
	require.Nil(t, resp)
}

func TestGetMetadata(t *testing.T) {
	// Create a mock store
	mockStore := mocks.NewStore(t)

	// Create test data
	key := "test_key"
	value := []byte("test_value")

	// Setup mock expectations
	mockStore.On("GetMetadata", mock.Anything, key).Return(value, nil)

	// Create server with mock store
	server := NewStoreServer(mockStore)

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
	mockStore := mocks.NewStore(t)
	mockStore.On("GetMetadata", mock.Anything, "bad").Return(nil, fmt.Errorf("meta error"))
	server := NewStoreServer(mockStore)
	resp, err := server.GetMetadata(context.Background(), connect.NewRequest(&pb.GetMetadataRequest{Key: "bad"}))
	require.Error(t, err)
	require.Nil(t, resp)
}

func TestP2PServer_GetPeerInfo(t *testing.T) {
	mockP2P := &mocks.P2PRPC{}
	addr, err := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	require.NoError(t, err)
	mockP2P.On("GetPeers").Return([]peer.AddrInfo{{ID: "id1", Addrs: []multiaddr.Multiaddr{addr}}}, nil)
	server := NewP2PServer(mockP2P)
	resp, err := server.GetPeerInfo(context.Background(), connect.NewRequest(&emptypb.Empty{}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Peers, 1)
	mockP2P.AssertExpectations(t)

	// Error case
	mockP2P2 := &mocks.P2PRPC{}
	mockP2P2.On("GetPeers").Return(nil, fmt.Errorf("p2p error"))
	server2 := NewP2PServer(mockP2P2)
	resp2, err2 := server2.GetPeerInfo(context.Background(), connect.NewRequest(&emptypb.Empty{}))
	require.Error(t, err2)
	require.Nil(t, resp2)
}

func TestP2PServer_GetNetInfo(t *testing.T) {
	mockP2P := &mocks.P2PRPC{}
	netInfo := p2p.NetworkInfo{ID: "nid", ListenAddress: []string{"addr1"}}
	mockP2P.On("GetNetworkInfo").Return(netInfo, nil)
	server := NewP2PServer(mockP2P)
	resp, err := server.GetNetInfo(context.Background(), connect.NewRequest(&emptypb.Empty{}))
	require.NoError(t, err)
	require.Equal(t, netInfo.ID, resp.Msg.NetInfo.Id)
	mockP2P.AssertExpectations(t)

	// Error case
	mockP2P2 := &mocks.P2PRPC{}
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
