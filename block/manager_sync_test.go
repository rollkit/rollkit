package block

import (
	"bytes"
	"context"
	"sync"
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
		store:          mockStore,
		exec:           mockExec,
		config:         cfg,
		genesis:        *genesisDoc,
		lastState:      initialState,
		lastStateMtx:   new(sync.RWMutex),
		logger:         log.NewTestLogger(t),
		headerCache:    cache.NewCache[types.SignedHeader](),
		dataCache:      cache.NewCache[types.Data](),
		headerInCh:     headerInCh,
		dataInCh:       dataInCh,
		headerStoreCh:  headerStoreCh,
		dataStoreCh:    dataStoreCh,
		retrieveCh:     retrieveCh,
		daHeight:       uint64(0),
		metrics:        NopMetrics(),
		headerStore:    &goheaderstore.Store[*types.SignedHeader]{},
		dataStore:      &goheaderstore.Store[*types.Data]{},
		pendingHeaders: &PendingHeaders{logger: log.NewNopLogger()},
	}
	m.daHeight = initialState.DAHeight

	_, cancel := context.WithCancel(context.Background())

	currentMockHeight := initialState.LastBlockHeight
	heightPtr := &currentMockHeight

	mockStore.On("Height", mock.Anything).Return(func(context.Context) uint64 {
		return *heightPtr
	}, nil).Maybe()

	return m, mockStore, mockExec, cancel, headerInCh, dataInCh, heightPtr
}

// TestSyncLoop_ProcessSingleBlock_HeaderFirst verifies the basic scenario:
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
	daHeight := initialState.DAHeight + 1

	m, mockStore, mockExec, cancel, headerInCh, dataInCh, _ := setupManagerForSyncLoopTest(t, initialState)
	defer cancel()

	// Create test block data
	header, data, privKey := types.GenerateRandomBlockCustom(&types.BlockConfig{Height: newHeight, NTxs: 2}, initialState.ChainID)
	require.NotNil(header)
	require.NotNil(data)
	require.NotNil(privKey)

	expectedNewAppHash := []byte("new_app_hash")
	expectedNewState := initialState
	expectedNewState.LastBlockHeight = newHeight
	expectedNewState.AppHash = expectedNewAppHash
	expectedNewState.LastBlockTime = header.Time()
	expectedNewState.DAHeight = daHeight

	syncChan := make(chan struct{})
	var txs [][]byte
	for _, tx := range data.Txs {
		txs = append(txs, tx)
	}
	mockExec.On("ExecuteTxs", mock.Anything, txs, newHeight, header.Time(), initialState.AppHash).
		Return(expectedNewAppHash, uint64(100), nil).Once()
	mockStore.On("SaveBlockData", mock.Anything, header, data, &header.Signature).Return(nil).Once()
	mockExec.On("SetFinal", mock.Anything, newHeight).Return(nil).Once()
	mockStore.On("SetHeight", mock.Anything, newHeight).Return(nil).Once()

	mockStore.On("UpdateState", mock.Anything, expectedNewState).Return(nil).
		Run(func(args mock.Arguments) { close(syncChan) }).
		Once()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.SyncLoop(context.Background())
	}()

	t.Logf("Sending header event for height %d", newHeight)
	headerInCh <- NewHeaderEvent{Header: header, DAHeight: daHeight}

	time.Sleep(10 * time.Millisecond)

	t.Logf("Sending data event for height %d", newHeight)
	dataInCh <- NewDataEvent{Data: data, DAHeight: daHeight}

	t.Log("Waiting for sync to complete...")
	select {
	case <-syncChan:
		t.Log("Sync completed.")
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for sync to complete")
	}

	cancel()

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

