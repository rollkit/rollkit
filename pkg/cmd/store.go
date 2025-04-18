package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// UnsafeCleanDataDir removes all contents of the specified data directory.
// It does not remove the data directory itself, only its contents.
func UnsafeCleanDataDir(dataDir string) error {
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		if os.IsNotExist(err) {
			// Data directory does not exist, nothing to clean.
			return nil
		}
		return fmt.Errorf("failed to read data directory: %w", err)
	}
	for _, entry := range entries {
		entryPath := filepath.Join(dataDir, entry.Name())
		err := os.RemoveAll(entryPath)
		if err != nil {
			return fmt.Errorf("failed to remove %s: %w", entryPath, err)
		}
	}
	return nil
}

// StoreUnsafeCleanCmd is a Cobra command that removes all contents of the data directory.
var StoreUnsafeCleanCmd = &cobra.Command{
	Use:   "unsafe-clean",
	Short: "Remove all contents of the data directory (DANGEROUS: cannot be undone)",
	Long: `Removes all files and subdirectories in the node's data directory.
This operation is unsafe and cannot be undone. Use with caution!`,
	RunE: func(cmd *cobra.Command, args []string) error {
		nodeConfig, err := ParseConfig(cmd)
		if err != nil {
			return fmt.Errorf("error parsing config: %w", err)
		}
		dataDir := filepath.Join(nodeConfig.RootDir, nodeConfig.DBPath)
		fmt.Println("Data directory:", dataDir)
		if dataDir == "" {
			return fmt.Errorf("data directory not found in node configuration")
		}

		if err := UnsafeCleanDataDir(dataDir); err != nil {
			return err
		}
		cmd.Printf("All contents of the data directory at %s have been removed.\n", dataDir)
		return nil
	},
}
