package block

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cosmossdk.io/log"
	goheaderstore "github.com/celestiaorg/go-header/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/pkg/cache"
	"github.com/rollkit/rollkit/pkg/config"
	"github.com/rollkit/rollkit/pkg/genesis"
	"github.com/rollkit/rollkit/test/mocks"
	"github.com/rollkit/rollkit/types"
)

// setupManagerForSyncLoopTest initializes a Manager instance suitable for SyncLoop testing.
func setupManagerForSyncLoopTest(t *testing.T, initialState types.State) (
	*Manager,
	*mocks.Store,
	*mocks.Executor,
	context.Context,
	context.CancelFunc,
	chan NewHeaderEvent,
	chan NewDataEvent,
	*uint64,
) {
	t.Helper()

	mockStore := mocks.NewStore(t)
	mockExec := mocks.NewExecutor(t)

	headerInCh := make(chan NewHeaderEvent, 10)
	dataInCh := make(chan NewDataEvent, 10)

	headerStoreCh := make(chan struct{}, 1)
	dataStoreCh := make(chan struct{}, 1)
	retrieveCh := make(chan struct{}, 1)

	cfg := config.DefaultConfig
	cfg.DA.BlockTime.Duration = 100 * time.Millisecond
	cfg.Node.BlockTime.Duration = 50 * time.Millisecond
	genesisDoc := &genesis.Genesis{ChainID: "syncLoopTest"}

	// Manager setup
	m := &Manager{
		store:                    mockStore,
		exec:                     mockExec,
		config:                   cfg,
		genesis:                  *genesisDoc,
		lastState:                initialState,
		lastStateMtx:             new(sync.RWMutex),
		logger:                   log.NewTestLogger(t),
		headerCache:              cache.NewCache[types.SignedHeader](),
		dataCache:                cache.NewCache[types.Data](),
		headerInCh:               headerInCh,
		dataInCh:                 dataInCh,
		headerStoreCh:            headerStoreCh,
		dataStoreCh:              dataStoreCh,
		retrieveCh:               retrieveCh,
		daHeight:                 &atomic.Uint64{},
		metrics:                  NopMetrics(),
		headerStore:              &goheaderstore.Store[*types.SignedHeader]{},
		dataStore:                &goheaderstore.Store[*types.Data]{},
		pendingHeaders:           &PendingHeaders{logger: log.NewNopLogger()},
		signaturePayloadProvider: types.CreateDefaultSignaturePayloadProvider(),
	}
	m.daHeight.Store(initialState.DAHeight)

	ctx, cancel := context.WithCancel(context.Background())

	currentMockHeight := initialState.LastBlockHeight
	heightPtr := &currentMockHeight

	mockStore.On("Height", mock.Anything).Return(func(context.Context) uint64 {
		return *heightPtr
	}, nil).Maybe()

	return m, mockStore, mockExec, ctx, cancel, headerInCh, dataInCh, heightPtr
}

// TestSyncLoop_ProcessSingleBlock_HeaderFirst verifies that the sync loop processes a single block when the header arrives before the data.
// 1. Header for H+1 arrives.
// 2. Data for H+1 arrives.
// 3. Block H+1 is successfully validated, applied, and committed.
// 4. State is updated.
// 5. Caches are cleared.
func TestSyncLoop_ProcessSingleBlock_HeaderFirst(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	initialHeight := uint64(10)
	initialState := types.State{
		LastBlockHeight: initialHeight,
		AppHash:         []byte("initial_app_hash"),
		ChainID:         "syncLoopTest",
		DAHeight:        5,
	}
	newHeight := initialHeight + 1
	daHeight := initialState.DAHeight

	m, mockStore, mockExec, ctx, cancel, headerInCh, dataInCh, _ := setupManagerForSyncLoopTest(t, initialState)
	defer cancel()

	// Create test block data
	header, data, privKey := types.GenerateRandomBlockCustomWithAppHash(&types.BlockConfig{Height: newHeight, NTxs: 2}, initialState.ChainID, initialState.AppHash)
	require.NotNil(header)
	require.NotNil(data)
	require.NotNil(privKey)

	expectedNewAppHash := []byte("new_app_hash")
	expectedNewState, err := initialState.NextState(header, expectedNewAppHash)
	require.NoError(err)

	syncChan := make(chan struct{})
	var txs [][]byte
	for _, tx := range data.Txs {
		txs = append(txs, tx)
	}
	mockExec.On("ExecuteTxs", mock.Anything, txs, newHeight, header.Time(), initialState.AppHash, mock.Anything).
		Return(expectedNewAppHash, uint64(100), nil).Once()
	mockStore.On("SaveBlockData", mock.Anything, header, data, &header.Signature).Return(nil).Once()

	mockStore.On("UpdateState", mock.Anything, expectedNewState).Return(nil).Run(func(args mock.Arguments) { close(syncChan) }).Once()

	mockStore.On("SetHeight", mock.Anything, newHeight).Return(nil).Once()

	ctx, loopCancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer loopCancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.SyncLoop(ctx, make(chan<- error))
	}()

	t.Logf("Sending header event for height %d", newHeight)
	headerInCh <- NewHeaderEvent{Header: header, DAHeight: daHeight}

	time.Sleep(10 * time.Millisecond)

	t.Logf("Sending data event for height %d", newHeight)
	dataInCh <- NewDataEvent{Data: data, DAHeight: daHeight}

	t.Log("Waiting for sync to complete...")
	wg.Wait()

	select {
	case <-syncChan:
		t.Log("Sync completed.")
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for sync to complete")
	}

	mockStore.AssertExpectations(t)
	mockExec.AssertExpectations(t)

	finalState := m.GetLastState()
	assert.Equal(expectedNewState.LastBlockHeight, finalState.LastBlockHeight)
	assert.Equal(expectedNewState.AppHash, finalState.AppHash)
	assert.Equal(expectedNewState.LastBlockTime, finalState.LastBlockTime)
	assert.Equal(expectedNewState.DAHeight, finalState.DAHeight)

	// Assert caches are cleared for the processed height
	assert.Nil(m.headerCache.GetItem(newHeight), "Header cache should be cleared for processed height")
	assert.Nil(m.dataCache.GetItem(newHeight), "Data cache should be cleared for processed height")
}