// TestSyncLoop_ProcessSingleBlock_DataFirst verifies the basic scenario but with data arriving before the header:
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
	daHeight := initialState.DAHeight + 1

	m, mockStore, mockExec, cancel, headerInCh, dataInCh, _ := setupManagerForSyncLoopTest(t, initialState)
	defer cancel()

	// Create test block data
	header, data, privKey := types.GenerateRandomBlockCustom(&types.BlockConfig{Height: newHeight, NTxs: 3}, initialState.ChainID)
	require.NotNil(header)
	require.NotNil(data)
	require.NotNil(privKey)

	expectedNewAppHash := []byte("new_app_hash_data_first")
	expectedNewState := initialState
	expectedNewState.LastBlockHeight = newHeight
	expectedNewState.AppHash = expectedNewAppHash
	expectedNewState.LastBlockTime = header.Time()
	expectedNewState.DAHeight = daHeight

	syncChan := make(chan struct{})
	var txs [][]byte
	for _, tx := range data.Txs {
		txs = append(txs, tx)
	}

	mockExec.On("ExecuteTxs", mock.Anything, txs, newHeight, header.Time(), initialState.AppHash).
		Return(expectedNewAppHash, uint64(1), nil).Once()
	mockStore.On("SaveBlockData", mock.Anything, header, data, &header.Signature).Return(nil).Once()
	mockExec.On("SetFinal", mock.Anything, newHeight).Return(nil).Once()
	mockStore.On("SetHeight", mock.Anything, newHeight).Return(nil).Once()
	mockStore.On("UpdateState", mock.Anything, expectedNewState).Return(nil).
		Run(func(args mock.Arguments) { close(syncChan) }).
		Once()

	ctx, testCancel := context.WithCancel(context.Background())
	defer testCancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.SyncLoop(ctx)
		t.Log("SyncLoop exited.")
	}()

	t.Logf("Sending data event for height %d", newHeight)
	dataInCh <- NewDataEvent{Data: data, DAHeight: daHeight}

	time.Sleep(50 * time.Millisecond)

	t.Logf("Sending header event for height %d", newHeight)
	headerInCh <- NewHeaderEvent{Header: header, DAHeight: daHeight}

	t.Log("Waiting for sync to complete...")
	select {
	case <-syncChan:
		t.Log("Sync completed.")
	case <-time.After(2 * time.Second):
		testCancel()
		wg.Wait() // Wait for SyncLoop goroutine to finish
		t.Fatal("Timeout waiting for sync to complete")
	}

	testCancel()
	wg.Wait()

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

// TestSyncLoop_ProcessMultipleBlocks_Sequentially verifies processing multiple blocks arriving sequentially.
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
	daHeight := initialState.DAHeight + 1

	m, mockStore, mockExec, cancel, headerInCh, dataInCh, heightPtr := setupManagerForSyncLoopTest(t, initialState)
	defer cancel()

	// --- Block H+1 Data ---
	headerH1, dataH1, privKeyH1 := types.GenerateRandomBlockCustom(&types.BlockConfig{Height: heightH1, NTxs: 1}, initialState.ChainID)
	require.NotNil(headerH1)
	require.NotNil(dataH1)
	require.NotNil(privKeyH1)
	appHashH1 := []byte("app_hash_h1")
	stateH1 := initialState
	stateH1.LastBlockHeight = heightH1
	stateH1.AppHash = appHashH1
	stateH1.LastBlockTime = headerH1.Time()
	stateH1.DAHeight = daHeight
	var txsH1 [][]byte
	for _, tx := range dataH1.Txs {
		txsH1 = append(txsH1, tx)
	}

	// --- Block H+2 Data ---
	headerH2, dataH2, privKeyH2 := types.GenerateRandomBlockCustom(&types.BlockConfig{Height: heightH2, NTxs: 2}, initialState.ChainID)
	require.NotNil(headerH2)
	require.NotNil(dataH2)
	require.NotNil(privKeyH2)
	appHashH2 := []byte("app_hash_h2")
	stateH2 := stateH1
	stateH2.LastBlockHeight = heightH2
	stateH2.AppHash = appHashH2
	stateH2.LastBlockTime = headerH2.Time()
	stateH2.DAHeight = daHeight
	var txsH2 [][]byte
	for _, tx := range dataH2.Txs {
		txsH2 = append(txsH2, tx)
	}

	syncChanH1 := make(chan struct{})
	syncChanH2 := make(chan struct{})

	// --- Mock Expectations for H+1 ---
	mockExec.On("ExecuteTxs", mock.Anything, txsH1, heightH1, headerH1.Time(), initialState.AppHash).
		Return(appHashH1, uint64(1), nil).Once()
	mockStore.On("SaveBlockData", mock.Anything, headerH1, dataH1, &headerH1.Signature).Return(nil).Once()
	mockExec.On("SetFinal", mock.Anything, heightH1).Return(nil).Once()

	mockStore.On("SetHeight", mock.Anything, heightH1).Return(nil).
		Run(func(args mock.Arguments) {
			newHeight := args.Get(1).(uint64)
			*heightPtr = newHeight // Update the mocked height
			t.Logf("Mock SetHeight called for H+1, updated mock height to %d", newHeight)
		}).
		Once()
	mockStore.On("UpdateState", mock.Anything, mock.MatchedBy(func(s types.State) bool {
		return s.LastBlockHeight == heightH1 && bytes.Equal(s.AppHash, appHashH1)
	})).Return(nil).
		Run(func(args mock.Arguments) { close(syncChanH1) }).
		Once()

	// --- Mock Expectations for H+2 ---
	mockExec.On("ExecuteTxs", mock.Anything, txsH2, heightH2, headerH2.Time(), mock.Anything).
		Return(appHashH2, uint64(1), nil).Once()
	mockStore.On("SaveBlockData", mock.Anything, headerH2, dataH2, &headerH2.Signature).Return(nil).Once()
	mockExec.On("SetFinal", mock.Anything, heightH2).Return(nil).Once()

	mockStore.On("SetHeight", mock.Anything, heightH2).Return(nil).
		Run(func(args mock.Arguments) {
			newHeight := args.Get(1).(uint64)
			*heightPtr = newHeight // Update the mocked height
			t.Logf("Mock SetHeight called for H+2, updated mock height to %d", newHeight)
		}).
		Once()
	mockStore.On("UpdateState", mock.Anything, mock.MatchedBy(func(s types.State) bool {
		// Note: Check against expected H+2 values
		return s.LastBlockHeight == heightH2 && bytes.Equal(s.AppHash, appHashH2)
	})).Return(nil).
		Run(func(args mock.Arguments) { close(syncChanH2) }).
		Once()

	ctx, testCancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.SyncLoop(ctx)
		t.Log("SyncLoop exited.")
	}()

	// --- Process H+1 ---
	headerInCh <- NewHeaderEvent{Header: headerH1, DAHeight: daHeight}
	time.Sleep(10 * time.Millisecond)
	dataInCh <- NewDataEvent{Data: dataH1, DAHeight: daHeight}

	select {
	case <-syncChanH1:
		t.Log("Sync H+1 completed.")
	case <-time.After(2 * time.Second):
		testCancel()
		wg.Wait()
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
		testCancel()
		wg.Wait()
		t.Fatal("Timeout waiting for sync H+2 to complete")
	}

	testCancel()
	wg.Wait()

	mockStore.AssertExpectations(t)
	mockExec.AssertExpectations(t)

	finalState := m.GetLastState()
	assert.Equal(stateH2.LastBlockHeight, finalState.LastBlockHeight)
	assert.Equal(stateH2.AppHash, finalState.AppHash)
	assert.Equal(stateH2.LastBlockTime, finalState.LastBlockTime)
	assert.Equal(stateH2.DAHeight, finalState.DAHeight)

	assert.Nil(m.headerCache.GetItem(heightH1), "Header cache should be cleared for H+1")
	assert.Nil(m.dataCache.GetItem(heightH1), "Data cache should be cleared for H+1")
	assert.Nil(m.headerCache.GetItem(heightH2), "Header cache should be cleared for H+2")
	assert.Nil(m.dataCache.GetItem(heightH2), "Data cache should be cleared for H+2")
}

