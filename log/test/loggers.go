package test

import (
	"fmt"
	"sync"
	"testing"
)

// TODO(tzdybal): move to some common place
type TestLogger struct {
	mtx sync.Mutex
	T   *testing.T
}

func (t *TestLogger) Debug(msg string, keyvals ...interface{}) {
	t.T.Helper()
	t.mtx.Lock()
	defer t.mtx.Unlock()
	t.T.Log(append([]interface{}{"DEBUG: " + msg}, keyvals...)...)
}

func (t *TestLogger) Info(msg string, keyvals ...interface{}) {
	t.T.Helper()
	t.mtx.Lock()
	defer t.mtx.Unlock()
	t.T.Log(append([]interface{}{"INFO:  " + msg}, keyvals...)...)
}

func (t *TestLogger) Error(msg string, keyvals ...interface{}) {
	t.T.Helper()
	t.mtx.Lock()
	defer t.mtx.Unlock()
	t.T.Log(append([]interface{}{"ERROR: " + msg}, keyvals...)...)
}

type MockLogger struct {
	DebugLines, InfoLines, ErrLines []string
}

func (t *MockLogger) Debug(msg string, keyvals ...interface{}) {
	t.DebugLines = append(t.DebugLines, fmt.Sprint(append([]interface{}{msg}, keyvals...)...))
}

func (t *MockLogger) Info(msg string, keyvals ...interface{}) {
	t.InfoLines = append(t.InfoLines, fmt.Sprint(append([]interface{}{msg}, keyvals...)...))
}

func (t *MockLogger) Error(msg string, keyvals ...interface{}) {
	t.ErrLines = append(t.ErrLines, fmt.Sprint(append([]interface{}{msg}, keyvals...)...))
}
