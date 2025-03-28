package block

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cosmossdk.io/log"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/libp2p/go-libp2p/core/crypto"
	coreda "github.com/rollkit/rollkit/core/da"
	coreexecutor "github.com/rollkit/rollkit/core/execution"
	"github.com/rollkit/rollkit/pkg/cache"
	"github.com/rollkit/rollkit/pkg/genesis"
	"github.com/rollkit/rollkit/pkg/store"
	"github.com/rollkit/rollkit/types"
)

// MockDALC implements the coreda.Client interface for testing
type MockDALC struct {
	mock.Mock
}

func (m *MockDALC) Submit(ctx context.Context, blobs []coreda.Blob, maxBlobSize uint64, gasPrice float64) coreda.ResultSubmit {
	args := m.Called(ctx, blobs, maxBlobSize, gasPrice)
	return args.Get(0).(coreda.ResultSubmit)
}

func (m *MockDALC) Retrieve(ctx context.Context, height uint64) coreda.ResultRetrieve {
	args := m.Called(ctx, height)
	return args.Get(0).(coreda.ResultRetrieve)
}

func (m *MockDALC) MaxBlobSize(ctx context.Context) (uint64, error) {
	args := m.Called(ctx)
	return args.Get(0).(uint64), args.Error(1)
}

func (m *MockDALC) GasPrice(ctx context.Context) (float64, error) {
	args := m.Called(ctx)
	return args.Get(0).(float64), args.Error(1)
}

func (m *MockDALC) GasMultiplier(ctx context.Context) (float64, error) {
	args := m.Called(ctx)
	return args.Get(0).(float64), args.Error(1)
}

// GetNamespace returns the namespace for the DA layer.
func (m *MockDALC) GetNamespace(ctx context.Context) ([]byte, error) {
	args := m.Called(ctx)
	return args.Get(0).([]byte), args.Error(1)
}

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

func (m *MockStoreForSync) SetMetadata(ctx context.Context, key string, value []byte) error {
	args := m.Called(ctx, key, value)
	return args.Error(0)
}

// TestManager extends Manager to allow overriding methods for testing
type TestManager struct {
	*Manager                     // Change to pointer to avoid copying
	fetchHeadersFunc             func(ctx context.Context, height uint64) (coreda.ResultRetrieve, error)
	isUsingExpectedSequencerFunc func(header *types.SignedHeader) bool
	setDAIncludedHeightFunc      func(ctx context.Context, height uint64) error
}

// Override methods for testing
func (m *TestManager) fetchHeaders(ctx context.Context, height uint64) (coreda.ResultRetrieve, error) {
	if m.fetchHeadersFunc != nil {
		return m.fetchHeadersFunc(ctx, height)
	}
	return m.Manager.fetchHeaders(ctx, height)
}

func (m *TestManager) isUsingExpectedCentralizedSequencer(header *types.SignedHeader) bool {
	if m.isUsingExpectedSequencerFunc != nil {
		return m.isUsingExpectedSequencerFunc(header)
	}
	return m.Manager.isUsingExpectedCentralizedSequencer(header)
}

