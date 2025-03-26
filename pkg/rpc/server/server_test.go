package server

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

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
	data := &types.Data{}
	signature := &types.Signature{}

	// Setup mock expectations
	mockStore.On("GetBlockData", mock.Anything, height).Return(header, data, nil)
	mockStore.On("GetSignature", mock.Anything, height).Return(signature, nil)

	// Create server with mock store
	server := NewStoreServer(mockStore)

	// Call GetBlock
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
	}

	// Setup mock expectations
	mockStore.On("GetState", mock.Anything).Return(state, nil)

	// Create server with mock store
	server := NewStoreServer(mockStore)

	// Call GetState
	req := connect.NewRequest(&pb.GetStateRequest{})
	resp, err := server.GetState(context.Background(), req)

	// Assert expectations
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.State)
	require.Equal(t, state.AppHash, resp.Msg.State.AppHash)
	require.Equal(t, state.InitialHeight, resp.Msg.State.InitialHeight)
	require.Equal(t, state.LastBlockHeight, resp.Msg.State.LastBlockHeight)
	require.Equal(t, state.LastBlockTime, resp.Msg.State.LastBlockTime)
	mockStore.AssertExpectations(t)
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
