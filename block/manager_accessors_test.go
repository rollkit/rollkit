package block

import (
	"context"
	"sync"
	"testing"

	coreexecutor "github.com/rollkit/rollkit/core/execution"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	"github.com/rollkit/rollkit/pkg/cache"
	"github.com/rollkit/rollkit/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// TestManagerAccessors tests the simple accessor methods of the Manager type
func TestManagerAccessors(t *testing.T) {
	// Create a test manager with minimal components needed for the tests
	pendingHeaders := &PendingHeaders{}
	sequencer := coresequencer.NewDummySequencer()
	executor := coreexecutor.NewDummyExecutor()
	headerCache := cache.NewCache[types.SignedHeader]()

	// We won't use an actual DA Client since it's complex to mock
	// Instead, we'll just test the nil check in DALCInitialized
	m := &Manager{
		pendingHeaders: pendingHeaders,
		sequencer:      sequencer,
		exec:           executor,
		dalc:           nil, // Start with nil to test DALCInitialized false case
		headerCache:    headerCache,
		lastStateMtx:   new(sync.RWMutex),
		isProposer:     true,
	}

	// Test DALCInitialized when dalc is nil
	require.False(t, m.DALCInitialized())

	// Test PendingHeaders
	require.Equal(t, pendingHeaders, m.PendingHeaders())

	// Test IsProposer
	require.True(t, m.IsProposer())

	// Test SeqClient
	require.Equal(t, sequencer, m.SeqClient())

	// Test GetLastState
	testState := types.State{
		ChainID:         "test-chain",
		LastBlockHeight: 10,
	}
	m.SetLastState(testState)
	require.Equal(t, testState, m.GetLastState())

	// Test GetExecutor
	require.Equal(t, executor, m.GetExecutor())

	// Test IsDAIncluded
	testHash := types.Hash([]byte("test-hash"))
	require.False(t, m.IsDAIncluded(testHash))

	m.headerCache.SetDAIncluded(testHash.String())
	require.True(t, m.IsDAIncluded(testHash))
}

// TestStoreAccessors tests the store-related accessor methods of the Manager
func TestStoreAccessors(t *testing.T) {
	// Create a mock store
	mockStore := new(MockStore)

	// Setup expectations
	mockStore.On("Height").Return(uint64(42))

	// Create a test manager with the mock store
	m := &Manager{
		store: mockStore,
	}

	// Test GetStoreHeight
	require.Equal(t, uint64(42), m.GetStoreHeight())

	// Verify expectations were met
	mockStore.AssertExpectations(t)
}

// TestChannelAccessors tests the channel accessor methods of the Manager
func TestChannelAccessors(t *testing.T) {
	// Create channels for testing
	headerInCh := make(chan NewHeaderEvent, 10)
	dataInCh := make(chan NewDataEvent, 10)

	// Create a test manager with the channels
	m := &Manager{
		headerInCh: headerInCh,
		dataInCh:   dataInCh,
	}

	// Test GetHeaderInCh
	require.Equal(t, headerInCh, m.GetHeaderInCh())

	// Test GetDataInCh
	require.Equal(t, dataInCh, m.GetDataInCh())
}

// TestBlockHashSeen tests the IsBlockHashSeen method
func TestBlockHashSeen(t *testing.T) {
	// Create a test manager with a header cache
	headerCache := cache.NewCache[types.SignedHeader]()

	m := &Manager{
		headerCache: headerCache,
	}

	// Create a test header and add it to the cache
	testHeader := &types.SignedHeader{
		Header: types.Header{
			BaseHeader: types.BaseHeader{
				Height: 1,
			},
		},
	}

	// Set the hash as seen
	hash := testHeader.Header.Hash()
	headerCache.SetSeen(hash.String())

	// Test with an existing hash
	require.True(t, m.IsBlockHashSeen(hash.String()))

	// Test with a non-existing hash
	nonExistingHash := "non-existing-hash"
	require.False(t, m.IsBlockHashSeen(nonExistingHash))
}

// MockStore is a mock implementation of the store.Store interface
type MockStore struct {
	mock.Mock
}

func (m *MockStore) Height() uint64 {
	args := m.Called()
	return args.Get(0).(uint64)
}

// Implement other required methods of the store.Store interface
// with empty implementations for the mock
func (m *MockStore) Close() error                                                    { return nil }
func (m *MockStore) SetHeight(ctx context.Context, height uint64)                    {}
func (m *MockStore) SaveBlock(ctx context.Context, header *types.SignedHeader) error { return nil }
func (m *MockStore) SaveBlockData(ctx context.Context, header *types.SignedHeader, data *types.Data, sig *types.Signature) error {
	return nil
}
func (m *MockStore) GetBlock(ctx context.Context, height uint64) (*types.SignedHeader, error) {
	return nil, nil
}
func (m *MockStore) GetBlockData(ctx context.Context, height uint64) (*types.SignedHeader, *types.Data, error) {
	return nil, nil, nil
}
func (m *MockStore) GetBlockByHash(ctx context.Context, hash []byte) (*types.SignedHeader, *types.Data, error) {
	return nil, nil, nil
}
func (m *MockStore) GetSignature(ctx context.Context, height uint64) (*types.Signature, error) {
	return nil, nil
}
func (m *MockStore) GetSignatureByHash(ctx context.Context, hash []byte) (*types.Signature, error) {
	return nil, nil
}
func (m *MockStore) UpdateState(ctx context.Context, state types.State) error        { return nil }
func (m *MockStore) GetState(ctx context.Context) (types.State, error)               { return types.State{}, nil }
func (m *MockStore) SetMetadata(ctx context.Context, key string, value []byte) error { return nil }
func (m *MockStore) GetMetadata(ctx context.Context, key string) ([]byte, error)     { return nil, nil }
