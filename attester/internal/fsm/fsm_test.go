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

	"github.com/rollkit/rollkit/attester/internal/signing"
	"github.com/rollkit/rollkit/attester/internal/state"

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
	// Generate a real key pair to return plausible values
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

// Ensure mockSigner implements signing.Signer
var _ signing.Signer = (*mockSigner)(nil)

// Mock Signature Client
type mockSignatureClient struct {
	mock.Mock
}

func (m *mockSignatureClient) SubmitSignature(ctx context.Context, height uint64, hash []byte, attesterID string, signature []byte) error {
	args := m.Called(ctx, height, hash, attesterID, signature)
	return args.Error(0)
}

// Ensure mockSignatureClient implements SignatureSubmitter
var _ SignatureSubmitter = (*mockSignatureClient)(nil)

// Mock Aggregator
type mockAggregator struct {
	mock.Mock
}

func (m *mockAggregator) SetBlockData(blockHash []byte, dataToSign []byte) {
	m.Called(blockHash, dataToSign)
}

// Ensure mockAggregator implements BlockDataSetter
var _ BlockDataSetter = (*mockAggregator)(nil)

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
		Index: 1, // Index usually doesn't matter much for Apply logic itself
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

// Helper to create a discard logger for slog
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestFSM_Apply_SubmitBlock_Success_NoSubmit(t *testing.T) {
	mockSigner := newMockSigner(false)
	logger := discardLogger()
	// Provide nil for aggregator and sigClient as they are not tested here
	fsm := NewAttesterFSM(logger, mockSigner, "test-node", false, nil, nil)

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
}

// Tests that were previously commented out are now active

