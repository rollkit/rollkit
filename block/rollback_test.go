package block

import (
	"context"
	"sync"
	"testing"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/pkg/config"
	"github.com/rollkit/rollkit/pkg/genesis"
	"github.com/rollkit/rollkit/test/mocks"
	"github.com/rollkit/rollkit/types"
)

func TestManager_RollbackLastBlock(t *testing.T) {
	tests := []struct {
		name               string
		currentHeight      uint64
		expectError        bool
		expectedErrorMsg   string
		setupMocks         func(*mocks.MockStore, *mocks.MockExecutor)
	}{
		{
			name:             "cannot rollback genesis block",
			currentHeight:    1,
			expectError:      true,
			expectedErrorMsg: "cannot rollback from height 1: must be > 1",
			setupMocks: func(mockStore *mocks.MockStore, mockExec *mocks.MockExecutor) {
				// No mocks needed as error should be returned early
			},
		},
		{
			name:          "successful rollback from height 2",
			currentHeight: 2,
			expectError:   false,
			setupMocks: func(mockStore *mocks.MockStore, mockExec *mocks.MockExecutor) {
				prevStateRoot := []byte{1, 2, 3, 4}
				prevState := types.State{
					ChainID:         "test-chain",
					LastBlockHeight: 1,
					LastBlockTime:   time.Now().Add(-time.Minute),
					AppHash:         prevStateRoot,
				}

				// Mock executor rollback
				mockExec.On("Rollback", mock.Anything, uint64(2)).Return(prevStateRoot, nil)

				// Mock store rollback
				mockStore.On("RollbackToHeight", mock.Anything, uint64(1)).Return(nil)

				// Mock getting state after rollback
				mockStore.On("GetState", mock.Anything).Return(prevState, nil)

				// Mock getting header for cache cleanup
				mockStore.On("GetHeader", mock.Anything, uint64(2)).Return(nil, assert.AnError)
			},
		},
		{
			name:             "executor rollback fails",
			currentHeight:    3,
			expectError:      true,
			expectedErrorMsg: "failed to rollback execution layer",
			setupMocks: func(mockStore *mocks.MockStore, mockExec *mocks.MockExecutor) {
				// Mock executor rollback failure
				mockExec.On("Rollback", mock.Anything, uint64(3)).Return(nil, assert.AnError)
			},
		},
		{
			name:             "store rollback fails",
			currentHeight:    2,
			expectError:      true,
			expectedErrorMsg: "failed to rollback store",
			setupMocks: func(mockStore *mocks.MockStore, mockExec *mocks.MockExecutor) {
				prevStateRoot := []byte{1, 2, 3, 4}

				// Mock executor rollback success
				mockExec.On("Rollback", mock.Anything, uint64(2)).Return(prevStateRoot, nil)

				// Mock store rollback failure
				mockStore.On("RollbackToHeight", mock.Anything, uint64(1)).Return(assert.AnError)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks
			mockStore := mocks.NewMockStore(t)
			mockExec := mocks.NewMockExecutor(t)

			// Setup the specific mocks for this test
			tt.setupMocks(mockStore, mockExec)

			// Create manager with mocks
			manager := &Manager{
				lastState: types.State{
					ChainID:         "test-chain",
					LastBlockHeight: tt.currentHeight,
					LastBlockTime:   time.Now(),
					AppHash:         []byte{5, 6, 7, 8},
				},
				lastStateMtx: &sync.RWMutex{},
				store:        mockStore,
				exec:         mockExec,
				config:       config.Config{},
				genesis:      genesis.Genesis{},
				logger:       logging.Logger("test"),
			}

			// Set DA included height to current height for testing
			manager.daIncludedHeight.Store(tt.currentHeight)

			// Execute rollback
			err := manager.RollbackLastBlock(context.Background())

			// Verify results
			if tt.expectError {
				require.Error(t, err)
				if tt.expectedErrorMsg != "" {
					assert.Contains(t, err.Error(), tt.expectedErrorMsg)
				}
			} else {
				require.NoError(t, err)
				
				// Verify state was updated
				assert.Equal(t, uint64(1), manager.lastState.LastBlockHeight)
				
				// Verify DA included height was updated
				assert.Equal(t, uint64(1), manager.daIncludedHeight.Load())
			}
		})
	}
}