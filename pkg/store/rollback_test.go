package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/types"
)

func TestDefaultStore_RollbackToHeight(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name             string
		setupBlocks      int
		targetHeight     uint64
		expectError      bool
		expectedErrorMsg string
	}{
		{
			name:             "cannot rollback to same height",
			setupBlocks:      2,
			targetHeight:     2,
			expectError:      true,
			expectedErrorMsg: "cannot rollback to height 2: current height is 2",
		},
		{
			name:             "cannot rollback to height 0",
			setupBlocks:      2,
			targetHeight:     0,
			expectError:      true,
			expectedErrorMsg: "cannot rollback to height 0: must be >= 1",
		},
		{
			name:         "successful rollback from height 3 to 1",
			setupBlocks:  3,
			targetHeight: 1,
			expectError:  false,
		},
		{
			name:         "successful rollback from height 2 to 1",
			setupBlocks:  2,
			targetHeight: 1,
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create new in-memory store for each test
			kvStore, err := NewDefaultInMemoryKVStore()
			require.NoError(t, err)
			s := New(kvStore)

			// Setup initial blocks
			for i := 1; i <= tt.setupBlocks; i++ {
				header, data := types.GetRandomBlock(uint64(i), 1, "test-chain") //nolint:gosec // G115: i is positive and within bounds
				signature := createTestSignature()

				err := s.SaveBlockData(ctx, header, data, signature)
				require.NoError(t, err)

				err = s.SetHeight(ctx, uint64(i)) //nolint:gosec // G115: i is positive and within bounds
				require.NoError(t, err)

				// Save state for this height
				state := types.State{
					ChainID:         "test-chain",
					LastBlockHeight: uint64(i), //nolint:gosec // G115: i is positive and within bounds
					LastBlockTime:   time.Now(),
					AppHash:         []byte{byte(i), byte(i), byte(i), byte(i)},
				}
				err = s.UpdateState(ctx, state)
				require.NoError(t, err)
			}

			// Verify initial setup
			currentHeight, err := s.Height(ctx)
			require.NoError(t, err)
			assert.Equal(t, uint64(tt.setupBlocks), currentHeight) //nolint:gosec // G115: setupBlocks is positive and within bounds

			// Execute rollback
			err = s.RollbackToHeight(ctx, tt.targetHeight)

			// Verify results
			if tt.expectError {
				require.Error(t, err)
				if tt.expectedErrorMsg != "" {
					assert.Contains(t, err.Error(), tt.expectedErrorMsg)
				}
			} else {
				require.NoError(t, err)

				// Verify height was updated
				newHeight, err := s.Height(ctx)
				require.NoError(t, err)
				assert.Equal(t, tt.targetHeight, newHeight)

				// Verify blocks at target height and below still exist
				for i := uint64(1); i <= tt.targetHeight; i++ {
					_, _, err := s.GetBlockData(ctx, i)
					assert.NoError(t, err, "Block at height %d should still exist", i)
				}

				// Verify blocks above target height are removed
				for i := tt.targetHeight + 1; i <= uint64(tt.setupBlocks); i++ { //nolint:gosec // G115: setupBlocks is positive and within bounds
					_, _, err := s.GetBlockData(ctx, i)
					assert.Error(t, err, "Block at height %d should be removed", i)
				}

				// Verify state was restored to target height
				state, err := s.GetState(ctx)
				require.NoError(t, err)
				assert.Equal(t, tt.targetHeight, state.LastBlockHeight)
			}
		})
	}
}

func createTestSignature() *types.Signature {
	return &types.Signature{}
}
