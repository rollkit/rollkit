package block

import (
	// ... other necessary imports ...
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"cosmossdk.io/log"
	ds "github.com/ipfs/go-datastore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/pkg/config"
	"github.com/rollkit/rollkit/pkg/signer/noop"
	extmocks "github.com/rollkit/rollkit/test/mocks/external"
	"github.com/rollkit/rollkit/types"

	// Use existing store mock if available, or define one
	mocksStore "github.com/rollkit/rollkit/test/mocks"
)

func setupManagerForStoreRetrieveTest(t *testing.T) (
	m *Manager,
	mockStore *mocksStore.Store,
	mockHeaderStore *extmocks.Store[*types.SignedHeader],
	mockDataStore *extmocks.Store[*types.Data],
	headerStoreCh chan struct{},
	dataStoreCh chan struct{},
	headerInCh chan NewHeaderEvent,
	dataInCh chan NewDataEvent,
	ctx context.Context,
	cancel context.CancelFunc,
) {
	t.Helper()

	// Mocks
	mockStore = mocksStore.NewStore(t)
	mockHeaderStore = extmocks.NewStore[*types.SignedHeader](t)
	mockDataStore = extmocks.NewStore[*types.Data](t)

	// Channels (buffered to prevent deadlocks in simple test cases)
	headerStoreCh = make(chan struct{}, 1)
	dataStoreCh = make(chan struct{}, 1)
	headerInCh = make(chan NewHeaderEvent, 10)
	dataInCh = make(chan NewDataEvent, 10)

	// Config & Genesis
	nodeConf := config.DefaultConfig
	genDoc, pk, _ := types.GetGenesisWithPrivkey("test") // Use test helper

	logger := log.NewNopLogger()
	ctx, cancel = context.WithCancel(context.Background())

	// Mock initial metadata reads during manager creation if necessary
	mockStore.On("GetMetadata", mock.Anything, DAIncludedHeightKey).Return(nil, ds.ErrNotFound).Maybe()
	mockStore.On("GetMetadata", mock.Anything, LastBatchDataKey).Return(nil, ds.ErrNotFound).Maybe()

	signer, err := noop.NewNoopSigner(pk)
	require.NoError(t, err)
	// Create Manager instance with mocks and necessary fields
	m = &Manager{
		store:         mockStore,
		headerStore:   mockHeaderStore,
		dataStore:     mockDataStore,
		headerStoreCh: headerStoreCh,
		dataStoreCh:   dataStoreCh,
		headerInCh:    headerInCh,
		dataInCh:      dataInCh,
		logger:        logger,
		genesis:       genDoc,
		// Initialize daHeight, lastStateMtx etc. if needed, though likely not critical for these loops
		lastStateMtx: new(sync.RWMutex),
		config:       nodeConf,
		signer:       signer,
	}
	// Initialize daHeight atomic variable
	m.init(ctx) // Call init to handle potential DAIncludedHeightKey loading

	return m, mockStore, mockHeaderStore, mockDataStore, headerStoreCh, dataStoreCh, headerInCh, dataInCh, ctx, cancel
}

func TestDataStoreRetrieveLoop_RetrievesNewData(t *testing.T) {
	assert := assert.New(t)
	m, mockStore, _, mockDataStore, _, dataStoreCh, _, dataInCh, ctx, cancel := setupManagerForStoreRetrieveTest(t)
	defer cancel()

	initialHeight := uint64(5)
	mockStore.On("Height", ctx).Return(initialHeight, nil).Once()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.DataStoreRetrieveLoop(ctx)
	}()

	// Configure mock

	newHeight := uint64(6)
	expectedData := &types.Data{Metadata: &types.Metadata{Height: newHeight}}

	mockDataStore.On("Height").Return(newHeight).Once() // Height check after trigger
	mockDataStore.On("GetByHeight", ctx, newHeight).Return(expectedData, nil).Once()

	// Trigger the loop
	dataStoreCh <- struct{}{}

	// Verify data received
	select {
	case receivedEvent := <-dataInCh:
		assert.Equal(expectedData, receivedEvent.Data)
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for data event on dataInCh")
	}

	// Cancel context and wait for loop to finish
	cancel()
	wg.Wait()

	// Assert mock expectations
	mockDataStore.AssertExpectations(t)
}

