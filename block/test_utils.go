package block

import (
	"crypto/sha256"
	"encoding/binary"
	"testing"

	"cosmossdk.io/log"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// GenerateHeaderHash creates a deterministic hash for a test header based on height and proposer.
// This is useful for predicting expected hashes in tests without needing full header construction.
func GenerateHeaderHash(t *testing.T, height uint64, proposer []byte) []byte {
	t.Helper()
	// Create a simple deterministic representation of the header's identity
	heightBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(heightBytes, height)

	hasher := sha256.New()
	_, err := hasher.Write([]byte("testheader:")) // Prefix to avoid collisions
	require.NoError(t, err)
	_, err = hasher.Write(heightBytes)
	require.NoError(t, err)
	_, err = hasher.Write(proposer)
	require.NoError(t, err)

	return hasher.Sum(nil)
}

type MockLogger struct {
	mock.Mock
}

func (m *MockLogger) Debug(msg string, keyvals ...any) { m.Called(msg, keyvals) }
func (m *MockLogger) Info(msg string, keyvals ...any)  { m.Called(msg, keyvals) }
func (m *MockLogger) Warn(msg string, keyvals ...any)  { m.Called(msg, keyvals) }
func (m *MockLogger) Error(msg string, keyvals ...any) { m.Called(msg, keyvals) }
func (m *MockLogger) With(keyvals ...any) log.Logger   { return m }
func (m *MockLogger) Impl() any                        { return m }