// TestSyncLoop_ProcessSingleBlock_DataFirst verifies that the sync loop processes a single block when the data arrives before the header.
// 1. Data for H+1 arrives.
// 2. Header for H+1 arrives.
// 3. Block H+1 is successfully validated, applied, and committed.
// 4. State is updated.
// 5. Caches are cleared.
func TestSyncLoop_ProcessSingleBlock_DataFirst(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	initialHeight := uint64(20)
	initialState := types.State{
		LastBlockHeight: initialHeight,
		AppHash:         []byte("initial_app_hash_data_first"),
		ChainID:         "syncLoopTest",
		DAHeight:        15,
	}
	newHeight := initialHeight + 1
	daHeight := initialState.DAHeight

	m, mockStore, mockExec, ctx, cancel, headerInCh, dataInCh, _ := setupManagerForSyncLoopTest(t, initialState)
	defer cancel()

	// Create test block data
	header, data, privKey := types.GenerateRandomBlockCustomWithAppHash(&types.BlockConfig{Height: newHeight, NTxs: 3}, initialState.ChainID, initialState.AppHash)
	require.NotNil(header)
	require.NotNil(data)
	require.NotNil(privKey)

	expectedNewAppHash := []byte("new_app_hash_data_first")
	expectedNewState, err := initialState.NextState(header, expectedNewAppHash)
	require.NoError(err)

	syncChan := make(chan struct{})
	var txs [][]byte
	for _, tx := range data.Txs {
		txs = append(txs, tx)
	}

	mockExec.On("ExecuteTxs", mock.Anything, txs, newHeight, header.Time(), initialState.AppHash, mock.Anything).
		Return(expectedNewAppHash, uint64(100), nil).Once()
	mockStore.On("SaveBlockData", mock.Anything, header, data, &header.Signature).Return(nil).Once()
	mockStore.On("UpdateState", mock.Anything, expectedNewState).Return(nil).Run(func(args mock.Arguments) { close(syncChan) }).Once()
	mockStore.On("SetHeight", mock.Anything, newHeight).Return(nil).Once()

	ctx, loopCancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer loopCancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.SyncLoop(ctx, make(chan<- error))
	}()

	t.Logf("Sending data event for height %d", newHeight)
	dataInCh <- NewDataEvent{Data: data, DAHeight: daHeight}
	time.Sleep(10 * time.Millisecond)
	t.Logf("Sending header event for height %d", newHeight)
	headerInCh <- NewHeaderEvent{Header: header, DAHeight: daHeight}

	t.Log("Waiting for sync to complete...")

	wg.Wait()

	select {
	case <-syncChan:
		t.Log("Sync completed.")
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for sync to complete")
	}

	mockStore.AssertExpectations(t)
	mockExec.AssertExpectations(t)

	finalState := m.GetLastState()
	assert.Equal(expectedNewState.LastBlockHeight, finalState.LastBlockHeight)
	assert.Equal(expectedNewState.AppHash, finalState.AppHash)
	assert.Equal(expectedNewState.LastBlockTime, finalState.LastBlockTime)
	assert.Equal(expectedNewState.DAHeight, finalState.DAHeight)

	// Assert caches are cleared for the processed height
	assert.Nil(m.headerCache.GetItem(newHeight), "Header cache should be cleared for processed height")
	assert.Nil(m.dataCache.GetItem(newHeight), "Data cache should be cleared for processed height")
}