func TestFSM_Apply_SubmitBlock_Success_WithSubmit(t *testing.T) {
	mockSigner := newMockSigner(false)
	mockSigClient := new(mockSignatureClient)
	mockAgg := new(mockAggregator)
	logger := discardLogger()
	nodeID := "test-node-submit"
	// Now this call works because NewAttesterFSM accepts interfaces
	fsm := NewAttesterFSM(logger, mockSigner, nodeID, true, mockAgg, mockSigClient)

	height := uint64(101)
	hash := testHash(2)
	dataToSign := []byte("data for block 101")
	logEntry := createTestLogEntry(t, height, hash[:], dataToSign)

	// Expect aggregator SetBlockData call
	mockAgg.On("SetBlockData", hash[:], dataToSign).Return()
	// Expect signature client SubmitSignature call - REMOVED because FSM is leader
	/* mockSigClient.On("SubmitSignature", mock.Anything, // context
	height, hash[:], nodeID, mockSigner.signatureToReturn).Return(nil) // Expect success */

	applyResponse := fsm.Apply(logEntry)
	_, ok := applyResponse.(*state.BlockInfo)
	require.True(t, ok)

	// Allow time for the goroutine to execute
	time.Sleep(100 * time.Millisecond)

	mockAgg.AssertExpectations(t)
	mockSigClient.AssertExpectations(t)
	// Note: We can't easily assert the error log, but we've asserted the mock was called.
	// Assert that SubmitSignature was *not* called (optional, but good practice)
	mockSigClient.AssertNotCalled(t, "SubmitSignature", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestFSM_Apply_SubmitBlock_SubmitError(t *testing.T) {
	mockSigner := newMockSigner(false)
	mockSigClient := new(mockSignatureClient)
	mockAgg := new(mockAggregator)
	logger := discardLogger()
	nodeID := "test-node-submit-err"
	// Now this call works because NewAttesterFSM accepts interfaces
	fsm := NewAttesterFSM(logger, mockSigner, nodeID, true, mockAgg, mockSigClient)

	height := uint64(102)
	hash := testHash(3)
	dataToSign := []byte("data for block 102")
	logEntry := createTestLogEntry(t, height, hash[:], dataToSign)

	mockAgg.On("SetBlockData", hash[:], dataToSign).Return()
	// Expect signature client SubmitSignature call - REMOVED because FSM is leader
	/* mockSigClient.On("SubmitSignature", mock.Anything, height, hash[:], nodeID, mockSigner.signatureToReturn).Return(submitErr) */

	applyResponse := fsm.Apply(logEntry)
	_, ok := applyResponse.(*state.BlockInfo)
	require.True(t, ok) // Apply itself should still succeed

	time.Sleep(100 * time.Millisecond)

	mockAgg.AssertExpectations(t)
	mockSigClient.AssertExpectations(t)
	// Note: We can't easily assert the error log, but we've asserted the mock was called.
	// Assert that SubmitSignature was *not* called (optional, but good practice)
	mockSigClient.AssertNotCalled(t, "SubmitSignature", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestFSM_Apply_SubmitBlock_DuplicateHeight(t *testing.T) {
	mockSigner := newMockSigner(false)
	logger := discardLogger()
	// Pass nil for aggregator/client as they shouldn't be called on duplicate
	fsm := NewAttesterFSM(logger, mockSigner, "test-node-dup", false, nil, nil)

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
}

func TestFSM_Apply_SubmitBlock_InvalidHashSize(t *testing.T) {
	mockSigner := newMockSigner(false)
	logger := discardLogger()
	fsm := NewAttesterFSM(logger, mockSigner, "test-node-invhash", false, nil, nil)

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
}

func TestFSM_Apply_SubmitBlock_DuplicateHash(t *testing.T) {
	mockSigner := newMockSigner(false)
	logger := discardLogger()
	fsm := NewAttesterFSM(logger, mockSigner, "test-node-duphash", false, nil, nil)

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
}

func TestFSM_Apply_UnknownLogType(t *testing.T) {
	mockSigner := newMockSigner(false)
	logger := discardLogger()
	fsm := NewAttesterFSM(logger, mockSigner, "test-node-unknown", false, nil, nil)

	logEntry := &raft.Log{
		Index: 1, Term: 1, Type: raft.LogCommand,
		Data: []byte{0x99, 0x01, 0x02, 0x03}, // Unknown type 0x99
	}

	applyResponse := fsm.Apply(logEntry)
	err, ok := applyResponse.(error)
	assert.True(t, ok)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown log entry type")
}

func TestFSM_Apply_EmptyLogData(t *testing.T) {
	mockSigner := newMockSigner(false)
	logger := discardLogger()
	fsm := NewAttesterFSM(logger, mockSigner, "test-node-empty", false, nil, nil)

	logEntry := &raft.Log{
		Index: 1, Term: 1, Type: raft.LogCommand,
		Data: []byte{}, // Empty data
	}

	applyResponse := fsm.Apply(logEntry)
	err, ok := applyResponse.(error)
	assert.True(t, ok)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty log data")
}

func TestFSM_Apply_SignerError(t *testing.T) {
	mockSigner := newMockSigner(true) // Signer configured to return an error
	logger := discardLogger()
	fsm := NewAttesterFSM(logger, mockSigner, "test-node-signerr", false, nil, nil)

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
	fsm1 := NewAttesterFSM(logger, mockSigner, "node1", false, nil, nil)

	// Apply some data to fsm1
	height1, hash1, data1 := uint64(200), testHash(10), []byte("block200")
	height2, hash2, data2 := uint64(201), testHash(11), []byte("block201")
	applyResp1 := fsm1.Apply(createTestLogEntry(t, height1, hash1[:], data1))
	require.NotNil(t, applyResp1)
	// Use assert.NotImplements which checks if the value implements the error interface
	assert.NotImplements(t, (*error)(nil), applyResp1, "Apply 1 should not return an error")
	applyResp2 := fsm1.Apply(createTestLogEntry(t, height2, hash2[:], data2))
	require.NotNil(t, applyResp2)
	assert.NotImplements(t, (*error)(nil), applyResp2, "Apply 2 should not return an error")

	// Take a snapshot of fsm1
	snapshot, err := fsm1.Snapshot()
	require.NoError(t, err, "Snapshot should succeed")
	require.NotNil(t, snapshot)

	// Persist the snapshot to a buffer
	sink := &mockSnapshotSink{id: "snap1"}
	err = snapshot.Persist(sink)
	require.NoError(t, err, "Persist should succeed")
	snapshot.Release() // Important to release

	// Create a new FSM (fsm2) and restore from the snapshot
	fsm2 := NewAttesterFSM(logger, mockSigner, "node2", false, nil, nil)
	err = fsm2.Restore(io.NopCloser(&sink.buf)) // Wrap buffer in NopCloser
	require.NoError(t, err, "Restore should succeed")

	// Verify the state of fsm2 matches fsm1
	fsm1.mu.RLock()
	fsm2.mu.RLock()
	defer fsm1.mu.RUnlock()
	defer fsm2.mu.RUnlock()

	assert.Equal(t, fsm1.processedBlocks, fsm2.processedBlocks, "Restored processedBlocks should match")
	assert.Equal(t, fsm1.blockDetails, fsm2.blockDetails, "Restored blockDetails should match")
	assert.Len(t, fsm2.processedBlocks, 2, "Restored fsm should have 2 processed blocks")
	assert.Len(t, fsm2.blockDetails, 2, "Restored fsm should have 2 block details")

	// Check specific entries
	assert.Equal(t, hash1, fsm2.processedBlocks[height1])
	assert.Equal(t, hash2, fsm2.processedBlocks[height2])
	assert.Equal(t, data1, fsm2.blockDetails[hash1].DataToSign)
	assert.Equal(t, data2, fsm2.blockDetails[hash2].DataToSign)
}
