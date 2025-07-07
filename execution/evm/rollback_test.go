package evm

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEngineClient_Rollback(t *testing.T) {
	tests := []struct {
		name             string
		currentHeight    uint64
		expectError      bool
		expectedErrorMsg string
	}{
		{
			name:             "cannot rollback from height 1",
			currentHeight:    1,
			expectError:      true,
			expectedErrorMsg: "cannot rollback from height 1: must be > 1",
		},
		{
			name:             "cannot rollback from height 0",
			currentHeight:    0,
			expectError:      true,
			expectedErrorMsg: "cannot rollback from height 0: must be > 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock EVM client
			client := &EngineClient{
				genesisHash:               common.HexToHash("0x1234"),
				feeRecipient:              common.HexToAddress("0x5678"),
				currentHeadBlockHash:      common.HexToHash("0xabcd"),
				currentSafeBlockHash:      common.HexToHash("0xabcd"),
				currentFinalizedBlockHash: common.HexToHash("0x1234"),
			}

			// Execute rollback
			prevStateRoot, err := client.Rollback(context.Background(), tt.currentHeight)

			// Verify results
			if tt.expectError {
				require.Error(t, err)
				if tt.expectedErrorMsg != "" {
					assert.Contains(t, err.Error(), tt.expectedErrorMsg)
				}
				assert.Nil(t, prevStateRoot)
			} else {
				// This case won't be tested without real EVM connection
				t.Skip("Requires real EVM connection for success case")
			}
		})
	}
}

func TestEngineClient_RollbackValidation(t *testing.T) {
	client := &EngineClient{}

	// Test validation without network calls
	_, err := client.Rollback(context.Background(), 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot rollback from height 1: must be > 1")

	_, err = client.Rollback(context.Background(), 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot rollback from height 0: must be > 1")
}