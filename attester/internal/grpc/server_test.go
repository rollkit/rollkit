package grpc_test

import (
	"context"
	"fmt"
	"io"
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
	"github.com/rollkit/rollkit/attester/internal/fsm"
	grpcServer "github.com/rollkit/rollkit/attester/internal/grpc"
	"github.com/rollkit/rollkit/attester/internal/mocks"
)

const bufSize = 1024 * 1024

// setupTestServer initializes an in-memory gRPC server and returns a client.
func setupTestServer(t *testing.T, mockRaft *mocks.MockRaftNode, mockAgg *mocks.MockAggregator) attesterv1.AttesterServiceClient {
	t.Helper()

	lis := bufconn.Listen(bufSize)
	t.Cleanup(func() { lis.Close() })

	s := grpc.NewServer()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	attesterServer := grpcServer.NewAttesterServer(mockRaft, logger, mockAgg)
	attesterv1.RegisterAttesterServiceServer(s, attesterServer)

	go func() {
		if err := s.Serve(lis); err != nil && err != grpc.ErrServerStopped {
			t.Logf("ERROR in server goroutine: %v", err)
		}
	}()

	time.Sleep(50 * time.Millisecond)

	t.Cleanup(func() { s.GracefulStop() })

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

	// Setup mock for GetConfiguration
	mockConfigFuture := new(mocks.MockConfigurationFuture)
	mockConfigFuture.Err = nil                    // Expect GetConfiguration to succeed
	mockConfigFuture.Config = raft.Configuration{ // Provide a dummy configuration
		Servers: []raft.Server{
			{ID: "leader", Address: "localhost:1"},
		},
	}
	mockRaft.On("GetConfiguration").Return(mockConfigFuture).Maybe() // Allow multiple calls if needed

	expectedBlockHeight := uint64(100)
	expectedBlockHash := []byte("test_block_hash_success")
	expectedDataToSign := []byte("data_to_be_signed_success")
	// The log data now includes the type byte
	expectedSubmitReq := &attesterv1.SubmitBlockRequest{
		BlockHeight: expectedBlockHeight,
		BlockHash:   expectedBlockHash,
		DataToSign:  expectedDataToSign,
	}
	expectedReqData, err := proto.Marshal(expectedSubmitReq)
	require.NoError(t, err)
	expectedLogData := append([]byte{fsm.LogEntryTypeSubmitBlock}, expectedReqData...) // Prepend type byte

	mockRaft.On("State").Return(raft.Leader).Once()
	mockRaft.On("Apply", expectedLogData, mock.AnythingOfType("time.Duration")).Return(new(mocks.MockApplyFuture)).Once() // Return a successful future
	mockAgg := new(mocks.MockAggregator)

	client := setupTestServer(t, mockRaft, mockAgg)

	req := &attesterv1.SubmitBlockRequest{
		BlockHeight: expectedBlockHeight,
		BlockHash:   expectedBlockHash,
		DataToSign:  expectedDataToSign,
	}

	resp, err := client.SubmitBlock(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.True(t, resp.Accepted)
	require.Empty(t, resp.ErrorMessage)
	require.Empty(t, resp.LeaderHint)
	mockRaft.AssertExpectations(t)
}

func TestAttesterServer_SubmitBlock_NotLeader(t *testing.T) {
	mockRaft := new(mocks.MockRaftNode)
	mockRaft.MockState = raft.Follower
	expectedLeader := raft.ServerAddress("192.168.1.100:12345")
	mockRaft.MockLeader = expectedLeader

	mockRaft.On("State").Return(raft.Follower)
	mockRaft.On("Leader").Return(expectedLeader).Once()
	mockAgg := new(mocks.MockAggregator)

	client := setupTestServer(t, mockRaft, mockAgg)

	req := &attesterv1.SubmitBlockRequest{
		BlockHeight: 101,
		BlockHash:   []byte("test_block_hash_follower"),
		DataToSign:  []byte("data_to_be_signed_follower"),
	}

	resp, err := client.SubmitBlock(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.False(t, resp.Accepted)
	require.Contains(t, resp.ErrorMessage, "Node is not the RAFT leader")
	require.Equal(t, string(expectedLeader), resp.LeaderHint)
	mockRaft.AssertExpectations(t)
	mockRaft.AssertNotCalled(t, "Apply", mock.Anything, mock.Anything)
}

// TestAttesterServer_SubmitBlock_ValidationErrors covers various input validation failures.
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
				BlockHash:   []byte("hash"),
				DataToSign:  []byte("data"),
			},
			expectedErrMsg: "Block height cannot be zero",
		},
		{
			name: "EmptyBlockHash",
			request: &attesterv1.SubmitBlockRequest{
				BlockHeight: 102,
				BlockHash:   []byte{},
				DataToSign:  []byte("data"),
			},
			expectedErrMsg: "Block hash cannot be empty",
		},
		{
			name: "NilBlockHash",
			request: &attesterv1.SubmitBlockRequest{
				BlockHeight: 103,
				BlockHash:   nil,
				DataToSign:  []byte("data"),
			},
			expectedErrMsg: "Block hash cannot be empty",
		},
		{
			name: "EmptyDataToSign",
			request: &attesterv1.SubmitBlockRequest{
				BlockHeight: 104,
				BlockHash:   []byte("hash"),
				DataToSign:  []byte{},
			},
			expectedErrMsg: "Data to sign cannot be empty",
		},
		{
			name: "NilDataToSign",
			request: &attesterv1.SubmitBlockRequest{
				BlockHeight: 105,
				BlockHash:   []byte("hash"),
				DataToSign:  nil,
			},
			expectedErrMsg: "Data to sign cannot be empty",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Node must be leader to hit validation logic after leader check.
			mockRaft := new(mocks.MockRaftNode)
			mockRaft.MockState = raft.Leader

			// Setup mock for GetConfiguration (needed even for validation errors if leader)
			mockConfigFuture := new(mocks.MockConfigurationFuture)
			mockConfigFuture.Err = nil
			mockConfigFuture.Config = raft.Configuration{Servers: []raft.Server{{ID: "leader", Address: "localhost:1"}}}
			mockRaft.On("GetConfiguration").Return(mockConfigFuture).Maybe()

			mockRaft.On("State").Return(raft.Leader).Once()
			mockAgg := new(mocks.MockAggregator)

			client := setupTestServer(t, mockRaft, mockAgg)

			resp, err := client.SubmitBlock(context.Background(), tc.request)

			require.NoError(t, err)
			require.NotNil(t, resp)
			require.False(t, resp.Accepted)
			require.Contains(t, resp.ErrorMessage, tc.expectedErrMsg)
			mockRaft.AssertExpectations(t)
			mockRaft.AssertNotCalled(t, "Apply", mock.Anything, mock.Anything)
		})
	}
}

