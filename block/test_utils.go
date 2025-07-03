package block

import (
	"crypto/sha256"
	"encoding/binary"
	"testing"

	logging "github.com/ipfs/go-log/v2"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap" // Needed for potential With behavior
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

// Ensure MockLogger implements the ipfs/go-log/v2 EventLogger (StandardLogger) interface
var _ logging.StandardLogger = &MockLogger{}

// For non-f methods, the first arg is typically the message string, others are keyvals.
func (m *MockLogger) Debug(args ...interface{})                 { m.Called(args...) }
func (m *MockLogger) Debugf(format string, args ...interface{}) { m.Called(format, args) } // Note: testify mock doesn't directly support (format, ...any) well for matching, usually match format string exactly.
func (m *MockLogger) Error(args ...interface{})                 { m.Called(args...) }
func (m *MockLogger) Errorf(format string, args ...interface{}) { m.Called(format, args) }
func (m *MockLogger) Fatal(args ...interface{})                 { m.Called(args...); panic("fatal error logged") }
func (m *MockLogger) Fatalf(format string, args ...interface{}) {
	m.Called(format, args)
	panic("fatal error logged")
}
func (m *MockLogger) Info(args ...interface{})                 { m.Called(args...) }
func (m *MockLogger) Infof(format string, args ...interface{}) { m.Called(format, args) }
func (m *MockLogger) Panic(args ...interface{})                { m.Called(args...); panic("panic error logged") }
func (m *MockLogger) Panicf(format string, args ...interface{}) {
	m.Called(format, args)
	panic("panic error logged")
}
func (m *MockLogger) Warn(args ...interface{})                 { m.Called(args...) }
func (m *MockLogger) Warnf(format string, args ...interface{}) { m.Called(format, args) }

func (m *MockLogger) With(keyvals ...interface{}) *zap.SugaredLogger {
	args := m.Called(append([]interface{}{"With"}, keyvals...)...)
	if logger, ok := args.Get(0).(*zap.SugaredLogger); ok {
		return logger
	}
	return nil
}