// TestSyncLoop_ProcessMultipleBlocks_Sequentially verifies that the sync loop processes multiple blocks arriving in order.
// 1. Events for H+1 arrive (header then data).
// 2. Block H+1 is processed.
// 3. Events for H+2 arrive (header then data).
// 4. Block H+2 is processed.
// 5. Final state is H+2.
func TestSyncLoop_ProcessMultipleBlocks_Sequentially(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	initialHeight := uint64(30)
	initialState := types.State{
		LastBlockHeight: initialHeight,
		AppHash:         []byte("initial_app_hash_multi"),
		ChainID:         "syncLoopTest",
		DAHeight:        25,
	}
	heightH1 := initialHeight + 1
	heightH2 := initialHeight + 2
	daHeight := initialState.DAHeight

	m, mockStore, mockExec, ctx, cancel, headerInCh, dataInCh, heightPtr := setupManagerForSyncLoopTest(t, initialState)
	defer cancel()

	// --- Block H+1 Data ---
	headerH1, dataH1, privKeyH1 := types.GenerateRandomBlockCustomWithAppHash(&types.BlockConfig{Height: heightH1, NTxs: 1}, initialState.ChainID, initialState.AppHash)
	require.NotNil(headerH1)
	require.NotNil(dataH1)
	require.NotNil(privKeyH1)

	expectedNewAppHashH1 := []byte("app_hash_h1")
	expectedNewStateH1, err := initialState.NextState(headerH1, expectedNewAppHashH1)
	require.NoError(err)

	var txsH1 [][]byte
	for _, tx := range dataH1.Txs {
		txsH1 = append(txsH1, tx)
	}

	// --- Block H+2 Data ---
	headerH2, dataH2, privKeyH2 := types.GenerateRandomBlockCustomWithAppHash(&types.BlockConfig{Height: heightH2, NTxs: 2}, initialState.ChainID, expectedNewAppHashH1)
	require.NotNil(headerH2)
	require.NotNil(dataH2)
	require.NotNil(privKeyH2)

	expectedNewAppHashH2 := []byte("app_hash_h2")
	expectedNewStateH2, err := expectedNewStateH1.NextState(headerH2, expectedNewAppHashH2)
	require.NoError(err)

	var txsH2 [][]byte
	for _, tx := range dataH2.Txs {
		txsH2 = append(txsH2, tx)
	}

	syncChanH1 := make(chan struct{})
	syncChanH2 := make(chan struct{})

	// --- Mock Expectations for H+1 ---

	mockExec.On("ExecuteTxs", mock.Anything, txsH1, heightH1, headerH1.Time(), initialState.AppHash, mock.Anything).
		Return(expectedNewAppHashH1, uint64(100), nil).Once()
	mockStore.On("SaveBlockData", mock.Anything, headerH1, dataH1, &headerH1.Signature).Return(nil).Once()
	mockStore.On("UpdateState", mock.Anything, expectedNewStateH1).Return(nil).Run(func(args mock.Arguments) { close(syncChanH1) }).Once()
	mockStore.On("SetHeight", mock.Anything, heightH1).Return(nil).
		Run(func(args mock.Arguments) {
			newHeight := args.Get(1).(uint64)
			*heightPtr = newHeight // Update the mocked height
			t.Logf("Mock SetHeight called for H+1, updated mock height to %d", newHeight)
		}).
		Once()

	// --- Mock Expectations for H+2 ---
	mockExec.On("ExecuteTxs", mock.Anything, txsH2, heightH2, headerH2.Time(), expectedNewAppHashH1, mock.Anything).
		Return(expectedNewAppHashH2, uint64(100), nil).Once()
	mockStore.On("SaveBlockData", mock.Anything, headerH2, dataH2, &headerH2.Signature).Return(nil).Once()
	mockStore.On("UpdateState", mock.Anything, expectedNewStateH2).Return(nil).Run(func(args mock.Arguments) { close(syncChanH2) }).Once()
	mockStore.On("SetHeight", mock.Anything, heightH2).Return(nil).
		Run(func(args mock.Arguments) {
			newHeight := args.Get(1).(uint64)
			*heightPtr = newHeight // Update the mocked height
			t.Logf("Mock SetHeight called for H+2, updated mock height to %d", newHeight)
		}).
		Once()

	ctx, loopCancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer loopCancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.SyncLoop(ctx, make(chan<- error))
		t.Log("SyncLoop exited.")
	}()

	// --- Process H+1 ---
	headerInCh <- NewHeaderEvent{Header: headerH1, DAHeight: daHeight}
	time.Sleep(10 * time.Millisecond)
	dataInCh <- NewDataEvent{Data: dataH1, DAHeight: daHeight}

	t.Log("Waiting for Sync H+1 to complete...")

	select {
	case <-syncChanH1:
		t.Log("Sync H+1 completed.")
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for sync H+1 to complete")
	}

	// --- Process H+2 ---
	headerInCh <- NewHeaderEvent{Header: headerH2, DAHeight: daHeight}
	time.Sleep(10 * time.Millisecond)
	dataInCh <- NewDataEvent{Data: dataH2, DAHeight: daHeight}

	select {
	case <-syncChanH2:
		t.Log("Sync H+2 completed.")
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for sync H+2 to complete")
	}

	t.Log("Waiting for SyncLoop H+2 to complete...")

	wg.Wait()

	mockStore.AssertExpectations(t)
	mockExec.AssertExpectations(t)

	finalState := m.GetLastState()
	assert.Equal(expectedNewStateH2.LastBlockHeight, finalState.LastBlockHeight)
	assert.Equal(expectedNewStateH2.AppHash, finalState.AppHash)
	assert.Equal(expectedNewStateH2.LastBlockTime, finalState.LastBlockTime)
	assert.Equal(expectedNewStateH2.DAHeight, finalState.DAHeight)

	assert.Nil(m.headerCache.GetItem(heightH1), "Header cache should be cleared for H+1")
	assert.Nil(m.dataCache.GetItem(heightH1), "Data cache should be cleared for H+1")
	assert.Nil(m.headerCache.GetItem(heightH2), "Header cache should be cleared for H+2")
	assert.Nil(m.dataCache.GetItem(heightH2), "Data cache should be cleared for H+2")
}

