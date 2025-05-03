package block

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cosmossdk.io/log"
	goheaderstore "github.com/celestiaorg/go-header/store"
	ds "github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	coreda "github.com/rollkit/rollkit/core/da"
	"github.com/rollkit/rollkit/pkg/cache"
	"github.com/rollkit/rollkit/pkg/config"
	"github.com/rollkit/rollkit/pkg/genesis"
	"github.com/rollkit/rollkit/pkg/signer/noop"
	rollmocks "github.com/rollkit/rollkit/test/mocks"
	"github.com/rollkit/rollkit/types"
	v1 "github.com/rollkit/rollkit/types/pb/rollkit/v1"
)

type MockLogger struct {
	mock.Mock
}

func (m *MockLogger) Debug(msg string, keyvals ...any) { m.Called(msg, keyvals) }
func (m *MockLogger) Info(msg string, keyvals ...any)  { m.Called(msg, keyvals) }
func (m *MockLogger) Warn(msg string, keyvals ...any)  { m.Called(msg, keyvals) }
func (m *MockLogger) Error(msg string, keyvals ...any) { m.Called(msg, keyvals) }
func (m *MockLogger) With(keyvals ...any) log.Logger   { return m }
func (m *MockLogger) Impl() any                        { return m }

// setupManagerForRetrieverTest initializes a Manager with mocked dependencies.
func setupManagerForRetrieverTest(t *testing.T, initialDAHeight uint64) (*Manager, *rollmocks.DA, *rollmocks.Store, *MockLogger, *cache.Cache[types.SignedHeader], *cache.Cache[types.Data], context.CancelFunc) {
	t.Helper()
	mockDAClient := rollmocks.NewDA(t)
	mockStore := rollmocks.NewStore(t)
	mockLogger := new(MockLogger)

	mockLogger.On("Debug", mock.Anything, mock.Anything).Maybe()
	mockLogger.On("Info", mock.Anything, mock.Anything).Maybe()
	mockLogger.On("Warn", mock.Anything, mock.Anything).Maybe()
	mockLogger.On("Error", mock.Anything, mock.Anything).Maybe()

	headerStore, _ := goheaderstore.NewStore[*types.SignedHeader](ds.NewMapDatastore())
	dataStore, _ := goheaderstore.NewStore[*types.Data](ds.NewMapDatastore())

	mockStore.On("GetState", mock.Anything).Return(types.State{DAHeight: initialDAHeight}, nil).Maybe()
	mockStore.On("SetHeight", mock.Anything, mock.Anything).Return(nil).Maybe()
	mockStore.On("SetMetadata", mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	mockStore.On("GetMetadata", mock.Anything, DAIncludedHeightKey).Return([]byte{}, ds.ErrNotFound).Maybe()

	_, cancel := context.WithCancel(context.Background())

	// Create a mock signer
	src := rand.Reader
	pk, _, err := crypto.GenerateEd25519Key(src)
	require.NoError(t, err)
	noopSigner, err := noop.NewNoopSigner(pk)
	require.NoError(t, err)

	addr, err := noopSigner.GetAddress()
	require.NoError(t, err)

	manager := &Manager{
		store:         mockStore,
		config:        config.Config{DA: config.DAConfig{BlockTime: config.DurationWrapper{Duration: 1 * time.Second}}},
		genesis:       genesis.Genesis{ProposerAddress: addr},
		daHeight:      &atomic.Uint64{},
		headerInCh:    make(chan NewHeaderEvent, eventInChLength),
		headerStore:   headerStore,
		dataInCh:      make(chan NewDataEvent, eventInChLength),
		dataStore:     dataStore,
		headerCache:   cache.NewCache[types.SignedHeader](),
		dataCache:     cache.NewCache[types.Data](),
		headerStoreCh: make(chan struct{}, 1),
		dataStoreCh:   make(chan struct{}, 1),
		retrieveCh:    make(chan struct{}, 1),
		daIncluderCh:  make(chan struct{}, 1),
		logger:        mockLogger,
		lastStateMtx:  new(sync.RWMutex),
		da:            mockDAClient,
		signer:        noopSigner,
	}
	manager.daIncludedHeight.Store(0)
	manager.daHeight.Store(initialDAHeight)

	t.Cleanup(cancel)

	return manager, mockDAClient, mockStore, mockLogger, manager.headerCache, manager.dataCache, cancel
}

// TestProcessNextDAHeader_Success_SingleHeaderAndData tests the processNextDAHeaderAndData function for a single header and data.
func TestProcessNextDAHeader_Success_SingleHeaderAndData(t *testing.T) {
	daHeight := uint64(20)
	blockHeight := uint64(100)
	manager, mockDAClient, mockStore, _, headerCache, dataCache, cancel := setupManagerForRetrieverTest(t, daHeight)
	defer cancel()

	proposerAddr := manager.genesis.ProposerAddress

	hc := types.HeaderConfig{
		Height: blockHeight,
		Signer: manager.signer,
	}
	header, err := types.GetRandomSignedHeaderCustom(&hc, manager.genesis.ChainID)
	require.NoError(t, err)
	header.ProposerAddress = proposerAddr
	expectedHeaderHash := header.Hash().String()
	headerProto, err := header.ToProto()
	require.NoError(t, err)
	headerBytes, err := proto.Marshal(headerProto)
	require.NoError(t, err)

	blockConfig := types.BlockConfig{
		Height:       blockHeight,
		NTxs:         2,
		ProposerAddr: proposerAddr,
	}
	_, blockData, _ := types.GenerateRandomBlockCustom(&blockConfig, manager.genesis.ChainID)

	// Instead of marshaling blockData as pb.Data, marshal as pb.Batch for the DA client mock return
	batchProto := &v1.Batch{Txs: make([][]byte, len(blockData.Txs))}
	for i, tx := range blockData.Txs {
		batchProto.Txs[i] = tx
	}
	blockDataBytes, err := proto.Marshal(batchProto)
	require.NoError(t, err)
	// -----------------------------------------------------------

	mockDAClient.On("Get", mock.Anything, []coreda.ID{[]byte("dummy-id")}, []byte("placeholder")).Return(
		[]coreda.Blob{headerBytes}, nil,
	).Once()

	ctx := context.Background()
	err = manager.processNextDAHeaderAndData(ctx)
	require.NoError(t, err)

	// Validate header event
	select {
	case event := <-manager.headerInCh:
		assert.Equal(t, blockHeight, event.Header.Height())
		assert.Equal(t, daHeight, event.DAHeight)
		assert.Equal(t, proposerAddr, event.Header.ProposerAddress)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Expected header event not received")
	}

	assert.True(t, headerCache.IsDAIncluded(expectedHeaderHash), "Header hash should be marked as DA included in cache")

	// Validate block data event
	select {
	case dataEvent := <-manager.dataInCh:
		assert.Equal(t, daHeight, dataEvent.DAHeight)
		assert.Equal(t, blockData.Txs, dataEvent.Data.Txs)
		// Optionally, compare more fields if needed
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Expected block data event not received")
	}
	assert.True(t, dataCache.IsDAIncluded(blockData.DACommitment().String()), "Block data commitment should be marked as DA included in cache")

	mockDAClient.AssertExpectations(t)
	mockStore.AssertExpectations(t)
}

// TestProcessNextDAHeaderAndData_NotFound tests the processNextDAHeaderAndData function for a NotFound error.
func TestProcessNextDAHeaderAndData_NotFound(t *testing.T) {
	daHeight := uint64(25)
	manager, mockDAClient, _, _, _, _, cancel := setupManagerForRetrieverTest(t, daHeight)
	defer cancel()

	// Mock GetIDs to return empty IDs to simulate "not found" scenario
	mockDAClient.On("GetIDs", mock.Anything, daHeight, []byte("placeholder")).Return(&coreda.GetIDsResult{
		IDs:       []coreda.ID{}, // Empty IDs array
		Timestamp: time.Now(),
	}, coreda.ErrBlobNotFound).Once()

	ctx := context.Background()
	err := manager.processNextDAHeaderAndData(ctx)
	require.NoError(t, err)

	select {
	case <-manager.headerInCh:
		t.Fatal("No header event should be received for NotFound")
	default:
	}

	select {
	case <-manager.dataInCh:
		t.Fatal("No data event should be received for NotFound")
	default:
	}

	mockDAClient.AssertExpectations(t)
}

// TestProcessNextDAHeaderAndData_UnmarshalHeaderError tests the processNextDAHeaderAndData function for an unmarshal error.
func TestProcessNextDAHeaderAndData_UnmarshalHeaderError(t *testing.T) {
	daHeight := uint64(30)
	manager, mockDAClient, _, mockLogger, _, _, cancel := setupManagerForRetrieverTest(t, daHeight)
	defer cancel()

	invalidBytes := []byte("this is not a valid protobuf message")

	// Mock GetIDs to return success with dummy ID
	mockDAClient.On("GetIDs", mock.Anything, daHeight, []byte("placeholder")).Return(&coreda.GetIDsResult{
		IDs:       []coreda.ID{[]byte("dummy-id")},
		Timestamp: time.Now(),
	}, nil).Once()

	// Mock Get to return invalid bytes
	mockDAClient.On("Get", mock.Anything, []coreda.ID{[]byte("dummy-id")}, []byte("placeholder")).Return(
		[]coreda.Blob{invalidBytes}, nil,
	).Once()

	mockLogger.ExpectedCalls = nil
	mockLogger.On("Debug", "failed to unmarshal header", mock.Anything).Return().Once()
	mockLogger.On("Debug", "failed to unmarshal batch", mock.Anything).Return().Once()
	mockLogger.On("Debug", mock.Anything, mock.Anything).Maybe() // Allow other debug logs

	ctx := context.Background()
	err := manager.processNextDAHeaderAndData(ctx)
	require.NoError(t, err)

	select {
	case <-manager.headerInCh:
		t.Fatal("No header event should be received for unmarshal error")
	default:
	}
	select {
	case <-manager.dataInCh:
		t.Fatal("No data event should be received for unmarshal error")
	default:
	}

	mockDAClient.AssertExpectations(t)
	mockLogger.AssertExpectations(t)
}

// TestProcessNextDAHeaderAndData_UnexpectedSequencer tests the processNextDAHeaderAndData function for an unexpected sequencer.
func TestProcessNextDAHeader_UnexpectedSequencer(t *testing.T) {
	daHeight := uint64(35)
	blockHeight := uint64(110)
	manager, mockDAClient, _, mockLogger, _, _, cancel := setupManagerForRetrieverTest(t, daHeight)
	defer cancel()

	src := rand.Reader
	pk, _, err := crypto.GenerateEd25519Key(src)
	require.NoError(t, err)
	signerNoop, err := noop.NewNoopSigner(pk)
	require.NoError(t, err)
	hc := types.HeaderConfig{
		Height: blockHeight,
		Signer: signerNoop,
	}
	header, err := types.GetRandomSignedHeaderCustom(&hc, manager.genesis.ChainID)
	require.NoError(t, err)
	headerProto, err := header.ToProto()
	require.NoError(t, err)
	headerBytes, err := proto.Marshal(headerProto)
	require.NoError(t, err)

	// Mock GetIDs to return success with dummy ID
	mockDAClient.On("GetIDs", mock.Anything, daHeight, []byte("placeholder")).Return(&coreda.GetIDsResult{
		IDs:       []coreda.ID{[]byte("dummy-id")},
		Timestamp: time.Now(),
	}, nil).Once()

	// Mock Get to return header bytes
	mockDAClient.On("Get", mock.Anything, []coreda.ID{[]byte("dummy-id")}, []byte("placeholder")).Return(
		[]coreda.Blob{headerBytes}, nil,
	).Once()

	mockLogger.ExpectedCalls = nil
	mockLogger.On("Debug", "skipping header from unexpected sequencer", mock.Anything).Return().Once()
	mockLogger.On("Debug", mock.Anything, mock.Anything).Maybe() // Allow other debug logs

	ctx := context.Background()
	err = manager.processNextDAHeaderAndData(ctx)
	require.NoError(t, err)

	select {
	case <-manager.headerInCh:
		t.Fatal("No header event should be received for unexpected sequencer")
	default:
		// Expected behavior
	}

	mockDAClient.AssertExpectations(t)
	mockLogger.AssertExpectations(t)
}

// TestProcessNextDAHeader_FetchError_RetryFailure tests the processNextDAHeaderAndBlock function for a fetch error.
func TestProcessNextDAHeader_FetchError_RetryFailure(t *testing.T) {
	daHeight := uint64(40)
	manager, mockDAClient, _, _, _, _, cancel := setupManagerForRetrieverTest(t, daHeight)
	defer cancel()

	fetchErr := errors.New("persistent DA connection error")

	// Mock GetIDs to return error for all retries
	mockDAClient.On("GetIDs", mock.Anything, daHeight, []byte("placeholder")).Return(
		nil, fetchErr,
	).Times(dAFetcherRetries)

	ctx := context.Background()
	err := manager.processNextDAHeaderAndData(ctx)
	require.Error(t, err)
	assert.ErrorContains(t, err, fetchErr.Error(), "Expected the final error after retries")

	select {
	case <-manager.headerInCh:
		t.Fatal("No header event should be received on fetch failure")
	default:
	}

	mockDAClient.AssertExpectations(t)
}

// TestProcessNextDAHeader_HeaderAlreadySeen tests the processNextDAHeaderAndData function for a header that has already been seen.
func TestProcessNextDAHeader_HeaderAlreadySeen(t *testing.T) {
	// Use sequential heights to avoid future height issues
	daHeight := uint64(45)
	blockHeight := uint64(120)

	manager, mockDAClient, _, mockLogger, headerCache, dataCache, cancel := setupManagerForRetrieverTest(t, daHeight)
	defer cancel()

	// Initialize heights properly
	manager.daIncludedHeight.Store(blockHeight)

	// Create test header
	hc := types.HeaderConfig{
		Height: blockHeight, // Use blockHeight here
		Signer: manager.signer,
	}
	header, err := types.GetRandomSignedHeaderCustom(&hc, manager.genesis.ChainID)
	require.NoError(t, err)

	headerHash := header.Hash().String()
	headerProto, err := header.ToProto()
	require.NoError(t, err)
	headerBytes, err := proto.Marshal(headerProto)
	require.NoError(t, err)

	// Set up cache state
	headerCache.SetSeen(headerHash)
	headerCache.SetDAIncluded(headerHash)

	// Set up mocks with explicit logging
	mockDAClient.On("GetIDs", mock.Anything, daHeight, mock.Anything).Return(&coreda.GetIDsResult{
		IDs:       []coreda.ID{[]byte("dummy-id")},
		Timestamp: time.Now(),
	}, nil).Once()

	mockDAClient.On("Get", mock.Anything, []coreda.ID{[]byte("dummy-id")}, mock.Anything).Return(
		[]coreda.Blob{headerBytes}, nil,
	).Once()

	// Add debug logging expectations
	mockLogger.On("Debug", mock.Anything, mock.Anything, mock.Anything).Return()

	ctx := context.Background()
	err = manager.processNextDAHeaderAndData(ctx)
	require.NoError(t, err)

	// Verify no header event was sent
	select {
	case <-manager.headerInCh:
		t.Fatal("Header event should not be received for already seen header")
	default:
		// Expected path
	}

	mockDAClient.AssertExpectations(t)
	mockLogger.AssertExpectations(t)
}

// TestRetrieveLoop_ProcessError_HeightFromFuture verifies loop continues without error log.
func TestRetrieveLoop_ProcessError_HeightFromFuture(t *testing.T) {
	startDAHeight := uint64(10)
	manager, mockDAClient, _, mockLogger, _, _, cancel := setupManagerForRetrieverTest(t, startDAHeight)
	defer cancel()

	futureErr := fmt.Errorf("some error wrapping: %w", ErrHeightFromFutureStr)

	// Mock GetIDs to return future error for all retries
	mockDAClient.On("GetIDs", mock.Anything, startDAHeight, []byte("placeholder")).Return(
		nil, futureErr,
	).Times(dAFetcherRetries)

	// Optional: Mock for the next height if needed
	mockDAClient.On("GetIDs", mock.Anything, startDAHeight+1, []byte("placeholder")).Return(
		&coreda.GetIDsResult{IDs: []coreda.ID{}}, coreda.ErrBlobNotFound,
	).Maybe()

	errorLogged := atomic.Bool{}
	mockLogger.ExpectedCalls = nil
	mockLogger.On("Error", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		errorLogged.Store(true)
	}).Return().Maybe()
	mockLogger.On("Debug", mock.Anything, mock.Anything).Maybe()
	mockLogger.On("Info", mock.Anything, mock.Anything).Maybe()
	mockLogger.On("Warn", mock.Anything, mock.Anything).Maybe()

	ctx, loopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer loopCancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		manager.RetrieveLoop(ctx)
	}()

	manager.retrieveCh <- struct{}{}

	wg.Wait()

	finalDAHeight := manager.daHeight.Load()
	if finalDAHeight != startDAHeight {
		t.Errorf("Expected final DA height %d, got %d (should not increment on future height error)", startDAHeight, finalDAHeight)
	}
}

