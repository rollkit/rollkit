package fsm

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"io"
	"log"
	"testing"

	"github.com/hashicorp/raft"
	"github.com/rollkit/rollkit/attester/internal/signing"
	"github.com/rollkit/rollkit/attester/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	// Import generated protobuf types from the api directory
	attesterv1 "github.com/rollkit/rollkit/attester/api/gen/attesterv1"
)

// --- Mocks --- //

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
	// Return a signature derived from data to make it unique if needed
	// return append(m.signatureToReturn, data...), nil
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

// --- Helpers --- //

// Helper to create a raft log entry with a marshaled request
func createTestLogEntry(t *testing.T, height uint64, hash []byte, dataToSign []byte) *raft.Log {
	t.Helper()
	// Use the generated type now
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

// --- Tests --- //

func TestFSM_Apply_SubmitBlock_Success(t *testing.T) {
	mockSigner := newMockSigner(false)                              // Signer should succeed
	logger := log.New(io.Discard, "", 0)                            // Discard logs
	fsm := NewAttesterFSM(logger, mockSigner, "/tmp/test-fsm-data") // dataDir not used yet

	height := uint64(100)
	hash := testHash(1)
	dataToSign := []byte("data for block 100")
	logEntry := createTestLogEntry(t, height, hash[:], dataToSign)

	applyResponse := fsm.Apply(logEntry)

	// Check response type and value
	blockInfo, ok := applyResponse.(*state.BlockInfo)
	require.True(t, ok, "Apply response should be of type *state.BlockInfo")
	require.NotNil(t, blockInfo)
	assert.Equal(t, height, blockInfo.Height)
	assert.Equal(t, hash, blockInfo.Hash)
	assert.Equal(t, dataToSign, blockInfo.DataToSign)
	assert.Equal(t, mockSigner.signatureToReturn, blockInfo.Signature)

	// Check internal FSM state
	fsm.mu.RLock()
	defer fsm.mu.RUnlock()

	savedHash, heightExists := fsm.processedBlocks[height]
	assert.True(t, heightExists, "Height should be recorded in processedBlocks")
	assert.Equal(t, hash, savedHash, "Hash in processedBlocks should match")

	savedInfo, hashExists := fsm.blockDetails[hash]
	assert.True(t, hashExists, "Hash should be recorded in blockDetails")
	assert.Equal(t, blockInfo, savedInfo, "BlockInfo in blockDetails should match the returned one")
}

func TestFSM_Apply_SubmitBlock_DuplicateHeight(t *testing.T) {
	mockSigner := newMockSigner(false)
	logger := log.New(io.Discard, "", 0)
	fsm := NewAttesterFSM(logger, mockSigner, "")

	height := uint64(101)
	hash1 := testHash(1)
	hash2 := testHash(2) // Different hash, same height
	dataToSign1 := []byte("data1")
	dataToSign2 := []byte("data2")

	logEntry1 := createTestLogEntry(t, height, hash1[:], dataToSign1)
	logEntry2 := createTestLogEntry(t, height, hash2[:], dataToSign2)

	// Apply the first one - should succeed
	applyResponse1 := fsm.Apply(logEntry1)
	_, ok1 := applyResponse1.(*state.BlockInfo)
	require.True(t, ok1, "First apply should return BlockInfo")

	// Apply the second one with the same height - should be ignored
	applyResponse2 := fsm.Apply(logEntry2)
	assert.Nil(t, applyResponse2, "Apply response for duplicate height should be nil")

	// Verify internal state reflects only the first block
	fsm.mu.RLock()
	defer fsm.mu.RUnlock()
	assert.Equal(t, 1, len(fsm.processedBlocks), "Only one height should be processed")
	assert.Equal(t, hash1, fsm.processedBlocks[height], "Stored hash should be from the first apply")
	assert.Equal(t, 1, len(fsm.blockDetails), "Only one block detail should be stored")
	_, detailExists := fsm.blockDetails[hash2]
	assert.False(t, detailExists, "Details for the second hash should not exist")
}

func TestFSM_Apply_SubmitBlock_InvalidHashSize(t *testing.T) {
	mockSigner := newMockSigner(false)
	logger := log.New(io.Discard, "", 0)
	fsm := NewAttesterFSM(logger, mockSigner, "")

	height := uint64(102)
	invalidHash := []byte{0x01, 0x02, 0x03} // Too short
	dataToSign := []byte("data")
	logEntry := createTestLogEntry(t, height, invalidHash, dataToSign)

	applyResponse := fsm.Apply(logEntry)

	// Should return an error
	err, ok := applyResponse.(error)
	assert.True(t, ok, "Apply response should be an error for invalid hash size")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid block hash size", "Error message should mention invalid hash size")

	// Verify internal state is unchanged
	fsm.mu.RLock()
	defer fsm.mu.RUnlock()
	assert.Empty(t, fsm.processedBlocks, "processedBlocks should be empty")
	assert.Empty(t, fsm.blockDetails, "blockDetails should be empty")
}

func TestFSM_Apply_SubmitBlock_DuplicateHash(t *testing.T) {
	mockSigner := newMockSigner(false)
	logger := log.New(io.Discard, "", 0)
	fsm := NewAttesterFSM(logger, mockSigner, "")

	height1 := uint64(103)
	height2 := uint64(104) // Different height, same hash
	hash := testHash(3)
	dataToSign1 := []byte("data1")
	dataToSign2 := []byte("data2")

	logEntry1 := createTestLogEntry(t, height1, hash[:], dataToSign1)
	logEntry2 := createTestLogEntry(t, height2, hash[:], dataToSign2)

	// Apply the first one - should succeed
	applyResponse1 := fsm.Apply(logEntry1)
	_, ok1 := applyResponse1.(*state.BlockInfo)
	require.True(t, ok1, "First apply should return BlockInfo")

	// Apply the second one with the same hash - should error
	applyResponse2 := fsm.Apply(logEntry2)
	err, ok := applyResponse2.(error)
	assert.True(t, ok, "Apply response for duplicate hash should be an error")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "collision or duplicate", "Error message should mention collision or duplicate")

	// Verify internal state reflects only the first block
	fsm.mu.RLock()
	defer fsm.mu.RUnlock()
	assert.Equal(t, 1, len(fsm.processedBlocks), "Only one height should be processed")
	assert.Equal(t, 1, len(fsm.blockDetails), "Only one block detail should be stored")
	assert.Equal(t, hash, fsm.processedBlocks[height1])
	_, height2Exists := fsm.processedBlocks[height2]
	assert.False(t, height2Exists)
}