func TestDataStoreRetrieveLoop_RetrievesMultipleData(t *testing.T) {
	assert := assert.New(t)
	m, mockStore, _, mockDataStore, _, dataStoreCh, _, dataInCh, ctx, cancel := setupManagerForStoreRetrieveTest(t)
	defer cancel()

	initialHeight := uint64(5)
	mockStore.On("Height", ctx).Return(initialHeight, nil).Once()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.DataStoreRetrieveLoop(ctx)
	}()

	// Configure mock
	finalHeight := uint64(8) // Retrieve heights 6, 7, 8
	expectedData := make(map[uint64]*types.Data)
	for h := initialHeight + 1; h <= finalHeight; h++ {
		expectedData[h] = &types.Data{Metadata: &types.Metadata{Height: h}}
	}

	mockDataStore.On("Height").Return(finalHeight).Once()
	for h := initialHeight + 1; h <= finalHeight; h++ {
		mockDataStore.On("GetByHeight", mock.Anything, h).Return(expectedData[h], nil).Once()
	}

	// Trigger the loop
	dataStoreCh <- struct{}{}

	// Verify data received
	receivedCount := 0
	expectedCount := len(expectedData)
	timeout := time.After(2 * time.Second)
	for receivedCount < expectedCount {
		select {
		case receivedEvent := <-dataInCh:
			receivedCount++
			h := receivedEvent.Data.Height()
			assert.Contains(expectedData, h)
			assert.Equal(expectedData[h], receivedEvent.Data)
			expectedItem, ok := expectedData[h]
			assert.True(ok, "Received unexpected height: %d", h)
			if ok {
				assert.Equal(expectedItem, receivedEvent.Data)
				delete(expectedData, h)
			}
		case <-timeout:
			t.Fatalf("timed out waiting for data events on dataInCh, received %d out of %d", receivedCount, len(expectedData)+receivedCount)
		}
	}
	assert.Empty(expectedData, "Not all expected data items were received")

	// Cancel context and wait for loop to finish
	cancel()
	wg.Wait()

	// Assert mock expectations
	mockDataStore.AssertExpectations(t)
}

func TestDataStoreRetrieveLoop_NoNewData(t *testing.T) {
	m, mockStore, _, mockDataStore, _, dataStoreCh, _, dataInCh, ctx, cancel := setupManagerForStoreRetrieveTest(t)
	defer cancel()

	currentHeight := uint64(5)
	mockStore.On("Height", ctx).Return(currentHeight, nil).Once()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.DataStoreRetrieveLoop(ctx)
	}()

	mockDataStore.On("Height").Return(currentHeight).Once()

	dataStoreCh <- struct{}{}

	select {
	case receivedEvent := <-dataInCh:
		t.Fatalf("received unexpected data event on dataInCh: %+v", receivedEvent)
	case <-time.After(100 * time.Millisecond):
	}

	cancel()
	wg.Wait()

	mockDataStore.AssertExpectations(t)
}

func TestDataStoreRetrieveLoop_HandlesFetchError(t *testing.T) {

	m, mockStore, _, mockDataStore, _, dataStoreCh, _, dataInCh, ctx, cancel := setupManagerForStoreRetrieveTest(t)
	defer cancel()

	currentHeight := uint64(5)
	mockStore.On("Height", ctx).Return(currentHeight, nil).Once()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.DataStoreRetrieveLoop(ctx)
	}()

	newHeight := uint64(6)
	fetchError := errors.New("failed to fetch data")

	mockDataStore.On("Height").Return(newHeight).Once()
	mockDataStore.On("GetByHeight", mock.Anything, newHeight).Return(nil, fetchError).Once()

	dataStoreCh <- struct{}{}

	select {
	case receivedEvent := <-dataInCh:
		t.Fatalf("received unexpected data event on dataInCh: %+v", receivedEvent)
	case <-time.After(100 * time.Millisecond):
	}

	cancel()
	wg.Wait()

	mockDataStore.AssertExpectations(t)
}

func TestDataStoreRetrieveLoop_ContextCancellation(t *testing.T) {
	m, mockStore, _, mockDataStore, _, _, _, _, ctx, cancel := setupManagerForStoreRetrieveTest(t)
	// Intentionally cancel context immediately after starting the loop

	currentHeight := uint64(5)
	mockStore.On("Height", ctx).Return(currentHeight, nil).Once()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.DataStoreRetrieveLoop(ctx)
	}()

	// Cancel context almost immediately
	cancel()

	// Wait for the loop goroutine to finish, with a timeout
	waitCh := make(chan struct{})
	go func() {
		wg.Wait()
		close(waitCh)
	}()

	select {
	case <-waitCh:
		// Goroutine finished as expected
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for DataStoreRetrieveLoop goroutine to finish after context cancellation")
	}

	// No specific mock expectations needed here, just verifying termination
	mockDataStore.AssertExpectations(t) // Should have no calls expected/made
}