func (m *TestManager) setDAIncludedHeight(ctx context.Context, height uint64) error {
	if m.setDAIncludedHeightFunc != nil {
		return m.setDAIncludedHeightFunc(ctx, height)
	}
	return m.Manager.setDAIncludedHeight(ctx, height)
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

func TestProcessNextDAHeader_Success(t *testing.T) {
	// Create test context
	ctx := context.Background()

	// Create header cache and channel
	headerCache := cache.NewCache[types.SignedHeader]()
	headerInCh := make(chan NewHeaderEvent, 10)

	// Create test logger
	logger := log.NewTestLogger(t)

	// Create mock store
	mockStore := new(MockStoreForSync)

	// Set initial daHeight
	initialDAHeight := uint64(100)

	// Generate a key pair for signing
	privKey, pubKey, err := crypto.GenerateKeyPair(crypto.Ed25519, 256)
	require.NoError(t, err)

	// Create a proper signer with the public key
	signer, err := types.NewSigner(pubKey)
	require.NoError(t, err)

	// Create genesis data
	genesisData := genesis.Genesis{
		ChainID: "test-chain",
		ExtraData: genesis.GenesisExtraData{
			ProposerAddress: signer.Address, // Use signer.Address for genesis
		},
	}

	// Create valid header for testing
	validHeader := &types.SignedHeader{
		Header: types.Header{
			Version: types.Version{
				Block: 1,
				App:   2,
			},
			BaseHeader: types.BaseHeader{
				ChainID: "test-chain",
				Height:  101,
				Time:    uint64(time.Now().UTC().UnixNano()),
			},
			ProposerAddress: signer.Address, // Use signer.Address for proposer
		},
		Signature: types.Signature([]byte("test-signature")),
		Signer:    signer,
	}

	// Create a valid vote to test signature verification
	vote, err := validHeader.Header.Vote()
	require.NoError(t, err)

	// Sign the vote with the private key
	signature, err := privKey.Sign(vote)
	require.NoError(t, err)

	// Update the signature in the header
	validHeader.Signature = signature

	// Convert header to protobuf for transmission
	headerPb, err := validHeader.ToProto()
	require.NoError(t, err)

	headerBytes, err := proto.Marshal(headerPb)
	require.NoError(t, err)

	// Create mock DALC
	mockDALC := new(MockDALC)
	// Set up default responses for any method calls required to avoid nil pointer dereference
	mockDALC.On("MaxBlobSize", mock.Anything).Return(uint64(1024), nil)
	mockDALC.On("GetNamespace", mock.Anything).Return([]byte("test-namespace"), nil)

	// For successful test case
	successRetrieve := coreda.ResultRetrieve{
		BaseResult: coreda.BaseResult{
			Code:    coreda.StatusSuccess,
			Height:  initialDAHeight,
			Message: "success",
		},
		Data: [][]byte{headerBytes},
	}

	// Set up mock for successful retrieval
	mockDALC.On("Retrieve", mock.Anything, initialDAHeight).Return(successRetrieve).Once()

	// Setup mock for SetMetadata
	mockStore.On("SetMetadata", mock.Anything, DAIncludedHeightKey, mock.Anything).Return(nil)

	// Create base manager
	baseManager := Manager{
		store:        mockStore,
		headerCache:  headerCache,
		headerInCh:   headerInCh,
		logger:       logger,
		genesis:      genesisData,
		dalc:         mockDALC,
		lastStateMtx: new(sync.RWMutex),
	}

	// Set daHeight atomically
	atomic.StoreUint64(&baseManager.daHeight, initialDAHeight)

	// Create test manager
	testManager := &TestManager{
		Manager: &baseManager,
	}

	// Override sequencer validation to always succeed
	testManager.isUsingExpectedSequencerFunc = func(header *types.SignedHeader) bool {
		return true
	}

	// Configure setDAIncludedHeight to succeed
	testManager.setDAIncludedHeightFunc = func(ctx context.Context, height uint64) error {
		require.Equal(t, uint64(101), height) // Should be called with header height
		return nil
	}

	// Execute the method
	err = testManager.processNextDAHeader(ctx)
	require.NoError(t, err)

	// Check that header was marked as DA included
	blockHash := validHeader.Hash().String()
	require.True(t, headerCache.IsDAIncluded(blockHash))

	// Verify header was sent to channel
	select {
	case event := <-headerInCh:
		require.NotNil(t, event.Header)
		require.Equal(t, validHeader.Height(), event.Header.Height())
		require.Equal(t, initialDAHeight, event.DAHeight)
	default:
		t.Fatal("Expected header to be sent to headerInCh")
	}
}

// TestProcessNextDAHeader_NotFound tests the case when DA layer returns StatusNotFound
func TestProcessNextDAHeader_NotFound(t *testing.T) {
	// Create test context
	ctx := context.Background()

	// Create header cache and channel
	headerCache := cache.NewCache[types.SignedHeader]()
	headerInCh := make(chan NewHeaderEvent, 10)

	// Create test logger
	logger := log.NewTestLogger(t)

	// Create mock store
	mockStore := new(MockStoreForSync)

	// Set initial daHeight
	initialDAHeight := uint64(100)

	// Create mock DALC
	mockDALC := new(MockDALC)
	// Set up default responses
	mockDALC.On("MaxBlobSize", mock.Anything).Return(uint64(1024), nil)
	mockDALC.On("GetNamespace", mock.Anything).Return([]byte("test-namespace"), nil)

	// Configure for not found response
	mockDALC.On("Retrieve", mock.Anything, initialDAHeight).Return(coreda.ResultRetrieve{
		BaseResult: coreda.BaseResult{
			Code:    coreda.StatusNotFound,
			Message: "no header found",
		},
	}).Once()

	// Create genesis data
	genesisData := genesis.Genesis{
		ChainID: "test-chain",
		ExtraData: genesis.GenesisExtraData{
			ProposerAddress: []byte("test-proposer-address"),
		},
	}

	// Create base manager
	baseManager := Manager{
		store:        mockStore,
		headerCache:  headerCache,
		headerInCh:   headerInCh,
		logger:       logger,
		genesis:      genesisData,
		dalc:         mockDALC,
		lastStateMtx: new(sync.RWMutex),
	}

	// Set daHeight atomically
	atomic.StoreUint64(&baseManager.daHeight, initialDAHeight)

	// Create test manager
	testManager := &TestManager{
		Manager: &baseManager,
	}

	// Execute the method - should return nil for NotFound
	err := testManager.processNextDAHeader(ctx)
	require.NoError(t, err)
}

// TestProcessNextDAHeader_InvalidSequencer tests header rejection when sequencer is not expected
func TestProcessNextDAHeader_InvalidSequencer(t *testing.T) {
	// Create test context
	ctx := context.Background()

	// Create header cache and channel
	headerCache := cache.NewCache[types.SignedHeader]()
	headerInCh := make(chan NewHeaderEvent, 10)

	// Create test logger
	logger := log.NewTestLogger(t)

	// Create mock store
	mockStore := new(MockStoreForSync)

	// Set initial daHeight
	initialDAHeight := uint64(100)

	// Generate a key pair for signing
	privKey, pubKey, err := crypto.GenerateKeyPair(crypto.Ed25519, 256)
	require.NoError(t, err)

	// Create a proper signer with the public key
	signer, err := types.NewSigner(pubKey)
	require.NoError(t, err)

	// Create valid header for testing
	validHeader := &types.SignedHeader{
		Header: types.Header{
			Version: types.Version{
				Block: 1,
				App:   2,
			},
			BaseHeader: types.BaseHeader{
				ChainID: "test-chain",
				Height:  101,
				Time:    uint64(time.Now().UTC().UnixNano()),
			},
			ProposerAddress: signer.Address,
		},
		Signature: types.Signature([]byte("test-signature")),
		Signer:    signer,
	}

	// Create a valid vote to test signature verification
	vote, err := validHeader.Header.Vote()
	require.NoError(t, err)

	// Sign the vote with the private key
	signature, err := privKey.Sign(vote)
	require.NoError(t, err)

	// Update the signature in the header
	validHeader.Signature = signature

	// Convert header to protobuf for transmission
	headerPb, err := validHeader.ToProto()
	require.NoError(t, err)

	headerBytes, err := proto.Marshal(headerPb)
	require.NoError(t, err)

	// Create mock DALC
	mockDALC := new(MockDALC)
	// Set up default responses for any method calls required to avoid nil pointer dereference
	mockDALC.On("MaxBlobSize", mock.Anything).Return(uint64(1024), nil)
	mockDALC.On("GetNamespace", mock.Anything).Return([]byte("test-namespace"), nil)

	// Configure for header retrieval
	mockDALC.On("Retrieve", mock.Anything, initialDAHeight).Return(coreda.ResultRetrieve{
		BaseResult: coreda.BaseResult{
			Code:   coreda.StatusSuccess,
			Height: initialDAHeight,
		},
		Data: [][]byte{headerBytes},
	}).Once()

	// Create genesis data with different proposer address
	genesisData := genesis.Genesis{
		ChainID: "test-chain",
		ExtraData: genesis.GenesisExtraData{
			ProposerAddress: []byte("different-proposer-address"), // Different from the signer's address
		},
	}

	// Create base manager
	baseManager := Manager{
		store:        mockStore,
		headerCache:  headerCache,
		headerInCh:   headerInCh,
		logger:       logger,
		genesis:      genesisData,
		dalc:         mockDALC,
		lastStateMtx: new(sync.RWMutex),
	}

	// Set daHeight atomically
	atomic.StoreUint64(&baseManager.daHeight, initialDAHeight)

	// Create test manager - don't override isUsingExpectedCentralizedSequencer
	testManager := &TestManager{
		Manager: &baseManager,
	}

	// Execute the method
	err = testManager.processNextDAHeader(ctx)
	require.NoError(t, err)

	// Verify that no header was sent to the channel
	require.Empty(t, headerInCh, "No header should have been sent to headerInCh")
}

// TestProcessNextDAHeader_UnmarshalError tests handling of invalid header data
func TestProcessNextDAHeader_UnmarshalError(t *testing.T) {
	// Create test context
	ctx := context.Background()

	// Create header cache and channel
	headerCache := cache.NewCache[types.SignedHeader]()
	headerInCh := make(chan NewHeaderEvent, 10)

	// Create test logger
	logger := log.NewTestLogger(t)

	// Create mock store
	mockStore := new(MockStoreForSync)

	// Set initial daHeight
	initialDAHeight := uint64(100)

	// Create mock DALC
	mockDALC := new(MockDALC)
	// Set up default responses
	mockDALC.On("MaxBlobSize", mock.Anything).Return(uint64(1024), nil)
	mockDALC.On("GetNamespace", mock.Anything).Return([]byte("test-namespace"), nil)

	// Configure for invalid header bytes
	mockDALC.On("Retrieve", mock.Anything, initialDAHeight).Return(coreda.ResultRetrieve{
		BaseResult: coreda.BaseResult{
			Code:   coreda.StatusSuccess,
			Height: initialDAHeight,
		},
		Data: [][]byte{[]byte("invalid-header-bytes")}, // Invalid protobuf
	}).Once()

	// Create genesis data
	genesisData := genesis.Genesis{
		ChainID: "test-chain",
		ExtraData: genesis.GenesisExtraData{
			ProposerAddress: []byte("test-proposer-address"),
		},
	}

	// Create base manager
	baseManager := Manager{
		store:        mockStore,
		headerCache:  headerCache,
		headerInCh:   headerInCh,
		logger:       logger,
		genesis:      genesisData,
		dalc:         mockDALC,
		lastStateMtx: new(sync.RWMutex),
	}

	// Set daHeight atomically
	atomic.StoreUint64(&baseManager.daHeight, initialDAHeight)

	// Create test manager
	testManager := &TestManager{
		Manager: &baseManager,
	}

	// Execute the method
	err := testManager.processNextDAHeader(ctx)
	require.NoError(t, err) // Should handle unmarshal errors gracefully
}

// TestProcessNextDAHeader_FetchError tests the error handling during DA header fetching
func TestProcessNextDAHeader_FetchError(t *testing.T) {
	// Create test context
	ctx := context.Background()

	// Create header cache and channel
	headerCache := cache.NewCache[types.SignedHeader]()
	headerInCh := make(chan NewHeaderEvent, 10)

	// Create test logger
	logger := log.NewTestLogger(t)

	// Create mock store
	mockStore := new(MockStoreForSync)

	// Set initial daHeight
	initialDAHeight := uint64(100)

	// Create genesis data
	genesisData := genesis.Genesis{
		ChainID: "test-chain",
		ExtraData: genesis.GenesisExtraData{
			ProposerAddress: []byte("test-proposer-address"),
		},
	}

	// Create mock DALC that will trigger an error
	mockDALC := new(MockDALC)
	mockDALC.On("MaxBlobSize", mock.Anything).Return(uint64(1024), nil)
	mockDALC.On("GetNamespace", mock.Anything).Return([]byte("test-namespace"), nil)

	// Make the Retrieve method return an error directly - no need for fetchHeadersFunc override
	testError := errors.New("fetch error")
	mockDALC.On("Retrieve", mock.Anything, initialDAHeight).Return(coreda.ResultRetrieve{
		BaseResult: coreda.BaseResult{
			Code:    coreda.StatusError,
			Message: testError.Error(),
		},
	}, nil)

	// Create Manager with all required components
	baseManager := Manager{
		store:        mockStore,
		headerCache:  headerCache,
		headerInCh:   headerInCh,
		logger:       logger,
		genesis:      genesisData,
		dalc:         mockDALC,
		lastStateMtx: new(sync.RWMutex),
	}

	// Set daHeight atomically
	atomic.StoreUint64(&baseManager.daHeight, initialDAHeight)

	// Create test manager
	testManager := &TestManager{
		Manager: &baseManager,
	}

	// Execute the method expecting an error
	err := testManager.processNextDAHeader(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "fetch error")
}