// TestSyncLoop_ProcessBlocks_OutOfOrderArrival verifies that the sync loop can handle blocks arriving out of order.
// 1. Events for H+2 arrive (header then data). Block H+2 is cached.
// 2. Events for H+1 arrive (header then data).
// 3. Block H+1 is processed.
// 4. Block H+2 is processed immediately after H+1 from the cache.
// 5. Final state is H+2.
func TestSyncLoop_ProcessBlocks_OutOfOrderArrival(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	initialHeight := uint64(40)
	initialState := types.State{
		LastBlockHeight: initialHeight,
		AppHash:         []byte("initial_app_hash_ooo"),
		ChainID:         "syncLoopTest",
		DAHeight:        35,
	}
	heightH1 := initialHeight + 1
	heightH2 := initialHeight + 2
	daHeight := initialState.DAHeight

	m, mockStore, mockExec, ctx, cancel, headerInCh, dataInCh, heightPtr := setupManagerForSyncLoopTest(t, initialState)
	defer cancel()

	// --- Block H+1 Data ---
	headerH1, dataH1, privKeyH1 := types.GenerateRandomBlockCustomWithAppHash(&types.BlockConfig{Height: heightH1, NTxs: 1}, initialState.ChainID, initialState.AppHash)
	require.NotNil(headerH1)
	require.NotNil(dataH1)
	require.NotNil(privKeyH1)

	appHashH1 := []byte("app_hash_h1_ooo")
	expectedNewStateH1, err := initialState.NextState(headerH1, appHashH1)
	require.NoError(err)

	var txsH1 [][]byte
	for _, tx := range dataH1.Txs {
		txsH1 = append(txsH1, tx)
	}

	// --- Block H+2 Data ---
	headerH2, dataH2, privKeyH2 := types.GenerateRandomBlockCustomWithAppHash(&types.BlockConfig{Height: heightH2, NTxs: 2}, initialState.ChainID, appHashH1)
	require.NotNil(headerH2)
	require.NotNil(dataH2)
	require.NotNil(privKeyH2)

	appHashH2 := []byte("app_hash_h2_ooo")
	expectedStateH2, err := expectedNewStateH1.NextState(headerH2, appHashH2)
	require.NoError(err)

	var txsH2 [][]byte
	for _, tx := range dataH2.Txs {
		txsH2 = append(txsH2, tx)
	}

	syncChanH1 := make(chan struct{})
	syncChanH2 := make(chan struct{})

	// --- Mock Expectations for H+1 (will be called first despite arrival order) ---
	mockStore.On("Height", mock.Anything).Return(initialHeight, nil).Maybe()
	mockExec.On("Validate", mock.Anything, &headerH1.Header, dataH1).Return(nil).Maybe()
	mockExec.On("ExecuteTxs", mock.Anything, txsH1, heightH1, headerH1.Time(), initialState.AppHash, mock.Anything).
		Return(appHashH1, uint64(100), nil).Once()
	mockStore.On("SaveBlockData", mock.Anything, headerH1, dataH1, &headerH1.Signature).Return(nil).Once()
	mockStore.On("UpdateState", mock.Anything, expectedNewStateH1).Return(nil).
		Run(func(args mock.Arguments) { close(syncChanH1) }).
		Once()
	mockStore.On("SetHeight", mock.Anything, heightH1).Return(nil).
		Run(func(args mock.Arguments) {
			newHeight := args.Get(1).(uint64)
			*heightPtr = newHeight // Update the mocked height
			t.Logf("Mock SetHeight called for H+2, updated mock height to %d", newHeight)
		}).
		Once()

	// --- Mock Expectations for H+2 (will be called second) ---
	mockStore.On("Height", mock.Anything).Return(heightH1, nil).Maybe()
	mockExec.On("Validate", mock.Anything, &headerH2.Header, dataH2).Return(nil).Maybe()
	mockExec.On("ExecuteTxs", mock.Anything, txsH2, heightH2, headerH2.Time(), expectedNewStateH1.AppHash, mock.Anything).
		Return(appHashH2, uint64(1), nil).Once()
	mockStore.On("SaveBlockData", mock.Anything, headerH2, dataH2, &headerH2.Signature).Return(nil).Once()
	mockStore.On("SetHeight", mock.Anything, heightH2).Return(nil).
		Run(func(args mock.Arguments) {
			newHeight := args.Get(1).(uint64)
			*heightPtr = newHeight // Update the mocked height
			t.Logf("Mock SetHeight called for H+2, updated mock height to %d", newHeight)
		}).
		Once()
	mockStore.On("UpdateState", mock.Anything, expectedStateH2).Return(nil).
		Run(func(args mock.Arguments) { close(syncChanH2) }).
		Once()

	ctx, loopCancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer loopCancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.SyncLoop(ctx, make(chan<- error))
		t.Log("SyncLoop exited.")
	}()

	// --- Send H+2 Events First ---
	headerInCh <- NewHeaderEvent{Header: headerH2, DAHeight: daHeight}
	time.Sleep(10 * time.Millisecond)
	dataInCh <- NewDataEvent{Data: dataH2, DAHeight: daHeight}

	time.Sleep(50 * time.Millisecond)
	assert.Equal(initialHeight, m.GetLastState().LastBlockHeight, "Height should not have advanced yet")
	assert.NotNil(m.headerCache.GetItem(heightH2), "Header H+2 should be in cache")
	assert.NotNil(m.dataCache.GetItem(heightH2), "Data H+2 should be in cache")

	// --- Send H+1 Events Second ---
	headerInCh <- NewHeaderEvent{Header: headerH1, DAHeight: daHeight}
	time.Sleep(10 * time.Millisecond)
	dataInCh <- NewDataEvent{Data: dataH1, DAHeight: daHeight}

	t.Log("Waiting for Sync H+1 to complete...")

	// --- Wait for Processing (H+1 then H+2) ---
	select {
	case <-syncChanH1:
		t.Log("Sync H+1 completed.")
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for sync H+1 to complete")
	}

	t.Log("Waiting for SyncLoop H+2 to complete...")

	select {
	case <-syncChanH2:
		t.Log("Sync H+2 completed.")
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for sync H+2 to complete")
	}

	wg.Wait()

	mockStore.AssertExpectations(t)
	mockExec.AssertExpectations(t)

	finalState := m.GetLastState()
	assert.Equal(expectedStateH2.LastBlockHeight, finalState.LastBlockHeight)
	assert.Equal(expectedStateH2.AppHash, finalState.AppHash)
	assert.Equal(expectedStateH2.LastBlockTime, finalState.LastBlockTime)
	assert.Equal(expectedStateH2.DAHeight, finalState.DAHeight)

	assert.Nil(m.headerCache.GetItem(heightH1), "Header cache should be cleared for H+1")
	assert.Nil(m.dataCache.GetItem(heightH1), "Data cache should be cleared for H+1")
	assert.Nil(m.headerCache.GetItem(heightH2), "Header cache should be cleared for H+2")
	assert.Nil(m.dataCache.GetItem(heightH2), "Data cache should be cleared for H+2")
}

