package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestUnsafeCleanDataDir(t *testing.T) {
	tempDir := t.TempDir()

	// Create some test files and directories
	subDir := filepath.Join(tempDir, "subdir")
	require.NoError(t, os.Mkdir(subDir, 0755))
	testFile := filepath.Join(tempDir, "testfile.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("test content"), 0o600))

	// Ensure the files and directories exist
	require.DirExists(t, subDir)
	require.FileExists(t, testFile)

	// Call the function to clean the directory
	err := UnsafeCleanDataDir(tempDir)
	require.NoError(t, err)

	// Ensure the directory is empty
	entries, err := os.ReadDir(tempDir)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestStoreUnsafeCleanCmd(t *testing.T) {
	tempDir := t.TempDir()
	dataDir := filepath.Join(tempDir, "data")
	configDir := filepath.Join(tempDir, "config")
	configFile := filepath.Join(configDir, "rollkit.yml")

	// Create necessary directories
	require.NoError(t, os.Mkdir(configDir, 0755))
	require.NoError(t, os.Mkdir(dataDir, 0755))

	// Create a dummy config file
	dummyConfig := `
	RootDir = "` + tempDir + `"
	DBPath = "data"
	`
	require.NoError(t, os.WriteFile(configFile, []byte(dummyConfig), 0o600))

	// Create some test files and directories inside dataDir
	testFile := filepath.Join(dataDir, "testfile.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("test content"), 0o600))

	// Ensure the files and directories exist
	require.FileExists(t, testFile)

	// Create a root command and add the subcommand
	rootCmd := &cobra.Command{Use: "root"}
	rootCmd.PersistentFlags().String("home", tempDir, "root directory")
	rootCmd.AddCommand(StoreUnsafeCleanCmd)

	// Capture output
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)

	// Execute the command
	rootCmd.SetArgs([]string{"unsafe-clean"})
	err := rootCmd.Execute()
	require.NoError(t, err)

	// Ensure the data directory is empty
	entries, err := os.ReadDir(dataDir)
	require.NoError(t, err)
	require.Empty(t, entries, "Data directory should be empty after clean")

	// Ensure the data directory itself still exists
	_, err = os.Stat(dataDir)
	require.NoError(t, err, "Data directory itself should still exist")

	// Check output message (optional)
	require.Contains(t, buf.String(), fmt.Sprintf("All contents of the data directory at %s have been removed.", dataDir))
}