func TestAttesterServer_SubmitBlock_ApplyError(t *testing.T) {
	mockRaft := new(mocks.MockRaftNode)
	mockRaft.MockState = raft.Leader
	expectedApplyError := fmt.Errorf("simulated RAFT Apply error")
	// Setup mock to return an error future for Apply
	mockApplyFuture := new(mocks.MockApplyFuture)
	mockApplyFuture.Err = expectedApplyError
	mockRaft.ApplyFn = func(cmd []byte, timeout time.Duration) raft.ApplyFuture { // Use ApplyFn for clarity
		return mockApplyFuture
	}

	// Setup mock for GetConfiguration
	mockConfigFuture := new(mocks.MockConfigurationFuture)
	mockConfigFuture.Err = nil
	mockConfigFuture.Config = raft.Configuration{Servers: []raft.Server{{ID: "leader", Address: "localhost:1"}}}
	mockRaft.On("GetConfiguration").Return(mockConfigFuture).Maybe()

	expectedBlockHeight := uint64(106)
	expectedBlockHash := []byte("test_block_hash_apply_error")
	expectedDataToSign := []byte("data_to_be_signed_apply_error")
	// Construct expected log data correctly
	expectedSubmitReq := &attesterv1.SubmitBlockRequest{
		BlockHeight: expectedBlockHeight,
		BlockHash:   expectedBlockHash,
		DataToSign:  expectedDataToSign,
	}
	expectedReqData, err := proto.Marshal(expectedSubmitReq)
	require.NoError(t, err)
	expectedLogData := append([]byte{fsm.LogEntryTypeSubmitBlock}, expectedReqData...)

	mockRaft.On("State").Return(raft.Leader).Once()
	// Expect Apply to be called, but return the error future
	mockRaft.On("Apply", expectedLogData, mock.AnythingOfType("time.Duration")).Return(mockApplyFuture).Once()
	mockAgg := new(mocks.MockAggregator)

	client := setupTestServer(t, mockRaft, mockAgg)

	req := &attesterv1.SubmitBlockRequest{
		BlockHeight: expectedBlockHeight,
		BlockHash:   expectedBlockHash,
		DataToSign:  expectedDataToSign,
	}

	resp, err := client.SubmitBlock(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.False(t, resp.Accepted)
	require.Contains(t, resp.ErrorMessage, "Failed to commit block via RAFT")
	require.Contains(t, resp.ErrorMessage, expectedApplyError.Error())
	mockRaft.AssertExpectations(t)
}

func TestAttesterServer_SubmitSignature_Success(t *testing.T) {
	mockRaft := new(mocks.MockRaftNode)
	mockAgg := new(mocks.MockAggregator)

	expectedBlockHeight := uint64(200)
	expectedBlockHash := []byte("test_block_hash_sig_success")
	expectedAttesterID := "attester-1"
	expectedSignature := []byte("test_signature_success")

	// Expect AddSignature to succeed, assume quorum not reached.
	mockAgg.On("AddSignature",
		expectedBlockHeight,
		expectedBlockHash,
		expectedAttesterID,
		expectedSignature,
	).Return(false, nil).Once()

	client := setupTestServer(t, mockRaft, mockAgg)

	req := &attesterv1.SubmitSignatureRequest{
		BlockHeight: expectedBlockHeight,
		BlockHash:   expectedBlockHash,
		AttesterId:  expectedAttesterID,
		Signature:   expectedSignature,
	}

	resp, err := client.SubmitSignature(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.True(t, resp.Success)
	mockAgg.AssertExpectations(t)
}

// TestAttesterServer_SubmitSignature_ValidationErrors covers input validation failures.
func TestAttesterServer_SubmitSignature_ValidationErrors(t *testing.T) {
	testCases := []struct {
		name           string
		request        *attesterv1.SubmitSignatureRequest
		expectedErrMsg string
	}{
		{
			name: "InvalidHeightZero",
			request: &attesterv1.SubmitSignatureRequest{
				BlockHeight: 0,
				BlockHash:   []byte("hash"),
				AttesterId:  "id",
				Signature:   []byte("sig"),
			},
			expectedErrMsg: "block height cannot be zero",
		},
		{
			name: "EmptyBlockHash",
			request: &attesterv1.SubmitSignatureRequest{
				BlockHeight: 201,
				BlockHash:   []byte{},
				AttesterId:  "id",
				Signature:   []byte("sig"),
			},
			expectedErrMsg: "block hash cannot be empty",
		},
		{
			name: "NilBlockHash",
			request: &attesterv1.SubmitSignatureRequest{
				BlockHeight: 202,
				BlockHash:   nil,
				AttesterId:  "id",
				Signature:   []byte("sig"),
			},
			expectedErrMsg: "block hash cannot be empty",
		},
		{
			name: "EmptyAttesterID",
			request: &attesterv1.SubmitSignatureRequest{
				BlockHeight: 203,
				BlockHash:   []byte("hash"),
				AttesterId:  "",
				Signature:   []byte("sig"),
			},
			expectedErrMsg: "attester ID cannot be empty",
		},
		{
			name: "EmptySignature",
			request: &attesterv1.SubmitSignatureRequest{
				BlockHeight: 204,
				BlockHash:   []byte("hash"),
				AttesterId:  "id",
				Signature:   []byte{},
			},
			expectedErrMsg: "signature cannot be empty",
		},
		{
			name: "NilSignature",
			request: &attesterv1.SubmitSignatureRequest{
				BlockHeight: 205,
				BlockHash:   []byte("hash"),
				AttesterId:  "id",
				Signature:   nil,
			},
			expectedErrMsg: "signature cannot be empty",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			mockRaft := new(mocks.MockRaftNode)
			mockAgg := new(mocks.MockAggregator)
			client := setupTestServer(t, mockRaft, mockAgg)

			resp, err := client.SubmitSignature(context.Background(), tc.request)

			// Assert based on current implementation returning fmt.Errorf on validation fail.
			// TODO: Consider returning gRPC status errors (codes.InvalidArgument) instead.
			require.Error(t, err)
			require.Nil(t, resp)
			require.ErrorContains(t, err, tc.expectedErrMsg)
			mockAgg.AssertNotCalled(t, "AddSignature", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

func TestAttesterServer_SubmitSignature_AggregatorError(t *testing.T) {
	mockRaft := new(mocks.MockRaftNode)
	mockAgg := new(mocks.MockAggregator)
	expectedAggError := fmt.Errorf("simulated aggregator error")

	expectedBlockHeight := uint64(206)
	expectedBlockHash := []byte("test_block_hash_agg_error")
	expectedAttesterID := "attester-2"
	expectedSignature := []byte("test_signature_agg_error")

	// Expect AddSignature to be called and return an error.
	mockAgg.On("AddSignature",
		expectedBlockHeight,
		expectedBlockHash,
		expectedAttesterID,
		expectedSignature,
	).Return(false, expectedAggError).Once()

	client := setupTestServer(t, mockRaft, mockAgg)

	req := &attesterv1.SubmitSignatureRequest{
		BlockHeight: expectedBlockHeight,
		BlockHash:   expectedBlockHash,
		AttesterId:  expectedAttesterID,
		Signature:   expectedSignature,
	}

	resp, err := client.SubmitSignature(context.Background(), req)

	require.Error(t, err)
	require.Nil(t, resp)
	require.ErrorContains(t, err, "failed to process signature")
	require.ErrorContains(t, err, expectedAggError.Error())
	mockAgg.AssertExpectations(t)
}
