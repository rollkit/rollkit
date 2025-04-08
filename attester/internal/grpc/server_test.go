package grpc_test

import (
	"context"
	"fmt"
	"io" // Added for io.Discard
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/hashicorp/raft"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"

	attesterv1 "github.com/rollkit/rollkit/attester/api/gen/attester/v1"
	grpcServer "github.com/rollkit/rollkit/attester/internal/grpc"
	"github.com/rollkit/rollkit/attester/internal/mocks"
)

const bufSize = 1024 * 1024

// setupTestServer initializes an in-memory gRPC server and returns a client.
// Uses mocks from the mocks package.
func setupTestServer(t *testing.T, mockRaft *mocks.MockRaftNode, mockAgg *mocks.MockAggregator) attesterv1.AttesterServiceClient {
	t.Helper()

	lis := bufconn.Listen(bufSize)
	t.Cleanup(func() {
		lis.Close()
	})

	s := grpc.NewServer()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	attesterServer := grpcServer.NewAttesterServer(mockRaft, logger, mockAgg)
	attesterv1.RegisterAttesterServiceServer(s, attesterServer)

	go func() {
		if err := s.Serve(lis); err != nil && err != grpc.ErrServerStopped {
			t.Logf("ERROR in server goroutine: %v", err)
		}
	}()

	time.Sleep(150 * time.Millisecond)

	t.Cleanup(func() {
		s.GracefulStop()
	})

	bufDialer := func(ctx context.Context, addr string) (net.Conn, error) {
		return lis.Dial()
	}

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(bufDialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err, "Failed to dial passthrough:///bufnet")

	t.Cleanup(func() {
		if conn != nil {
			conn.Close()
		}
	})

	client := attesterv1.NewAttesterServiceClient(conn)

	return client
}

func TestAttesterServer_SubmitBlock_SuccessLeader(t *testing.T) {
	mockRaft := new(mocks.MockRaftNode)
	mockRaft.MockState = raft.Leader
	mockRaft.ApplyErr = nil

	expectedBlockHeight := uint64(100)
	expectedBlockHash := []byte("test_block_hash_success")
	expectedDataToSign := []byte("data_to_be_signed_success")

	expectedLogEntry := &attesterv1.BlockInfo{
		Height:     expectedBlockHeight,
		Hash:       expectedBlockHash,
		DataToSign: expectedDataToSign,
	}
	expectedLogData, err := proto.Marshal(expectedLogEntry)
	require.NoError(t, err, "Failed to marshal expected BlockInfo")

	mockRaft.On("State").Return(raft.Leader).Once()
	mockRaft.On("Apply", expectedLogData, mock.AnythingOfType("time.Duration")).Return(nil).Once()
	mockAgg := new(mocks.MockAggregator)

	client := setupTestServer(t, mockRaft, mockAgg)

	req := &attesterv1.SubmitBlockRequest{
		BlockHeight: expectedBlockHeight,
		BlockHash:   expectedBlockHash,
		DataToSign:  expectedDataToSign,
	}

	resp, err := client.SubmitBlock(context.Background(), req)

	require.NoError(t, err, "SubmitBlock failed unexpectedly")
	require.NotNil(t, resp, "Response should not be nil")
	require.True(t, resp.Accepted, "Response should indicate acceptance")
	require.Empty(t, resp.ErrorMessage, "Error message should be empty on success")
	require.Empty(t, resp.LeaderHint, "Leader hint should be empty when node is leader")

	mockRaft.AssertExpectations(t)
}

func TestAttesterServer_SubmitBlock_NotLeader(t *testing.T) {
	mockRaft := new(mocks.MockRaftNode)
	mockRaft.MockState = raft.Follower
	expectedLeader := raft.ServerAddress("192.168.1.100:12345")
	mockRaft.MockLeader = expectedLeader

	mockRaft.On("State").Return(raft.Follower) // Allow multiple calls if necessary
	mockRaft.On("Leader").Return(expectedLeader).Once()

	mockAgg := new(mocks.MockAggregator)

	client := setupTestServer(t, mockRaft, mockAgg)

	req := &attesterv1.SubmitBlockRequest{
		BlockHeight: 101,
		BlockHash:   []byte("test_block_hash_follower"),
		DataToSign:  []byte("data_to_be_signed_follower"),
	}

	resp, err := client.SubmitBlock(context.Background(), req)

	require.NoError(t, err, "SubmitBlock should not return a transport-level error when node is not leader")
	require.NotNil(t, resp, "Response should not be nil")
	require.False(t, resp.Accepted, "Response should indicate not accepted")
	require.Contains(t, resp.ErrorMessage, "Node is not the RAFT leader", "Error message should indicate node is not leader")
	require.Equal(t, string(expectedLeader), resp.LeaderHint, "Response should provide the correct leader hint")

	mockRaft.AssertExpectations(t)
	mockRaft.AssertNotCalled(t, "Apply", mock.Anything, mock.Anything)
}