// TestSyncLoop_IgnoreDuplicateEvents verifies that the SyncLoop correctly processes
// a block once even if the header and data events are received multiple times.
func TestSyncLoop_IgnoreDuplicateEvents(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	initialHeight := uint64(40)
	initialState := types.State{
		LastBlockHeight: initialHeight,
		AppHash:         []byte("initial_app_hash_dup"),
		ChainID:         "syncLoopTest",
		DAHeight:        35,
	}
	heightH1 := initialHeight + 1
	daHeight := initialState.DAHeight

	m, mockStore, mockExec, ctx, cancel, headerInCh, dataInCh, _ := setupManagerForSyncLoopTest(t, initialState)
	defer cancel()

	// --- Block H+1 Data ---
	headerH1, dataH1, privKeyH1 := types.GenerateRandomBlockCustomWithAppHash(&types.BlockConfig{Height: heightH1, NTxs: 1}, initialState.ChainID, initialState.AppHash)
	require.NotNil(headerH1)
	require.NotNil(dataH1)
	require.NotNil(privKeyH1)

	appHashH1 := []byte("app_hash_h1_dup")
	expectedStateH1, err := initialState.NextState(headerH1, appHashH1)
	require.NoError(err)

	var txsH1 [][]byte
	for _, tx := range dataH1.Txs {
		txsH1 = append(txsH1, tx)
	}

	syncChanH1 := make(chan struct{})

	// --- Mock Expectations (Expect processing exactly ONCE) ---
	mockExec.On("ExecuteTxs", mock.Anything, txsH1, heightH1, headerH1.Time(), initialState.AppHash, mock.Anything).
		Return(appHashH1, uint64(1), nil).Once()
	mockStore.On("SaveBlockData", mock.Anything, headerH1, dataH1, &headerH1.Signature).Return(nil).Once()
	mockStore.On("SetHeight", mock.Anything, heightH1).Return(nil).Once()
	mockStore.On("UpdateState", mock.Anything, expectedStateH1).Return(nil).
		Run(func(args mock.Arguments) { close(syncChanH1) }).
		Once()

	ctx, loopCancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer loopCancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.SyncLoop(ctx, make(chan<- error))
		t.Log("SyncLoop exited.")
	}()

	// --- Send First Set of Events ---
	headerInCh <- NewHeaderEvent{Header: headerH1, DAHeight: daHeight}
	time.Sleep(10 * time.Millisecond)
	dataInCh <- NewDataEvent{Data: dataH1, DAHeight: daHeight}

	t.Log("Waiting for first sync to complete...")
	select {
	case <-syncChanH1:
		t.Log("First sync completed.")
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for first sync to complete")
	}

	// --- Send Duplicate Events ---
	headerInCh <- NewHeaderEvent{Header: headerH1, DAHeight: daHeight}
	time.Sleep(10 * time.Millisecond)
	dataInCh <- NewDataEvent{Data: dataH1, DAHeight: daHeight}

	// Wait a bit to ensure duplicates are not processed
	time.Sleep(100 * time.Millisecond)

	wg.Wait()

	// Assertions
	mockStore.AssertExpectations(t) // Crucial: verifies calls happened exactly once
	mockExec.AssertExpectations(t)  // Crucial: verifies calls happened exactly once

	finalState := m.GetLastState()
	assert.Equal(expectedStateH1.LastBlockHeight, finalState.LastBlockHeight)
	assert.Equal(expectedStateH1.AppHash, finalState.AppHash)
	assert.Equal(expectedStateH1.LastBlockTime, finalState.LastBlockTime)
	assert.Equal(expectedStateH1.DAHeight, finalState.DAHeight)

	// Assert caches are cleared
	assert.Nil(m.headerCache.GetItem(heightH1), "Header cache should be cleared for H+1")
	assert.Nil(m.dataCache.GetItem(heightH1), "Data cache should be cleared for H+1")
}

