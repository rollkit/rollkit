package test

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/cometbft/cometbft/libs/log"
)

// TODO(tzdybal): move to some common place

// FileLogger is a simple, thread-safe logger that logs to a tempFile and is
// intended for use in testing.
type FileLogger struct {
	kv      []interface{}
	logFile *os.File
	T       *testing.T
	mtx     *sync.Mutex
}

// TempLogFileName returns a filename for a log file that is unique to the test.
func TempLogFileName(t *testing.T, suffix string) string {
	return strings.ReplaceAll(t.Name(), "/", "_") + suffix + ".log"
}

// NewLogger create a Logger using the name of the test as the filename.
func NewFileLogger(t *testing.T) *FileLogger {
	// Use the test name but strip out any slashes since they are not
	// allowed in filenames.
	return NewFileLoggerCustom(t, TempLogFileName(t, ""))
}

// NewLoggerCustom create a Logger using the given filename.
func NewFileLoggerCustom(t *testing.T, fileName string) *FileLogger {
	logFile, err := os.CreateTemp("", fileName)
	if err != nil {
		panic(err)
	}
	t.Log("Test logs can be found:", logFile.Name())
	t.Cleanup(func() {
		_ = logFile.Close()
	})
	return &FileLogger{
		kv:      []interface{}{},
		logFile: logFile,
		T:       t,
		mtx:     new(sync.Mutex),
	}
}

// Debug prints a debug message.
func (f *FileLogger) Debug(msg string, keyvals ...interface{}) {
	f.T.Helper()
	f.mtx.Lock()
	defer f.mtx.Unlock()
	kvs := append(f.kv, keyvals...)
	_, err := fmt.Fprintln(f.logFile, append([]interface{}{"DEBUG: " + msg}, kvs...)...)
	if err != nil {
		panic(err)
	}
}

// Info prints an info message.
func (f *FileLogger) Info(msg string, keyvals ...interface{}) {
	f.T.Helper()
	f.mtx.Lock()
	defer f.mtx.Unlock()
	kvs := append(f.kv, keyvals...)
	_, err := fmt.Fprintln(f.logFile, append([]interface{}{"INFO: " + msg}, kvs...)...)
	if err != nil {
		panic(err)
	}
}

// Error prints an error message.
func (f *FileLogger) Error(msg string, keyvals ...interface{}) {
	f.T.Helper()
	f.mtx.Lock()
	defer f.mtx.Unlock()
	kvs := append(f.kv, keyvals...)
	_, err := fmt.Fprintln(f.logFile, append([]interface{}{"ERROR: " + msg}, kvs...)...)
	if err != nil {
		panic(err)
	}
}

// With returns a new Logger with the given keyvals added.
func (f *FileLogger) With(keyvals ...interface{}) log.Logger {
	f.mtx.Lock()
	defer f.mtx.Unlock()
	return &FileLogger{
		kv:      append(f.kv, keyvals...),
		logFile: f.logFile,
		T:       f.T,
		mtx:     new(sync.Mutex),
	}
}

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
