package cmd

import (
	"bufio" // Ensure this import is present
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	ds "github.com/ipfs/go-datastore"
	"github.com/spf13/cobra"

	// Ensure block package is imported if not already
	"github.com/rollkit/rollkit/pkg/store"
)

var targetRollbackHeight uint64

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

// StoreRollbackCmd defines the command for rolling back the store to a specific height.
var StoreRollbackCmd = &cobra.Command{
	Use:   "rollback --height <target_height>",
	Short: "Roll back the block store to a specific height (DANGEROUS: deletes blocks)",
	Long: `Rolls back the block store to the specified target height.
All blocks with height > target_height will be deleted from the store.
This operation is destructive and cannot be undone. Use with extreme caution!
The '--height' flag specifies the last block height to KEEP.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		nodeConfig, err := ParseConfig(cmd)
		if err != nil {
			return fmt.Errorf("error parsing config: %w", err)
		}

		if targetRollbackHeight == 0 {
			cmd.PrintErrln("Warning: Rolling back to height 0. This keeps only the initial state before the first block.")
		}

		dbPath := filepath.Join(nodeConfig.RootDir, nodeConfig.DBPath)
		fmt.Printf("Data directory: %s\n", dbPath)

		kvStore, err := store.NewDefaultKVStore(nodeConfig.RootDir, nodeConfig.DBPath, "rollkit-rollback")
		if err != nil {
			return fmt.Errorf("failed to create KV store: %w", err)
		}
		s := store.New(kvStore)
		defer func() {
			if err := s.Close(); err != nil {
				cmd.PrintErrf("Error closing store: %v\n", err)
			}
		}()

		ctx := cmd.Context()

		currentHeight, err := s.Height(ctx)
		if err != nil {
			if errors.Is(err, ds.ErrNotFound) {
				cmd.Println("Store appears to be empty or height not set. Nothing to roll back.")
				return nil
			}
			return fmt.Errorf("failed to get current store height: %w", err)
		}

		state, err := s.GetState(ctx)
		if err != nil {
			if errors.Is(err, ds.ErrNotFound) {
				return fmt.Errorf("failed to get initial state (not found): cannot perform rollback on uninitialized store")
			}
			return fmt.Errorf("failed to get initial state: %w", err)
		}
		initialHeight := state.InitialHeight

		if targetRollbackHeight >= currentHeight {
			return fmt.Errorf("target height (%d) must be less than current height (%d)", targetRollbackHeight, currentHeight)
		}
		if initialHeight > 0 && targetRollbackHeight < initialHeight-1 {
			return fmt.Errorf("target height (%d) cannot be less than initial height - 1 (%d)", targetRollbackHeight, initialHeight-1)
		}

		fmt.Printf("Current store height: %d\n", currentHeight)
		fmt.Printf("Initial block height: %d\n", initialHeight)
		fmt.Printf("Target rollback height (last height to keep): %d\n", targetRollbackHeight)
		fmt.Printf("This will DELETE blocks from height %d to %d.\n", targetRollbackHeight+1, currentHeight)
		fmt.Println("THIS OPERATION IS IRREVERSIBLE.")

		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Are you sure you want to proceed? (y/N): ")
		confirm, _ := reader.ReadString('\n')
		confirm = strings.TrimSpace(strings.ToLower(confirm))

		if confirm != "y" {
			fmt.Println("Rollback aborted.")
			return nil
		}

		fmt.Println("Starting rollback...")

		err = s.Rollback(ctx, currentHeight, targetRollbackHeight)
		if err != nil {
			return fmt.Errorf("rollback failed: %w", err)
		}

		cmd.Printf("Rollback to height %d completed successfully.\n", targetRollbackHeight)
		return nil
	},
}

func init() {
	StoreRollbackCmd.Flags().Uint64Var(&targetRollbackHeight, "height", 0, "The target height to roll back to (last height to keep)")
}