// TestSyncLoop_ErrorOnApplyError verifies that the SyncLoop halts if ApplyBlock fails.
// Halting after sync loop error is handled in full.go and is not tested here.
func TestSyncLoop_ErrorOnApplyError(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	initialHeight := uint64(50)
	initialState := types.State{
		LastBlockHeight: initialHeight,
		AppHash:         []byte("initial_app_hash_panic"),
		ChainID:         "syncLoopTest",
		DAHeight:        45,
	}
	heightH1 := initialHeight + 1
	daHeight := initialState.DAHeight

	m, mockStore, mockExec, ctx, cancel, headerInCh, dataInCh, _ := setupManagerForSyncLoopTest(t, initialState)
	defer cancel() // Ensure context cancellation happens even on panic

	// --- Block H+1 Data ---
	headerH1, dataH1, privKeyH1 := types.GenerateRandomBlockCustomWithAppHash(&types.BlockConfig{Height: heightH1, NTxs: 1}, initialState.ChainID, initialState.AppHash)
	require.NotNil(headerH1)
	require.NotNil(dataH1)
	require.NotNil(privKeyH1)

	var txsH1 [][]byte
	for _, tx := range dataH1.Txs {
		txsH1 = append(txsH1, tx)
	}

	applyErrorSignal := make(chan struct{})
	applyError := errors.New("apply failed")

	// --- Mock Expectations ---
	mockExec.On("ExecuteTxs", mock.Anything, txsH1, heightH1, headerH1.Time(), initialState.AppHash, mock.Anything).
		Return(nil, uint64(100), applyError).                       // Return the error that should cause panic
		Run(func(args mock.Arguments) { close(applyErrorSignal) }). // Signal *before* returning error
		Once()
	// NO further calls expected after Apply error

	ctx, loopCancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer loopCancel()

	var wg sync.WaitGroup
	wg.Add(1)

	errCh := make(chan error, 1)

	go func() {
		defer wg.Done()
		m.SyncLoop(ctx, errCh)
	}()

	// --- Send Events ---
	t.Logf("Sending header event for height %d", heightH1)
	headerInCh <- NewHeaderEvent{Header: headerH1, DAHeight: daHeight}
	time.Sleep(10 * time.Millisecond)
	t.Logf("Sending data event for height %d", heightH1)
	dataInCh <- NewDataEvent{Data: dataH1, DAHeight: daHeight}

	t.Log("Waiting for ApplyBlock error...")
	select {
	case <-applyErrorSignal:
		t.Log("ApplyBlock error occurred.")
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for ApplyBlock error signal")
	}

	require.Error(<-errCh, "SyncLoop should return an error when ApplyBlock errors")

	wg.Wait()

	mockStore.AssertExpectations(t)
	mockExec.AssertExpectations(t)

	finalState := m.GetLastState()
	assert.Equal(initialState.LastBlockHeight, finalState.LastBlockHeight, "State height should not change after panic")
	assert.Equal(initialState.AppHash, finalState.AppHash, "State AppHash should not change after panic")

	assert.NotNil(m.headerCache.GetItem(heightH1), "Header cache should still contain item for H+1")
	assert.NotNil(m.dataCache.GetItem(heightH1), "Data cache should still contain item for H+1")
}

func TestHandleEmptyDataHash(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	// Mock store and data cache
	store := mocks.NewStore(t)
	dataCache := cache.NewCache[types.Data]()

	// Setup the manager with the mock and data cache
	m := &Manager{
		store:     store,
		dataCache: dataCache,
	}

	// Define the test data
	headerHeight := 2
	header := &types.Header{
		DataHash: dataHashForEmptyTxs,
		BaseHeader: types.BaseHeader{
			Height: 2,
			Time:   uint64(time.Now().UnixNano()),
		},
	}

	// Mock data for the previous block
	lastData := &types.Data{}
	lastDataHash := lastData.Hash()

	// header.DataHash equals dataHashForEmptyTxs and no error occurs
	store.On("GetBlockData", ctx, uint64(headerHeight-1)).Return(nil, lastData, nil)

	// Execute the method under test
	m.handleEmptyDataHash(ctx, header)

	// Assertions
	store.AssertExpectations(t)

	// make sure that the store has the correct data
	d := dataCache.GetItem(header.Height())
	require.NotNil(d)
	require.Equal(d.LastDataHash, lastDataHash)
	require.Equal(d.Metadata.ChainID, header.ChainID())
	require.Equal(d.Metadata.Height, header.Height())
	require.Equal(d.Metadata.Time, header.BaseHeader.Time)
}

// TestDataCommitmentToHeight_HeaderThenData verifies that when a header arrives before its data, the data is set for the correct height and the mapping is removed after processing.
func TestDataCommitmentToHeight_HeaderThenData(t *testing.T) {
	assert := assert.New(t)
	initialHeight := uint64(100)
	initialState := types.State{
		LastBlockHeight: initialHeight,
		AppHash:         []byte("app_hash"),
		ChainID:         "syncLoopTest",
		DAHeight:        50,
	}
	newHeight := initialHeight + 1
	m, mockStore, mockExec, ctx, cancel, headerInCh, dataInCh, _ := setupManagerForSyncLoopTest(t, initialState)
	defer cancel()

	header, data, _ := types.GenerateRandomBlockCustomWithAppHash(&types.BlockConfig{Height: newHeight, NTxs: 1}, initialState.ChainID, initialState.AppHash)
	dataHash := data.DACommitment().String()
	var txs [][]byte
	for _, tx := range data.Txs {
		txs = append(txs, tx)
	}
	mockExec.On("ExecuteTxs", mock.Anything, txs, newHeight, header.Time(), initialState.AppHash, mock.Anything).
		Return([]byte("new_app_hash"), uint64(100), nil).Once()
	mockStore.On("SaveBlockData", mock.Anything, header, data, &header.Signature).Return(nil).Once()
	mockStore.On("UpdateState", mock.Anything, mock.Anything).Return(nil).Once()
	mockStore.On("SetHeight", mock.Anything, newHeight).Return(nil).Once()

	ctx, loopCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer loopCancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.SyncLoop(ctx, make(chan<- error))
	}()

	headerInCh <- NewHeaderEvent{Header: header, DAHeight: initialState.DAHeight}
	time.Sleep(10 * time.Millisecond)
	dataInCh <- NewDataEvent{Data: data, DAHeight: initialState.DAHeight}
	wg.Wait()

	// Data should be processed and cache cleared
	assert.Nil(m.dataCache.GetItem(newHeight), "Data cache should be cleared after processing")
	_, ok := m.dataCommitmentToHeight.Load(dataHash)
	assert.False(ok, "Mapping should be removed after data is set")
}