func TestFSM_Apply_UnknownLogType(t *testing.T) {
	mockSigner := newMockSigner(false)
	logger := log.New(io.Discard, "", 0)
	fsm := NewAttesterFSM(logger, mockSigner, "")

	logEntry := &raft.Log{
		Index: 1, Term: 1, Type: raft.LogCommand,
		Data: []byte{0x99, 0x01, 0x02, 0x03}, // Unknown type 0x99
	}

	applyResponse := fsm.Apply(logEntry)
	err, ok := applyResponse.(error)
	assert.True(t, ok, "Apply response for unknown log type should be an error")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown log entry type", "Error message should mention unknown type")
}

func TestFSM_Apply_EmptyLogData(t *testing.T) {
	mockSigner := newMockSigner(false)
	logger := log.New(io.Discard, "", 0)
	fsm := NewAttesterFSM(logger, mockSigner, "")

	logEntry := &raft.Log{
		Index: 1, Term: 1, Type: raft.LogCommand,
		Data: []byte{}, // Empty data
	}

	applyResponse := fsm.Apply(logEntry)
	err, ok := applyResponse.(error)
	assert.True(t, ok, "Apply response for empty log data should be an error")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty log data", "Error message should mention empty data")
}

func TestFSM_Apply_SignerError(t *testing.T) {
	mockSigner := newMockSigner(true) // Signer configured to return an error
	logger := log.New(io.Discard, "", 0)
	fsm := NewAttesterFSM(logger, mockSigner, "")

	height := uint64(105)
	hash := testHash(4)
	dataToSign := []byte("data")
	logEntry := createTestLogEntry(t, height, hash[:], dataToSign)

	applyResponse := fsm.Apply(logEntry)

	err, ok := applyResponse.(error)
	assert.True(t, ok, "Apply response should be an error when signer fails")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to sign data", "Error message should mention signing failure")
	// Check the wrapped error maybe?
	// assert.ErrorContains(t, err, "mock signer error")

	// Verify internal state is unchanged
	fsm.mu.RLock()
	defer fsm.mu.RUnlock()
	assert.Empty(t, fsm.processedBlocks, "processedBlocks should be empty on signer error")
	assert.Empty(t, fsm.blockDetails, "blockDetails should be empty on signer error")
}

// TODO: Add tests for Snapshot/Restore (once marshaling is implemented)
