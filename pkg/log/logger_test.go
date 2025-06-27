package log

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLogger(t *testing.T) {
	// Test basic logger creation
	logger := NewLogger(nil)
	assert.NotNil(t, logger)

	// Test logger methods don't panic
	logger.Info("test info", "key", "value")
	logger.Debug("test debug", "key", "value")
	logger.Warn("test warn", "key", "value")
	logger.Error("test error", "key", "value")

	// Test With method
	withLogger := logger.With("module", "test")
	assert.NotNil(t, withLogger)
	withLogger.Info("test with logger")

	// Test implementation access
	impl := logger.Impl()
	assert.NotNil(t, impl)
}

func TestNopLogger(t *testing.T) {
	logger := NewNopLogger()
	assert.NotNil(t, logger)

	// These should not panic
	logger.Info("test info")
	logger.Debug("test debug")
	logger.Warn("test warn")
	logger.Error("test error")
}

func TestTestLogger(t *testing.T) {
	logger := NewTestLogger(t)
	assert.NotNil(t, logger)

	// These should not panic
	logger.Info("test info")
	logger.Debug("test debug")
	logger.Warn("test warn")
	logger.Error("test error")
}