// TestDataCommitmentToHeight_HeaderWithExistingData verifies that if data is already present when the header arrives, the data is immediately set for the header height and no mapping is created.
func TestDataCommitmentToHeight_HeaderWithExistingData(t *testing.T) {
	assert := assert.New(t)
	initialHeight := uint64(200)
	initialState := types.State{
		LastBlockHeight: initialHeight,
		AppHash:         []byte("app_hash2"),
		ChainID:         "syncLoopTest",
		DAHeight:        150,
	}
	newHeight := initialHeight + 1
	m, mockStore, mockExec, ctx, cancel, headerInCh, _, _ := setupManagerForSyncLoopTest(t, initialState)
	defer cancel()

	header, data, _ := types.GenerateRandomBlockCustomWithAppHash(&types.BlockConfig{Height: newHeight, NTxs: 1}, initialState.ChainID, initialState.AppHash)
	dataHash := data.DACommitment().String()
	m.dataCache.SetItemByHash(dataHash, data)
	var txs [][]byte
	for _, tx := range data.Txs {
		txs = append(txs, tx)
	}
	mockExec.On("ExecuteTxs", mock.Anything, txs, newHeight, header.Time(), initialState.AppHash, mock.Anything).
		Return([]byte("new_app_hash2"), uint64(100), nil).Once()
	mockStore.On("SaveBlockData", mock.Anything, header, data, &header.Signature).Return(nil).Once()
	mockStore.On("UpdateState", mock.Anything, mock.Anything).Return(nil).Once()
	mockStore.On("SetHeight", mock.Anything, newHeight).Return(nil).Once()

	ctx, loopCancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer loopCancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.SyncLoop(ctx, make(chan<- error))
	}()

	headerInCh <- NewHeaderEvent{Header: header, DAHeight: initialState.DAHeight}
	wg.Wait()

	// Data should be processed and cache cleared
	assert.Nil(m.dataCache.GetItem(newHeight), "Data cache should be cleared after processing")
	_, ok := m.dataCommitmentToHeight.Load(dataHash)
	assert.False(ok, "No mapping should be created if data is already present")
}

// TestDataCommitmentToHeight_DataBeforeHeader verifies that if data arrives before the header, it is not set for any height until the header and a subsequent data event arrive, after which the data is processed and the mapping is removed.
func TestDataCommitmentToHeight_DataBeforeHeader(t *testing.T) {
	assert := assert.New(t)
	initialHeight := uint64(300)
	initialState := types.State{
		LastBlockHeight: initialHeight,
		AppHash:         []byte("app_hash3"),
		ChainID:         "syncLoopTest",
		DAHeight:        250,
	}
	newHeight := initialHeight + 1
	m, mockStore, mockExec, ctx, cancel, headerInCh, dataInCh, _ := setupManagerForSyncLoopTest(t, initialState)
	defer cancel()

	header, data, _ := types.GenerateRandomBlockCustomWithAppHash(&types.BlockConfig{Height: newHeight, NTxs: 1}, initialState.ChainID, initialState.AppHash)
	var txs [][]byte
	for _, tx := range data.Txs {
		txs = append(txs, tx)
	}
	mockExec.On("ExecuteTxs", mock.Anything, txs, newHeight, header.Time(), initialState.AppHash, mock.Anything).
		Return([]byte("new_app_hash3"), uint64(100), nil).Once()
	mockStore.On("SaveBlockData", mock.Anything, header, data, &header.Signature).Return(nil).Once()
	mockStore.On("UpdateState", mock.Anything, mock.Anything).Return(nil).Once()
	mockStore.On("SetHeight", mock.Anything, newHeight).Return(nil).Once()

	ctx, loopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer loopCancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.SyncLoop(ctx, make(chan<- error))
	}()

	data.Metadata = nil

	// Data arrives before header
	dataInCh <- NewDataEvent{Data: data, DAHeight: initialState.DAHeight}
	time.Sleep(100 * time.Millisecond)
	// Data should not be set for any height yet, but should be present by hash
	assert.Nil(m.dataCache.GetItem(newHeight))
	assert.NotNil(m.dataCache.GetItemByHash(data.DACommitment().String()), "Data should be present in cache by hash after first data event")
	// Now header arrives
	headerInCh <- NewHeaderEvent{Header: header, DAHeight: initialState.DAHeight}
	// Data should be processed and cache cleared after header event
	time.Sleep(100 * time.Millisecond)
	assert.Nil(m.dataCache.GetItem(newHeight))
	assert.Nil(m.dataCache.GetItemByHash(data.DACommitment().String()), "Data cache by hash should be cleared after header event")
	// Data event arrives again
	dataInCh <- NewDataEvent{Data: data, DAHeight: initialState.DAHeight}
	wg.Wait()
	// Data should still be cleared
	assert.Nil(m.dataCache.GetItem(newHeight), "Data cache should be cleared after processing")
	assert.Nil(m.dataCache.GetItemByHash(data.DACommitment().String()), "Data cache by hash should be cleared after processing")
	_, ok := m.dataCommitmentToHeight.Load(data.DACommitment().String())
	assert.False(ok, "Mapping should be removed after data is set")
}

