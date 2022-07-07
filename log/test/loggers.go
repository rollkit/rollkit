package test

import (
	"fmt"
	"sync"
	"testing"
)

// TODO(tzdybal): move to some common place

// Logger is a simple, yet thread-safe, logger intended for use in unit tests.
type Logger struct {
	mtx *sync.Mutex
	T   *testing.T
}

// NewLogger create a Logger that outputs data using given testing.T instance.
func NewLogger(t *testing.T) *Logger {
	return &Logger{
		mtx: new(sync.Mutex),
		T:   t,
	}
}

// Debug prints a debug message.
func (t *Logger) Debug(msg string, keyvals ...interface{}) {
	t.T.Helper()
	t.mtx.Lock()
	defer t.mtx.Unlock()
	t.T.Log(append([]interface{}{"DEBUG: " + msg}, keyvals...)...)
}

// Info prints an info message.
func (t *Logger) Info(msg string, keyvals ...interface{}) {
	t.T.Helper()
	t.mtx.Lock()
	defer t.mtx.Unlock()
	t.T.Log(append([]interface{}{"INFO:  " + msg}, keyvals...)...)
}

// Error prints an error message.
func (t *Logger) Error(msg string, keyvals ...interface{}) {
	t.T.Helper()
	t.mtx.Lock()
	defer t.mtx.Unlock()
	t.T.Log(append([]interface{}{"ERROR: " + msg}, keyvals...)...)
}

// MockLogger is a fake logger that accumulates all the inputs.
//
// It can be used in tests to ensure that certain messages was logged with correct severity.
type MockLogger struct {
	DebugLines, InfoLines, ErrLines []string
}

// Debug saves a debug message.
func (t *MockLogger) Debug(msg string, keyvals ...interface{}) {
	t.DebugLines = append(t.DebugLines, fmt.Sprint(append([]interface{}{msg}, keyvals...)...))
}

// Info saves an info message.
func (t *MockLogger) Info(msg string, keyvals ...interface{}) {
	t.InfoLines = append(t.InfoLines, fmt.Sprint(append([]interface{}{msg}, keyvals...)...))
}

// Error saves an error message.
func (t *MockLogger) Error(msg string, keyvals ...interface{}) {
	t.ErrLines = append(t.ErrLines, fmt.Sprint(append([]interface{}{msg}, keyvals...)...))
}