func TestHeaderStoreRetrieveLoop_RetrievesNewHeader(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	m, mockStore, mockHeaderStore, _, headerStoreCh, _, headerInCh, _, ctx, cancel := setupManagerForStoreRetrieveTest(t)
	defer cancel()

	initialHeight := uint64(0)
	newHeight := uint64(1)

	mockStore.On("Height", ctx).Return(initialHeight, nil).Once()

	validHeader, err := types.GetFirstSignedHeader(m.signer, m.genesis.ChainID)
	require.NoError(err)
	require.Equal(m.genesis.ProposerAddress, validHeader.ProposerAddress)

	mockHeaderStore.On("Height").Return(newHeight).Once() // Height check after trigger
	mockHeaderStore.On("GetByHeight", mock.Anything, newHeight).Return(validHeader, nil).Once()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.HeaderStoreRetrieveLoop(ctx)
	}()

	headerStoreCh <- struct{}{}

	select {
	case receivedEvent := <-headerInCh:
		assert.Equal(validHeader, receivedEvent.Header)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for header event on headerInCh")
	}

	cancel()
	wg.Wait()

	mockHeaderStore.AssertExpectations(t)
}

func TestHeaderStoreRetrieveLoop_RetrievesMultipleHeaders(t *testing.T) {
	t.Skip()
	assert := assert.New(t)
	require := require.New(t)

	m, mockStore, mockHeaderStore, _, headerStoreCh, _, headerInCh, _, ctx, cancel := setupManagerForStoreRetrieveTest(t)
	defer cancel()

	initialHeight := uint64(5)
	finalHeight := uint64(8)
	numHeaders := finalHeight - initialHeight

	headers := make([]*types.SignedHeader, numHeaders)
	var lastHeader *types.SignedHeader
	for i := uint64(0); i < numHeaders; i++ {
		currentHeight := initialHeight + 1 + i
		var h *types.SignedHeader
		var err error
		if currentHeight == m.genesis.InitialHeight {
			h, err = types.GetFirstSignedHeader(m.signer, m.genesis.ChainID)
		} else {
			if lastHeader == nil {
				if initialHeight == m.genesis.InitialHeight-1 {
					lastHeader, err = types.GetFirstSignedHeader(m.signer, m.genesis.ChainID)
					require.NoError(err)
				} else {
					dummyHeader, _, err := types.GetRandomSignedHeader(m.genesis.ChainID)
					require.NoError(err)
					dummyHeader.Header.BaseHeader.Height = initialHeight
					lastHeader = dummyHeader
				}
			}
			h, err = types.GetRandomNextSignedHeader(lastHeader, m.signer, m.genesis.ChainID)
		}
		require.NoError(err)
		h.ProposerAddress = m.genesis.ProposerAddress
		headers[i] = h
		lastHeader = h
		mockHeaderStore.On("GetByHeight", ctx, currentHeight).Return(h, nil).Once()
	}

	mockHeaderStore.On("Height").Return(finalHeight).Once()

	mockStore.On("Height", ctx).Return(initialHeight, nil).Once()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.HeaderStoreRetrieveLoop(ctx)
	}()

	headerStoreCh <- struct{}{}

	receivedCount := 0
	timeout := time.After(3 * time.Second)
	expectedHeaders := make(map[uint64]*types.SignedHeader)
	for _, h := range headers {
		expectedHeaders[h.Height()] = h
	}

	for receivedCount < int(numHeaders) {
		select {
		case receivedEvent := <-headerInCh:
			receivedCount++
			h := receivedEvent.Header
			expected, found := expectedHeaders[h.Height()]
			assert.True(found, "Received unexpected header height: %d", h.Height())
			if found {
				assert.Equal(expected, h)
				delete(expectedHeaders, h.Height()) // Remove found header
			}
		case <-timeout:
			t.Fatalf("timed out waiting for all header events on headerInCh, received %d out of %d", receivedCount, numHeaders)
		}
	}

	assert.Empty(expectedHeaders, "Not all expected headers were received")

	// Cancel context and wait for loop to finish
	cancel()
	wg.Wait()

	// Verify mock expectations
	mockHeaderStore.AssertExpectations(t)
}

func TestHeaderStoreRetrieveLoop_NoNewHeaders(t *testing.T) {
	m, mockStore, mockHeaderStore, _, headerStoreCh, _, headerInCh, _, ctx, cancel := setupManagerForStoreRetrieveTest(t)
	defer cancel()

	currentHeight := uint64(5)

	mockStore.On("Height", ctx).Return(currentHeight, nil).Once()
	mockHeaderStore.On("Height").Return(currentHeight).Once()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.HeaderStoreRetrieveLoop(ctx)
	}()

	// Trigger the loop
	headerStoreCh <- struct{}{}

	// Wait briefly and assert nothing is received
	select {
	case receivedEvent := <-headerInCh:
		t.Fatalf("received unexpected header event on headerInCh: %+v", receivedEvent)
	case <-time.After(100 * time.Millisecond):
		// Expected timeout, nothing received
	}

	// Cancel context and wait for loop to finish
	cancel()
	wg.Wait()

	// Verify mock expectations
	mockHeaderStore.AssertExpectations(t)
}