// TestDataCommitmentToHeight_HeaderAlreadySynced verifies that if a header arrives for an already-synced height, the data is not set and the mapping is cleaned up.
func TestDataCommitmentToHeight_HeaderAlreadySynced(t *testing.T) {
	assert := assert.New(t)
	initialHeight := uint64(400)
	initialState := types.State{
		LastBlockHeight: initialHeight,
		AppHash:         []byte("app_hash4"),
		ChainID:         "syncLoopTest",
		DAHeight:        350,
	}
	m, _, _, _, cancel, headerInCh, dataInCh, _ := setupManagerForSyncLoopTest(t, initialState)
	defer cancel()

	header, data, _ := types.GenerateRandomBlockCustomWithAppHash(&types.BlockConfig{Height: initialHeight, NTxs: 1}, initialState.ChainID, initialState.AppHash)
	dataHash := data.DACommitment().String()

	ctx, loopCancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer loopCancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.SyncLoop(ctx, make(chan<- error))
	}()

	headerInCh <- NewHeaderEvent{Header: header, DAHeight: initialState.DAHeight}
	time.Sleep(10 * time.Millisecond)
	dataInCh <- NewDataEvent{Data: data, DAHeight: initialState.DAHeight}
	wg.Wait()

	// Data should not be set for already synced height
	assert.Nil(m.dataCache.GetItem(initialHeight))
	// Mapping should be cleaned up
	_, ok := m.dataCommitmentToHeight.Load(dataHash)
	assert.False(ok, "Mapping should be cleaned up for already synced height")
}

// TestDataCommitmentToHeight_EmptyDataHash verifies that no mapping is created for dataHashForEmptyTxs and the empty block logic is handled correctly.
func TestDataCommitmentToHeight_EmptyDataHash(t *testing.T) {
	assert := assert.New(t)
	initialHeight := uint64(500)
	initialState := types.State{
		LastBlockHeight: initialHeight,
		AppHash:         []byte("app_hash5"),
		ChainID:         "syncLoopTest",
		DAHeight:        450,
	}
	newHeight := initialHeight + 1
	m, mockStore, mockExec, _, cancel, headerInCh, _, _ := setupManagerForSyncLoopTest(t, initialState)
	defer cancel()

	header, _, _ := types.GenerateRandomBlockCustomWithAppHash(&types.BlockConfig{Height: newHeight, NTxs: 0}, initialState.ChainID, initialState.AppHash)
	header.DataHash = dataHashForEmptyTxs
	// Mock GetBlockData for previous height
	mockStore.On("GetBlockData", mock.Anything, newHeight-1).Return(nil, &types.Data{}, nil).Once()
	// Mock ExecuteTxs, SaveBlockData, UpdateState, SetHeight for empty block
	mockExec.On("ExecuteTxs", mock.Anything, [][]byte{}, newHeight, header.Time(), initialState.AppHash, mock.Anything).
		Return([]byte("new_app_hash5"), uint64(100), nil).Once()
	mockStore.On("SaveBlockData", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
	mockStore.On("UpdateState", mock.Anything, mock.Anything).Return(nil).Once()
	mockStore.On("SetHeight", mock.Anything, newHeight).Return(nil).Once()

	ctx, loopCancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer loopCancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.SyncLoop(ctx, make(chan<- error))
	}()

	headerInCh <- NewHeaderEvent{Header: header, DAHeight: initialState.DAHeight}
	wg.Wait()

	// No mapping should be created for dataHashForEmptyTxs
	_, ok := m.dataCommitmentToHeight.Load(string(dataHashForEmptyTxs))
	assert.False(ok, "No mapping should be created for dataHashForEmptyTxs")
}

// TestDataCommitmentToHeight_DataAlreadySeen verifies that if data is already marked as seen, it is not set again for any height.
func TestDataCommitmentToHeight_DataAlreadySeen(t *testing.T) {
	assert := assert.New(t)
	initialHeight := uint64(600)
	initialState := types.State{
		LastBlockHeight: initialHeight,
		AppHash:         []byte("app_hash6"),
		ChainID:         "syncLoopTest",
		DAHeight:        550,
	}
	newHeight := initialHeight + 1
	m, _, _, _, cancel, _, dataInCh, _ := setupManagerForSyncLoopTest(t, initialState)
	defer cancel()

	_, data, _ := types.GenerateRandomBlockCustomWithAppHash(&types.BlockConfig{Height: newHeight, NTxs: 1}, initialState.ChainID, initialState.AppHash)
	dataHash := data.DACommitment().String()
	m.dataCache.SetSeen(dataHash)

	ctx, loopCancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer loopCancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.SyncLoop(ctx, make(chan<- error))
	}()

	dataInCh <- NewDataEvent{Data: data, DAHeight: initialState.DAHeight}
	wg.Wait()

	// Data should not be set again if already seen
	assert.Nil(m.dataCache.GetItem(newHeight))
}

// --- END dataCommitmentToHeight TESTS ---
