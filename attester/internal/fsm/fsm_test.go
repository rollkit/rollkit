package fsm

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/hashicorp/raft"
	"github.com/stretchr/testify/mock"

	"github.com/rollkit/rollkit/attester/internal/mocks"
	"github.com/rollkit/rollkit/attester/internal/signing"
	"github.com/rollkit/rollkit/attester/internal/state"
	"github.com/rollkit/rollkit/attester/internal/verification"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	attesterv1 "github.com/rollkit/rollkit/attester/api/gen/attester/v1"
)

type mockSigner struct {
	signShouldError   bool
	signatureToReturn []byte
	publicKeyToReturn []byte
	schemeToReturn    string
}

func newMockSigner(shouldError bool) *mockSigner {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	return &mockSigner{
		signShouldError:   shouldError,
		signatureToReturn: []byte("mock-signature"),
		publicKeyToReturn: pub,
		schemeToReturn:    "mock-scheme",
	}
}

func (m *mockSigner) Sign(data []byte) ([]byte, error) {
	if m.signShouldError {
		return nil, fmt.Errorf("mock signer error")
	}
	return m.signatureToReturn, nil
}

func (m *mockSigner) PublicKey() []byte {
	return m.publicKeyToReturn
}

func (m *mockSigner) Scheme() string {
	return m.schemeToReturn
}

var _ signing.Signer = (*mockSigner)(nil)

type mockSignatureClient struct {
	mock.Mock
}

func (m *mockSignatureClient) SubmitSignature(ctx context.Context, height uint64, hash []byte, attesterID string, signature []byte) error {
	args := m.Called(ctx, height, hash, attesterID, signature)
	return args.Error(0)
}

var _ SignatureSubmitter = (*mockSignatureClient)(nil)

// Helper to create a raft log entry with a marshaled request
func createTestLogEntry(t *testing.T, height uint64, hash []byte, dataToSign []byte) *raft.Log {
	t.Helper()
	req := &attesterv1.SubmitBlockRequest{
		BlockHeight: height,
		BlockHash:   hash,
		DataToSign:  dataToSign,
	}
	reqBytes, err := proto.Marshal(req)
	require.NoError(t, err, "Failed to marshal SubmitBlockRequest")

	logData := append([]byte{LogEntryTypeSubmitBlock}, reqBytes...)

	return &raft.Log{
		Index: 1,
		Term:  1,
		Type:  raft.LogCommand,
		Data:  logData,
	}
}