func TestAttesterServer_SubmitBlock_ValidationErrors(t *testing.T) {
	testCases := []struct {
		name           string
		request        *attesterv1.SubmitBlockRequest
		expectedErrMsg string
	}{
		{
			name: "InvalidHeightZero",
			request: &attesterv1.SubmitBlockRequest{
				BlockHeight: 0,
				BlockHash:   []byte("some_hash"),
				DataToSign:  []byte("some_data"),
			},
			expectedErrMsg: "Block height cannot be zero",
		},
		{
			name: "EmptyBlockHash",
			request: &attesterv1.SubmitBlockRequest{
				BlockHeight: 102,
				BlockHash:   []byte{},
				DataToSign:  []byte("some_data"),
			},
			expectedErrMsg: "Block hash cannot be empty",
		},
		{
			name: "NilBlockHash",
			request: &attesterv1.SubmitBlockRequest{
				BlockHeight: 103,
				BlockHash:   nil,
				DataToSign:  []byte("some_data"),
			},
			expectedErrMsg: "Block hash cannot be empty",
		},
		{
			name: "EmptyDataToSign",
			request: &attesterv1.SubmitBlockRequest{
				BlockHeight: 104,
				BlockHash:   []byte("some_hash"),
				DataToSign:  []byte{},
			},
			expectedErrMsg: "Data to sign cannot be empty",
		},
		{
			name: "NilDataToSign",
			request: &attesterv1.SubmitBlockRequest{
				BlockHeight: 105,
				BlockHash:   []byte("some_hash"),
				DataToSign:  nil,
			},
			expectedErrMsg: "Data to sign cannot be empty",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			mockRaft := new(mocks.MockRaftNode)
			mockRaft.MockState = raft.Leader
			mockRaft.On("State").Return(raft.Leader).Once()
			mockAgg := new(mocks.MockAggregator)

			client := setupTestServer(t, mockRaft, mockAgg)

			resp, err := client.SubmitBlock(context.Background(), tc.request)

			require.NoError(t, err, "SubmitBlock should not return a transport-level error for validation failures")
			require.NotNil(t, resp, "Response should not be nil")
			require.False(t, resp.Accepted, "Response should indicate not accepted due to validation error")
			require.Contains(t, resp.ErrorMessage, tc.expectedErrMsg, "Incorrect error message")

			mockRaft.AssertExpectations(t)
			mockRaft.AssertNotCalled(t, "Apply", mock.Anything, mock.Anything)
		})
	}
}

func TestAttesterServer_SubmitBlock_ApplyError(t *testing.T) {
	// Arrange Mocks: Node is leader, but Apply call fails
	mockRaft := new(mocks.MockRaftNode)
	mockRaft.MockState = raft.Leader
	expectedApplyError := fmt.Errorf("simulated RAFT Apply error")
	mockRaft.ApplyErr = expectedApplyError // Configure Apply to return this error

	// Define expected data for the Apply call (must still be correct)
	expectedBlockHeight := uint64(106)
	expectedBlockHash := []byte("test_block_hash_apply_error")
	expectedDataToSign := []byte("data_to_be_signed_apply_error")
	expectedLogEntry := &attesterv1.BlockInfo{
		Height:     expectedBlockHeight,
		Hash:       expectedBlockHash,
		DataToSign: expectedDataToSign,
	}
	expectedLogData, err := proto.Marshal(expectedLogEntry)
	require.NoError(t, err, "Failed to marshal expected BlockInfo")

	// Set mock expectations:
	// State() should be called once.
	// Apply() should be called once with the correct data.
	mockRaft.On("State").Return(raft.Leader).Once()
	mockRaft.On("Apply", expectedLogData, mock.AnythingOfType("time.Duration")).Return(nil).Once()

	mockAgg := new(mocks.MockAggregator)

	// Arrange gRPC Client
	client := setupTestServer(t, mockRaft, mockAgg)

	// Arrange Request
	req := &attesterv1.SubmitBlockRequest{
		BlockHeight: expectedBlockHeight,
		BlockHash:   expectedBlockHash,
		DataToSign:  expectedDataToSign,
	}

	// Act: Call the gRPC method
	resp, err := client.SubmitBlock(context.Background(), req)

	// Assert: Check the response indicates rejection due to Apply error
	require.NoError(t, err, "SubmitBlock should not return a transport-level error for Apply failures")
	require.NotNil(t, resp, "Response should not be nil")
	require.False(t, resp.Accepted, "Response should indicate not accepted due to Apply error")
	require.Contains(t, resp.ErrorMessage, "Failed to commit block via RAFT", "Error message prefix mismatch")
	require.Contains(t, resp.ErrorMessage, expectedApplyError.Error(), "Original apply error mismatch")

	// Verify that mock expectations were met (State and Apply called)
	mockRaft.AssertExpectations(t)
	// mockAgg.AssertExpectations(t) // No calls expected
}

// TODO: Add more test cases:
// - TestAttesterServer_SubmitBlock_ApplyError
// ...
