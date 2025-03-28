package block

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"cosmossdk.io/log"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	coreexecutor "github.com/rollkit/rollkit/core/execution"
	"github.com/rollkit/rollkit/pkg/cache"
	"github.com/rollkit/rollkit/pkg/genesis"
	"github.com/rollkit/rollkit/pkg/store"
	"github.com/rollkit/rollkit/types"
)

// MockStore extends the real store to allow us to control its behavior
type MockStoreForSync struct {
	mock.Mock
	store.Store
	height uint64
}

func (m *MockStoreForSync) Height() uint64 {
	return m.height
}

func (m *MockStoreForSync) SetHeight(ctx context.Context, height uint64) {
	m.height = height
	m.Called(ctx, height)
}

func (m *MockStoreForSync) SaveBlockData(ctx context.Context, header *types.SignedHeader, data *types.Data, sig *types.Signature) error {
	args := m.Called(ctx, header, data, sig)
	return args.Error(0)
}

func (m *MockStoreForSync) UpdateState(ctx context.Context, state types.State) error {
	args := m.Called(ctx, state)
	return args.Error(0)
}

func TestTrySyncNextBlock(t *testing.T) {
	// Create mock store
	mockStore := new(MockStoreForSync)
	mockStore.height = 10 // Start at height 10

	// Create dummy executor
	dummyExec := coreexecutor.NewDummyExecutor()

	// Create header and data caches
	headerCache := cache.NewCache[types.SignedHeader]()
	dataCache := cache.NewCache[types.Data]()

	// Create test logger
	logger := log.NewTestLogger(t)

	// Create metrics
	metrics := NopMetrics()

	// Create genesis data
	genesisData := genesis.Genesis{
		ChainID: "test-chain",
		ExtraData: genesis.GenesisExtraData{
			ProposerAddress: []byte("test-proposer-address"),
		},
	}

	// Setup initial state
	initialState := types.State{
		ChainID:         "test-chain",
		Version:         types.Version{Block: 1, App: 2},
		LastBlockHeight: 10,
		AppHash:         []byte("previous-app-hash"),
	}

	// Create Manager with the required components
	m := &Manager{
		store:        mockStore,
		exec:         dummyExec,
		headerCache:  headerCache,
		dataCache:    dataCache,
		logger:       logger,
		lastState:    initialState,
		lastStateMtx: new(sync.RWMutex),
		metrics:      metrics,
		genesis:      genesisData,
	}

	// Setup test context
	ctx := context.Background()

	// Create a block at height 11 (next height)
	blockTime := time.Now().UTC()
	header11 := &types.SignedHeader{
		Header: types.Header{
			Version: types.Version{
				Block: 1,
				App:   2,
			},
			BaseHeader: types.BaseHeader{
				ChainID: "test-chain",
				Height:  11,
				Time:    uint64(blockTime.UnixNano()),
			},
			LastHeaderHash:  types.Hash([]byte("last-header-hash")),
			AppHash:         []byte("app-hash"),
			ProposerAddress: []byte("proposer-address"),
		},
		// Signature would be validated in execValidate which we're mocking
		Signature: types.Signature([]byte("test-signature")),
	}

	// Create block data
	data11 := &types.Data{
		Txs: types.Txs{types.Tx([]byte("tx1"))},
		Metadata: &types.Metadata{
			ChainID: "test-chain",
			Height:  11,
			Time:    uint64(blockTime.UnixNano()),
		},
	}

	// Add the header and data to caches
	headerCache.SetItem(11, header11)
	dataCache.SetItem(11, data11)

	// Set up mock expectations
	mockStore.On("SaveBlockData", ctx, header11, data11, &header11.Signature).Return(nil)
	mockStore.On("SetHeight", ctx, uint64(11)).Return()
	mockStore.On("UpdateState", ctx, mock.Anything).Return(nil)

	// Test 1: Successfully sync a block
	err := m.trySyncNextBlock(ctx, 5) // DA height doesn't matter much here
	require.NoError(t, err)

	// Verify that the store methods were called
	mockStore.AssertExpectations(t)

	// Verify that the header and data were removed from cache
	require.Nil(t, headerCache.GetItem(11))
	require.Nil(t, dataCache.GetItem(11))

	// Test 2: No header in cache
	err = m.trySyncNextBlock(ctx, 5)
	require.NoError(t, err) // Should return nil, not error

	// Create a new header and data but don't add header to cache
	header12 := &types.SignedHeader{
		Header: types.Header{
			BaseHeader: types.BaseHeader{
				Height: 12,
			},
		},
	}
	data12 := &types.Data{}
	dataCache.SetItem(12, data12)

	// Test 3: Header missing, should return without error
	err = m.trySyncNextBlock(ctx, 5)
	require.NoError(t, err)

	// Test 4: Data missing, should return without error
	headerCache.SetItem(12, header12)
	dataCache.DeleteItem(12) // Remove data

	err = m.trySyncNextBlock(ctx, 5)
	require.NoError(t, err)
}

func TestTrySyncNextBlockWithSaveError(t *testing.T) {
	// Create mock store
	mockStore := new(MockStoreForSync)
	mockStore.height = 10

	// Create dummy executor
	dummyExec := coreexecutor.NewDummyExecutor()

	// Create header and data caches
	headerCache := cache.NewCache[types.SignedHeader]()
	dataCache := cache.NewCache[types.Data]()

	// Create test logger
	logger := log.NewTestLogger(t)

	// Create metrics
	metrics := NopMetrics()

	// Create genesis data
	genesisData := genesis.Genesis{
		ChainID: "test-chain",
		ExtraData: genesis.GenesisExtraData{
			ProposerAddress: []byte("test-proposer-address"),
		},
	}

	// Setup initial state
	initialState := types.State{
		ChainID:         "test-chain",
		Version:         types.Version{Block: 1, App: 2},
		LastBlockHeight: 10,
		AppHash:         []byte("previous-app-hash"),
	}

	// Create Manager with the required components
	m := &Manager{
		store:        mockStore,
		exec:         dummyExec,
		headerCache:  headerCache,
		dataCache:    dataCache,
		logger:       logger,
		lastState:    initialState,
		lastStateMtx: new(sync.RWMutex),
		metrics:      metrics,
		genesis:      genesisData,
	}

	// Setup test context
	ctx := context.Background()

	// Create a block at height 11 (next height)
	header11 := &types.SignedHeader{
		Header: types.Header{
			Version: types.Version{
				Block: 1,
				App:   2,
			},
			BaseHeader: types.BaseHeader{
				ChainID: "test-chain",
				Height:  11,
				Time:    uint64(time.Now().UTC().UnixNano()),
			},
			AppHash: []byte("app-hash"),
		},
		Signature: types.Signature([]byte("test-signature")),
	}

	// Create block data
	data11 := &types.Data{
		Txs: types.Txs{types.Tx([]byte("tx1"))},
		Metadata: &types.Metadata{
			ChainID: "test-chain",
			Height:  11,
		},
	}

	// Add the header and data to caches
	headerCache.SetItem(11, header11)
	dataCache.SetItem(11, data11)

	// Create custom error
	testError := errors.New("test save error")

	// Set up mock to simulate an error when saving block data
	mockStore.On("SaveBlockData", ctx, header11, data11, &header11.Signature).Return(testError)

	// Test: Save error should be returned as SaveBlockError
	err := m.trySyncNextBlock(ctx, 5)
	require.Error(t, err)
	require.IsType(t, SaveBlockError{}, err)

	// Verify that items remain in cache since sync failed
	require.NotNil(t, headerCache.GetItem(11))
	require.NotNil(t, dataCache.GetItem(11))
}