// TestSyncLoop_ProcessBlocks_OutOfOrderArrival verifies processing blocks arriving out of order.
// 1. Events for H+2 arrive (header then data). Block H+2 is cached.
// 2. Events for H+1 arrive (header then data).
// 3. Block H+1 is processed.
// 4. Block H+2 is processed immediately after H+1 from the cache.
// 5. Final state is H+2.
func TestSyncLoop_ProcessBlocks_OutOfOrderArrival(t *testing.T) {
	t.Skip()
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
	daHeight := initialState.DAHeight + 1

	m, mockStore, mockExec, cancel, headerInCh, dataInCh, heightPtr := setupManagerForSyncLoopTest(t, initialState)
	defer cancel()

	// --- Block H+1 Data ---
	headerH1, dataH1, privKeyH1 := types.GenerateRandomBlockCustom(&types.BlockConfig{Height: heightH1, NTxs: 1}, initialState.ChainID)
	require.NotNil(headerH1)
	require.NotNil(dataH1)
	require.NotNil(privKeyH1)
	appHashH1 := []byte("app_hash_h1_ooo")
	stateH1 := initialState
	stateH1.LastBlockHeight = heightH1
	stateH1.AppHash = appHashH1
	stateH1.LastBlockTime = headerH1.Time()
	stateH1.DAHeight = daHeight
	var txsH1 [][]byte
	for _, tx := range dataH1.Txs {
		txsH1 = append(txsH1, tx)
	}

	// --- Block H+2 Data ---
	headerH2, dataH2, privKeyH2 := types.GenerateRandomBlockCustom(&types.BlockConfig{Height: heightH2, NTxs: 2}, initialState.ChainID)
	require.NotNil(headerH2)
	require.NotNil(dataH2)
	require.NotNil(privKeyH2)
	appHashH2 := []byte("app_hash_h2_ooo")
	stateH2 := stateH1
	stateH2.LastBlockHeight = heightH2
	stateH2.AppHash = appHashH2
	stateH2.LastBlockTime = headerH2.Time()
	stateH2.DAHeight = daHeight
	var txsH2 [][]byte
	for _, tx := range dataH2.Txs {
		txsH2 = append(txsH2, tx)
	}

	syncChanH1 := make(chan struct{})
	syncChanH2 := make(chan struct{})

	// --- Mock Expectations for H+1 (will be called first despite arrival order) ---
	mockStore.On("Height", mock.Anything).Return(initialHeight, nil).Maybe()
	mockExec.On("Validate", mock.Anything, &headerH1.Header, dataH1).Return(nil).Maybe()
	mockExec.On("ExecuteTxs", mock.Anything, txsH1, heightH1, headerH1.Time(), initialState.AppHash).
		Return(appHashH1, uint64(1), nil).Once()
	mockStore.On("SaveBlockData", mock.Anything, headerH1, dataH1, &headerH1.Signature).Return(nil).Once()
	mockExec.On("SetFinal", mock.Anything, heightH1).Return(nil).Once()
	mockStore.On("SetHeight", mock.Anything, heightH1).Return(nil).
		Run(func(args mock.Arguments) {
			newHeight := args.Get(1).(uint64)
			*heightPtr = newHeight // Update the mocked height
			t.Logf("Mock SetHeight called for H+2, updated mock height to %d", newHeight)
		}).
		Once()
	mockStore.On("UpdateState", mock.Anything, stateH1).Return(nil).
		Run(func(args mock.Arguments) { close(syncChanH1) }).
		Once()

	// --- Mock Expectations for H+2 (will be called second) ---
	mockStore.On("Height", mock.Anything).Return(heightH1, nil).Maybe()
	mockExec.On("Validate", mock.Anything, &headerH2.Header, dataH2).Return(nil).Maybe()
	mockExec.On("ExecuteTxs", mock.Anything, txsH2, heightH2, headerH2.Time(), stateH1.AppHash).
		Return(appHashH2, uint64(1), nil).Once()
	mockStore.On("SaveBlockData", mock.Anything, headerH2, dataH2, &headerH2.Signature).Return(nil).Once()
	mockExec.On("SetFinal", mock.Anything, heightH2).Return(nil).Once()
	mockStore.On("SetHeight", mock.Anything, heightH2).Return(nil).
		Run(func(args mock.Arguments) {
			newHeight := args.Get(1).(uint64)
			*heightPtr = newHeight // Update the mocked height
			t.Logf("Mock SetHeight called for H+2, updated mock height to %d", newHeight)
		}).
		Once()
	mockStore.On("UpdateState", mock.Anything, stateH2).Return(nil).
		Run(func(args mock.Arguments) { close(syncChanH2) }).
		Once()

	ctx, testCancel := context.WithCancel(context.Background())
	defer testCancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.SyncLoop(ctx)
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

	// --- Wait for Processing (H+1 then H+2) ---
	select {
	case <-syncChanH1:
		t.Log("Sync H+1 completed.")
	case <-time.After(2 * time.Second):
		testCancel()
		wg.Wait()
		t.Fatal("Timeout waiting for sync H+1 to complete")
	}

	select {
	case <-syncChanH2:
		t.Log("Sync H+2 completed.")
	case <-time.After(2 * time.Second):
		testCancel()
		wg.Wait()
		t.Fatal("Timeout waiting for sync H+2 to complete")
	}

	testCancel()
	wg.Wait()

	mockStore.AssertExpectations(t)
	mockExec.AssertExpectations(t)

	finalState := m.GetLastState()
	assert.Equal(stateH2.LastBlockHeight, finalState.LastBlockHeight)
	assert.Equal(stateH2.AppHash, finalState.AppHash)
	assert.Equal(stateH2.LastBlockTime, finalState.LastBlockTime)
	assert.Equal(stateH2.DAHeight, finalState.DAHeight)

	assert.Nil(m.headerCache.GetItem(heightH1), "Header cache should be cleared for H+1")
	assert.Nil(m.dataCache.GetItem(heightH1), "Data cache should be cleared for H+1")
	assert.Nil(m.headerCache.GetItem(heightH2), "Header cache should be cleared for H+2")
	assert.Nil(m.dataCache.GetItem(heightH2), "Data cache should be cleared for H+2")
}