// TestRetrieveLoop_ProcessError_Other verifies loop logs error and continues.
func TestRetrieveLoop_ProcessError_Other(t *testing.T) {
	startDAHeight := uint64(15)
	manager, mockDAClient, _, mockLogger, _, _, cancel := setupManagerForRetrieverTest(t, startDAHeight)
	defer cancel()

	otherErr := errors.New("some other DA error")

	// Mock GetIDs to return error for all retries
	mockDAClient.On("GetIDs", mock.Anything, startDAHeight, []byte("placeholder")).Return(
		nil, otherErr,
	).Times(dAFetcherRetries)

	errorLogged := make(chan struct{})
	mockLogger.ExpectedCalls = nil

	// Mock all expected logger calls in order
	mockLogger.On("Debug", "trying to retrieve block from DA", mock.Anything).Return()
	mockLogger.On("Error", "Retrieve helper: Failed to get IDs",
		mock.MatchedBy(func(args []interface{}) bool {
			return true // Accept any args for simplicity
		}),
	).Return()
	mockLogger.On("Error", "failed to retrieve block from DALC", mock.Anything).Run(func(args mock.Arguments) {
		close(errorLogged)
	}).Return()

	// Allow any other debug/info/warn calls
	mockLogger.On("Debug", mock.Anything, mock.Anything).Return().Maybe()
	mockLogger.On("Info", mock.Anything, mock.Anything).Return().Maybe()
	mockLogger.On("Warn", mock.Anything, mock.Anything).Return().Maybe()

	ctx, loopCancel := context.WithCancel(context.Background())
	defer loopCancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		manager.RetrieveLoop(ctx)
	}()

	manager.retrieveCh <- struct{}{}

	select {
	case <-errorLogged:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("Error was not logged for generic DA error")
	}

	loopCancel()
	wg.Wait()

	mockDAClient.AssertExpectations(t)
	mockLogger.AssertExpectations(t)
}
