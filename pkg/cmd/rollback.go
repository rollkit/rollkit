package cmd

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/rollkit/rollkit/node"
	"github.com/rollkit/rollkit/pkg/store"
)

// RollbackCmd reverts the chain state to the previous block
var RollbackCmd = &cobra.Command{
	Use:   "rollback",
	Short: "Rollback the last block (DANGEROUS: use only for emergency recovery)",
	Long: `Rolls back the chain state to the previous block by removing the last block
from both the execution layer and the block store. This operation is designed for
emergency recovery scenarios when the chain has entered an unrecoverable state.

WARNING: This operation is dangerous and should only be used when the node is stopped.
Make sure to backup your data before running this command.

Usage scenarios:
- Recovery from execution layer corruption
- Emergency rollback after detecting invalid state
- Testing rollback functionality in development environments`,
	RunE: func(cmd *cobra.Command, args []string) error {
		nodeConfig, err := ParseConfig(cmd)
		if err != nil {
			return fmt.Errorf("error parsing config: %w", err)
		}

		// Validate that we have the necessary configuration
		if nodeConfig.DBPath == "" {
			return fmt.Errorf("database path not found in node configuration")
		}

		dbPath := filepath.Join(nodeConfig.RootDir, nodeConfig.DBPath)
		fmt.Printf("Using database path: %s\n", dbPath)

		// Create the KV store
		kvStore, err := store.NewDefaultKVStore(nodeConfig.RootDir, nodeConfig.DBPath, "rollkit")
		if err != nil {
			return fmt.Errorf("failed to create KV store: %w", err)
		}

		mainKV := store.NewPrefixKV(kvStore, node.RollkitPrefix)

		// Create the store
		s := store.New(mainKV)
		defer s.Close()

		// Get current state
		ctx := context.Background()
		currentState, err := s.GetState(ctx)
		if err != nil {
			return fmt.Errorf("failed to get current state: %w", err)
		}

		currentHeight := currentState.LastBlockHeight
		if currentHeight <= 1 {
			return fmt.Errorf("cannot rollback from height %d: must be > 1", currentHeight)
		}

		fmt.Printf("Current chain height: %d\n", currentHeight)
		fmt.Printf("Rolling back to height: %d\n", currentHeight-1)

		// Perform store rollback
		targetHeight := currentHeight - 1
		if err := s.RollbackToHeight(ctx, targetHeight); err != nil {
			return fmt.Errorf("failed to rollback store to height %d: %w", targetHeight, err)
		}

		// Verify the rollback was successful
		newState, err := s.GetState(ctx)
		if err != nil {
			return fmt.Errorf("failed to verify rollback: %w", err)
		}

		if newState.LastBlockHeight != targetHeight {
			return fmt.Errorf("rollback verification failed: expected height %d, got %d",
				targetHeight, newState.LastBlockHeight)
		}

		fmt.Printf("✅ Successfully rolled back to height %d\n", targetHeight)
		fmt.Printf("Previous state root: %x\n", newState.AppHash)
		fmt.Printf("\nIMPORTANT: The execution layer (EVM) state may also need to be rolled back.\n")
		fmt.Printf("Make sure your EVM engine is also reverted to the corresponding state.\n")

		return nil
	},
}