// Helper to create a deterministic hash for testing
func testHash(seed byte) state.BlockHash {
	var hash state.BlockHash
	for i := range hash {
		hash[i] = seed + byte(i)
	}
	return hash
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestLeaderFSM(t *testing.T, logger *slog.Logger, signer signing.Signer, nodeID string, agg AggregatorService) *AttesterFSM {
	t.Helper()
	fsm, err := NewAttesterFSM(logger, signer, nodeID, true, agg, nil, verification.NewNoOpVerifier())
	// Allow specific test cases to expect errors from NewAttesterFSM
	if err != nil {
		// Check if the test function name indicates an error is expected
		if !(assert.Contains(t, t.Name(), "Error") || assert.Contains(t, t.Name(), "Panic")) {
			t.Fatalf("NewAttesterFSM (leader) failed unexpectedly in test %s: %v", t.Name(), err)
		}
		// If error is expected, return nil FSM so the test can assert on the error
		return nil
	}
	require.NotNil(t, fsm, "NewAttesterFSM (leader) should return a non-nil FSM when no error occurs")
	require.True(t, fsm.isLeader, "FSM created with newTestLeaderFSM should be leader")
	require.NotNil(t, fsm.aggregator, "Leader FSM should have a non-nil aggregator")
	require.Nil(t, fsm.sigClient, "Leader FSM should have a nil sigClient")
	return fsm
}

func newTestFollowerFSM(t *testing.T, logger *slog.Logger, signer signing.Signer, nodeID string, client SignatureSubmitter) *AttesterFSM {
	t.Helper()
	// Follower still needs an AggregatorService, but it can be nil or a dummy mock
	// since it shouldn't be used. Passing nil is consistent with NewAttesterFSM validation.
	fsm, err := NewAttesterFSM(logger, signer, nodeID, false, nil, client, verification.NewNoOpVerifier())
	// Allow specific test cases to expect errors from NewAttesterFSM
	if err != nil {
		// Check if the test function name indicates an error is expected
		if !(assert.Contains(t, t.Name(), "Error") || assert.Contains(t, t.Name(), "Panic")) {
			t.Fatalf("NewAttesterFSM (follower) failed unexpectedly in test %s: %v", t.Name(), err)
		}
		// If error is expected, return nil FSM so the test can assert on the error
		return nil
	}
	require.NotNil(t, fsm, "NewAttesterFSM (follower) should return a non-nil FSM when no error occurs")
	require.False(t, fsm.isLeader, "FSM created with newTestFollowerFSM should be follower")
	require.Nil(t, fsm.aggregator, "Follower FSM should have a nil aggregator")
	require.NotNil(t, fsm.sigClient, "Follower FSM should have a non-nil sigClient")
	return fsm
}

func TestFSM_Apply_SubmitBlock_Success_Leader(t *testing.T) {
	mockSigner := newMockSigner(false)
	mockSigClient := new(mockSignatureClient)
	mockAgg := new(mocks.MockAggregator) // Use central mock
	logger := discardLogger()
	nodeID := "test-node-submit"
	fsm := newTestLeaderFSM(t, logger, mockSigner, nodeID, mockAgg)

	height := uint64(101)
	hash := testHash(2)
	dataToSign := []byte("data for block 101")
	logEntry := createTestLogEntry(t, height, hash[:], dataToSign)

	mockAgg.On("SetBlockData", hash[:], dataToSign).Return().Once()
	// Expect the leader to also add its own signature
	mockAgg.On("AddSignature", height, hash[:], nodeID, mockSigner.signatureToReturn).Return(false, nil).Once()

	applyResponse := fsm.Apply(logEntry)
	_, ok := applyResponse.(*state.BlockInfo)
	require.True(t, ok)

	time.Sleep(50 * time.Millisecond)

	mockAgg.AssertExpectations(t)
	mockSigClient.AssertExpectations(t)

	mockSigClient.AssertNotCalled(t, "SubmitSignature", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestFSM_Apply_SubmitBlock_Success_Follower(t *testing.T) {
	mockSigner := newMockSigner(false)
	logger := discardLogger()
	mockSigClient := new(mockSignatureClient) // Create mock client

	fsm := newTestFollowerFSM(t, logger, mockSigner, "test-node", mockSigClient)

	mockSigClient.On("SubmitSignature", mock.Anything, uint64(100), mock.AnythingOfType("[]uint8"), "test-node", mockSigner.signatureToReturn).Return(nil).Once()

	height := uint64(100)
	hash := testHash(1)
	dataToSign := []byte("data for block 100")
	logEntry := createTestLogEntry(t, height, hash[:], dataToSign)

	applyResponse := fsm.Apply(logEntry)

	blockInfo, ok := applyResponse.(*state.BlockInfo)
	require.True(t, ok, "Apply response should be of type *state.BlockInfo")
	require.NotNil(t, blockInfo)
	assert.Equal(t, height, blockInfo.Height)
	assert.Equal(t, hash, blockInfo.Hash)
	assert.Equal(t, dataToSign, blockInfo.DataToSign)

	fsm.mu.RLock()
	defer fsm.mu.RUnlock()

	savedHash, heightExists := fsm.processedBlocks[height]
	assert.True(t, heightExists)
	assert.Equal(t, hash, savedHash)
	savedInfo, hashExists := fsm.blockDetails[hash]
	assert.True(t, hashExists)
	assert.Equal(t, blockInfo, savedInfo)

	// Allow time for goroutine
	time.Sleep(50 * time.Millisecond)
	mockSigClient.AssertExpectations(t)
}

func TestFSM_Apply_SubmitBlock_DuplicateHeight(t *testing.T) {
	mockSigner := newMockSigner(false)
	logger := discardLogger()
	mockSigClient := new(mockSignatureClient) // Create mock client
	// Provide nil aggregator, but mock client for follower
	fsm := newTestFollowerFSM(t, logger, mockSigner, "test-node-dup", mockSigClient) // Use follower helper

	// Expect SubmitSignature calls for the first successful Apply only
	mockSigClient.On("SubmitSignature", mock.Anything, uint64(103), mock.AnythingOfType("[]uint8"), "test-node-dup", mockSigner.signatureToReturn).Return(nil).Once()

	height := uint64(103)
	hash1 := testHash(4)
	hash2 := testHash(5)
	dataToSign1 := []byte("data1")
	dataToSign2 := []byte("data2")

	logEntry1 := createTestLogEntry(t, height, hash1[:], dataToSign1)
	logEntry2 := createTestLogEntry(t, height, hash2[:], dataToSign2)

	applyResponse1 := fsm.Apply(logEntry1)
	_, ok1 := applyResponse1.(*state.BlockInfo)
	require.True(t, ok1)

	applyResponse2 := fsm.Apply(logEntry2)
	assert.Nil(t, applyResponse2)

	fsm.mu.RLock()
	defer fsm.mu.RUnlock()
	assert.Equal(t, 1, len(fsm.processedBlocks))
	assert.Equal(t, hash1, fsm.processedBlocks[height])
	assert.Equal(t, 1, len(fsm.blockDetails))
	_, detailExists := fsm.blockDetails[hash2]
	assert.False(t, detailExists)

	// Allow time for goroutine
	time.Sleep(50 * time.Millisecond)
	mockSigClient.AssertExpectations(t)
}

func TestFSM_Apply_SubmitBlock_InvalidHashSize(t *testing.T) {
	mockSigner := newMockSigner(false)
	logger := discardLogger()
	mockSigClient := new(mockSignatureClient)
	fsm := newTestFollowerFSM(t, logger, mockSigner, "test-node-invhash", mockSigClient)

	height := uint64(104)
	invalidHash := []byte{0x01, 0x02, 0x03}
	dataToSign := []byte("data")
	logEntry := createTestLogEntry(t, height, invalidHash, dataToSign)

	applyResponse := fsm.Apply(logEntry)

	err, ok := applyResponse.(error)
	assert.True(t, ok)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid block hash size")

	fsm.mu.RLock()
	defer fsm.mu.RUnlock()
	assert.Empty(t, fsm.processedBlocks)
	assert.Empty(t, fsm.blockDetails)

	mockSigClient.AssertNotCalled(t, "SubmitSignature", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestFSM_Apply_SubmitBlock_DuplicateHash(t *testing.T) {
	mockSigner := newMockSigner(false)
	logger := discardLogger()
	mockSigClient := new(mockSignatureClient)
	fsm := newTestFollowerFSM(t, logger, mockSigner, "test-node-duphash", mockSigClient)

	mockSigClient.On("SubmitSignature", mock.Anything, uint64(105), mock.AnythingOfType("[]uint8"), "test-node-duphash", mockSigner.signatureToReturn).Return(nil).Once()

	height1 := uint64(105)
	height2 := uint64(106)
	hash := testHash(6)
	dataToSign1 := []byte("data1")
	dataToSign2 := []byte("data2")

	logEntry1 := createTestLogEntry(t, height1, hash[:], dataToSign1)
	logEntry2 := createTestLogEntry(t, height2, hash[:], dataToSign2)

	applyResponse1 := fsm.Apply(logEntry1)
	_, ok1 := applyResponse1.(*state.BlockInfo)
	require.True(t, ok1)

	applyResponse2 := fsm.Apply(logEntry2)
	err, ok := applyResponse2.(error)
	assert.True(t, ok)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "collision or duplicate")

	fsm.mu.RLock()
	defer fsm.mu.RUnlock()
	assert.Equal(t, 1, len(fsm.processedBlocks))
	assert.Equal(t, 1, len(fsm.blockDetails))
	assert.Equal(t, hash, fsm.processedBlocks[height1])
	_, height2Exists := fsm.processedBlocks[height2]
	assert.False(t, height2Exists)

	// Allow time for goroutine
	time.Sleep(50 * time.Millisecond)
	mockSigClient.AssertExpectations(t)
}

func TestFSM_Apply_UnknownLogType(t *testing.T) {
	mockSigner := newMockSigner(false)
	logger := discardLogger()
	mockSigClient := new(mockSignatureClient)
	fsm := newTestFollowerFSM(t, logger, mockSigner, "test-node-unknown", mockSigClient)

	logEntry := &raft.Log{
		Index: 1, Term: 1, Type: raft.LogCommand,
		Data: []byte{0x99, 0x01, 0x02, 0x03},
	}

	applyResponse := fsm.Apply(logEntry)
	err, ok := applyResponse.(error)
	assert.True(t, ok)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown log entry type")

	mockSigClient.AssertNotCalled(t, "SubmitSignature", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestFSM_Apply_EmptyLogData(t *testing.T) {
	mockSigner := newMockSigner(false)
	logger := discardLogger()
	mockSigClient := new(mockSignatureClient)
	fsm := newTestFollowerFSM(t, logger, mockSigner, "test-node-empty", mockSigClient)

	logEntry := &raft.Log{
		Index: 1, Term: 1, Type: raft.LogCommand,
		Data: []byte{},
	}

	applyResponse := fsm.Apply(logEntry)
	err, ok := applyResponse.(error)
	assert.True(t, ok)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty log data")

	mockSigClient.AssertNotCalled(t, "SubmitSignature", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestFSM_Apply_SignerError(t *testing.T) {
	mockSigner := newMockSigner(true)
	logger := discardLogger()
	mockSigClient := new(mockSignatureClient)
	fsm := newTestFollowerFSM(t, logger, mockSigner, "test-node-signerr", mockSigClient)

	height := uint64(107)
	hash := testHash(7)
	dataToSign := []byte("data")
	logEntry := createTestLogEntry(t, height, hash[:], dataToSign)

	applyResponse := fsm.Apply(logEntry)

	err, ok := applyResponse.(error)
	assert.True(t, ok)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to sign data")

	fsm.mu.RLock()
	defer fsm.mu.RUnlock()
	assert.Empty(t, fsm.processedBlocks)
	assert.Empty(t, fsm.blockDetails)

	mockSigClient.AssertNotCalled(t, "SubmitSignature", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// Mock SnapshotSink
type mockSnapshotSink struct {
	buf bytes.Buffer
	id  string
}

func (m *mockSnapshotSink) Write(p []byte) (n int, err error) {
	return m.buf.Write(p)
}

func (m *mockSnapshotSink) Close() error {
	return nil // Indicate success
}

func (m *mockSnapshotSink) ID() string {
	return m.id
}

func (m *mockSnapshotSink) Cancel() error {
	return fmt.Errorf("snapshot cancelled")
}

// Test Snapshot and Restore
func TestFSM_Snapshot_Restore(t *testing.T) {
	mockSigner := newMockSigner(false)
	logger := discardLogger()
	mockSigClient := new(mockSignatureClient)
	fsm1 := newTestFollowerFSM(t, logger, mockSigner, "node1", mockSigClient)

	mockSigClient.On("SubmitSignature", mock.Anything, uint64(200), mock.AnythingOfType("[]uint8"), "node1", mockSigner.signatureToReturn).Return(nil).Once()
	mockSigClient.On("SubmitSignature", mock.Anything, uint64(201), mock.AnythingOfType("[]uint8"), "node1", mockSigner.signatureToReturn).Return(nil).Once()

	height1, hash1, data1 := uint64(200), testHash(10), []byte("block200")
	height2, hash2, data2 := uint64(201), testHash(11), []byte("block201")
	applyResp1 := fsm1.Apply(createTestLogEntry(t, height1, hash1[:], data1))
	require.NotNil(t, applyResp1)
	assert.NotImplements(t, (*error)(nil), applyResp1, "Apply 1 should not return an error")
	applyResp2 := fsm1.Apply(createTestLogEntry(t, height2, hash2[:], data2))
	require.NotNil(t, applyResp2)
	assert.NotImplements(t, (*error)(nil), applyResp2, "Apply 2 should not return an error")

	time.Sleep(50 * time.Millisecond)
	mockSigClient.AssertExpectations(t)

	snapshot, err := fsm1.Snapshot()
	require.NoError(t, err, "Snapshot should succeed")
	require.NotNil(t, snapshot)

	sink := &mockSnapshotSink{id: "snap1"}
	err = snapshot.Persist(sink)
	require.NoError(t, err, "Persist should succeed")
	snapshot.Release()

	fsm2 := newTestFollowerFSM(t, logger, mockSigner, "node2", new(mockSignatureClient))
	err = fsm2.Restore(io.NopCloser(&sink.buf))
	require.NoError(t, err, "Restore should succeed")

	fsm1.mu.RLock()
	fsm2.mu.RLock()
	defer fsm1.mu.RUnlock()
	defer fsm2.mu.RUnlock()

	assert.Equal(t, fsm1.processedBlocks, fsm2.processedBlocks, "Restored processedBlocks should match")
	assert.Equal(t, fsm1.blockDetails, fsm2.blockDetails, "Restored blockDetails should match")
	assert.Len(t, fsm2.processedBlocks, 2, "Restored fsm should have 2 processed blocks")
	assert.Len(t, fsm2.blockDetails, 2, "Restored fsm should have 2 block details")

	assert.Equal(t, hash1, fsm2.processedBlocks[height1])
	assert.Equal(t, hash2, fsm2.processedBlocks[height2])
	assert.Equal(t, data1, fsm2.blockDetails[hash1].DataToSign)
	assert.Equal(t, data2, fsm2.blockDetails[hash2].DataToSign)
}

func TestNewAttesterFSM_Errors(t *testing.T) {
	logger := discardLogger()
	mockSigner := newMockSigner(false)
	mockAgg := new(mocks.MockAggregator)
	mockClient := new(mockSignatureClient)
	noopVerifier := verification.NewNoOpVerifier() // Reuse verifier

	tests := []struct {
		name        string
		logger      *slog.Logger
		signer      signing.Signer
		nodeID      string
		isLeader    bool
		agg         AggregatorService
		client      SignatureSubmitter
		verifier    verification.ExecutionVerifier
		expectError bool
	}{
		{"NilLogger", nil, mockSigner, "id", false, nil, mockClient, noopVerifier, true},
		{"EmptyNodeID", logger, mockSigner, "", false, nil, mockClient, noopVerifier, true},
		{"LeaderNilAgg", logger, mockSigner, "id", true, nil, nil, noopVerifier, true},
		{"FollowerNilClient", logger, mockSigner, "id", false, nil, nil, noopVerifier, true},
		// Nil verifier is allowed, should not error
		{"NilVerifierLeader", logger, mockSigner, "id", true, mockAgg, nil, nil, false},
		{"NilVerifierFollower", logger, mockSigner, "id", false, nil, mockClient, nil, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewAttesterFSM(tc.logger, tc.signer, tc.nodeID, tc.isLeader, tc.agg, tc.client, tc.verifier)
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNewAttesterFSM_Success(t *testing.T) {
	logger := discardLogger()
	mockSigner := newMockSigner(false)
	mockAgg := new(mocks.MockAggregator)
	mockClient := new(mockSignatureClient)
	noopVerifier := verification.NewNoOpVerifier()
	nodeID := "test-node-success"

	// Test leader creation
	t.Run("Leader", func(t *testing.T) {
		// Added noopVerifier
		fsm, err := NewAttesterFSM(logger, mockSigner, nodeID, true, mockAgg, nil, noopVerifier)
		require.NoError(t, err)
		require.NotNil(t, fsm)
		assert.True(t, fsm.isLeader)
		assert.Equal(t, nodeID, fsm.nodeID)
		assert.Equal(t, mockSigner, fsm.signer)
		assert.Equal(t, mockAgg, fsm.aggregator)
		assert.Nil(t, fsm.sigClient)
		assert.Equal(t, noopVerifier, fsm.verifier)
	})

	// Test follower creation
	t.Run("Follower", func(t *testing.T) {
		// Added noopVerifier
		fsm, err := NewAttesterFSM(logger, mockSigner, nodeID, false, nil, mockClient, noopVerifier)
		require.NoError(t, err)
		require.NotNil(t, fsm)
		assert.False(t, fsm.isLeader)
		assert.Equal(t, nodeID, fsm.nodeID)
		assert.Equal(t, mockSigner, fsm.signer)
		assert.Nil(t, fsm.aggregator)
		assert.Equal(t, mockClient, fsm.sigClient)
		assert.Equal(t, noopVerifier, fsm.verifier)
	})
}
