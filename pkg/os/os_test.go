//nolint:gosec
package os

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

type mockLogger struct {
	lastMsg string
	lastKV  []any
}

func (m *mockLogger) Info(msg string, keyvals ...any) {
	m.lastMsg = msg
	m.lastKV = keyvals
}

// Note: TrapSignal is difficult to test completely because it calls os.Exit
// In a production environment, you might want to make the exit behavior injectable
// for better testability. This test only verifies the signal handling setup.
func TestTrapSignal(t *testing.T) {
	logger := &mockLogger{}
	cb := func() {}

	done := make(chan bool)
	go func() {
		TrapSignal(logger, cb)
		done <- true
	}()

	// Give some time for the signal handler to be set up
	time.Sleep(100 * time.Millisecond)

	// We can't actually test the full signal handling as it would exit the program
	// Instead, we just verify that the setup completed without errors
	select {
	case <-done:
		t.Log("Signal handler setup completed")
	case <-time.After(time.Second):
		t.Error("Timed out waiting for signal handler setup")
	}
}

func TestEnsureDir(t *testing.T) {
	tempDir := t.TempDir()
	testDir := filepath.Join(tempDir, "test_dir")

	// Test creating a new directory
	err := EnsureDir(testDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	// Check if directory exists
	if !FileExists(testDir) {
		t.Error("Directory was not created")
	}

	// Test creating an existing directory (should not error)
	err = EnsureDir(testDir, 0755)
	if err != nil {
		t.Errorf("Failed to ensure existing directory: %v", err)
	}

	// Test creating a directory with invalid path
	invalidPath := filepath.Join(testDir, string([]byte{0}))
	if err := EnsureDir(invalidPath, 0755); err == nil {
		t.Error("Expected error for invalid path")
	}
}

func TestFileExists(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")

	// Test non-existent file
	if FileExists(testFile) {
		t.Error("FileExists returned true for non-existent file")
	}

	// Create file and test again
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil { //nolint:gosec
		t.Fatalf("Failed to create test file: %v", err)
	}

	if !FileExists(testFile) {
		t.Error("FileExists returned false for existing file")
	}
}

func TestReadWriteFile(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	content := []byte("test content")

	// Test WriteFile
	err := WriteFile(testFile, content, 0644) //nolint:gosec
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Test ReadFile
	readContent, err := ReadFile(testFile) //nolint:gosec
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	if string(readContent) != string(content) {
		t.Errorf("Expected content %q, got %q", string(content), string(readContent))
	}

	// Test reading non-existent file
	_, err = ReadFile(filepath.Join(tempDir, "nonexistent.txt")) //nolint:gosec
	if err == nil {
		t.Error("Expected error when reading non-existent file")
	}
}

func TestCopyFile(t *testing.T) {
	tempDir := t.TempDir()
	srcFile := filepath.Join(tempDir, "src.txt")
	dstFile := filepath.Join(tempDir, "dst.txt")
	content := []byte("test content")

	// Create source file
	if err := os.WriteFile(srcFile, content, 0644); err != nil { //nolint:gosec
		t.Fatalf("Failed to create source file: %v", err)
	}

	// Test copying file
	err := CopyFile(srcFile, dstFile)
	if err != nil {
		t.Fatalf("CopyFile failed: %v", err)
	}

	// Verify content
	dstContent, err := os.ReadFile(dstFile) //nolint:gosec
	if err != nil {
		t.Fatalf("Failed to read destination file: %v", err)
	}

	if string(dstContent) != string(content) {
		t.Errorf("Expected content %q, got %q", string(content), string(dstContent))
	}

	// Test copying from non-existent file
	err = CopyFile(filepath.Join(tempDir, "nonexistent.txt"), dstFile)
	if err == nil {
		t.Error("Expected error when copying non-existent file")
	}

	// Test copying from directory
	err = CopyFile(tempDir, dstFile)
	if err == nil {
		t.Error("Expected error when copying from directory")
	}
}

func TestMustReadFile(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	content := []byte("test content")

	// Create test file
	if err := os.WriteFile(testFile, content, 0644); err != nil { //nolint:gosec
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test successful read
	readContent := MustReadFile(testFile)
	if string(readContent) != string(content) {
		t.Errorf("Expected content %q, got %q", string(content), string(readContent))
	}
}

func TestMustWriteFile(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	content := []byte("test content")

	// Test successful write
	MustWriteFile(testFile, content, 0644)

	// Verify content
	readContent, err := os.ReadFile(testFile) //nolint:gosec
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	if string(readContent) != string(content) {
		t.Errorf("Expected content %q, got %q", string(content), string(readContent))
	}
}
