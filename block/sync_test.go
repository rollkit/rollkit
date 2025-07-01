package block

import (
	"context"
	"errors"
	"fmt"
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
	"github.com/rollkit/rollkit/pkg/store"
	"github.com/rollkit/rollkit/test/mocks"
	"github.com/rollkit/rollkit/types"
)

// setupManagerForSyncLoopTest initializes a Manager instance suitable for SyncLoop testing.
func setupManagerForSyncLoopTest(t *testing.T, initialState types.State) (
	*Manager,
	*mocks.MockStore,
	*mocks.MockExecutor,
	context.Context,
	context.CancelFunc,
	chan NewHeaderEvent,
	chan NewDataEvent,
	*uint64,
) {
	t.Helper()

	mockStore := mocks.NewMockStore(t)
	mockExec := mocks.NewMockExecutor(t)

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
		store:         mockStore,
		exec:          mockExec,
		config:        cfg,
		genesis:       *genesisDoc,
		lastState:     initialState,
		lastStateMtx:  new(sync.RWMutex),
		logger:        log.NewTestLogger(t),
		headerCache:   cache.NewCache[types.SignedHeader](),
		dataCache:     cache.NewCache[types.Data](),
		headerInCh:    headerInCh,
		dataInCh:      dataInCh,
		headerStoreCh: headerStoreCh,
		dataStoreCh:   dataStoreCh,
		retrieveCh:    retrieveCh,
		daHeight:      &atomic.Uint64{},
		metrics:       NopMetrics(),
		headerStore:   &goheaderstore.Store[*types.SignedHeader]{},
		dataStore:     &goheaderstore.Store[*types.Data]{},
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
	mockExec.On("ExecuteTxs", mock.Anything, txs, newHeight, header.Time(), initialState.AppHash).
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

	mockExec.On("ExecuteTxs", mock.Anything, txs, newHeight, header.Time(), initialState.AppHash).
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

	mockExec.On("ExecuteTxs", mock.Anything, txsH1, heightH1, headerH1.Time(), initialState.AppHash).
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
	mockExec.On("ExecuteTxs", mock.Anything, txsH2, heightH2, headerH2.Time(), expectedNewAppHashH1).
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
	mockExec.On("ExecuteTxs", mock.Anything, txsH1, heightH1, headerH1.Time(), initialState.AppHash).
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
	mockExec.On("ExecuteTxs", mock.Anything, txsH2, heightH2, headerH2.Time(), expectedNewStateH1.AppHash).
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
	dataInCh <- NewDataEvent{Data: dataH2, DAHeight: daHeight}

	// Wait for H+2 to be cached (but not processed since H+1 is missing)
	require.Eventually(func() bool {
		return m.headerCache.GetItem(heightH2) != nil && m.dataCache.GetItem(heightH2) != nil
	}, 1*time.Second, 10*time.Millisecond, "H+2 header and data should be cached")

	assert.Equal(initialHeight, m.GetLastState().LastBlockHeight, "Height should not have advanced yet")
	assert.NotNil(m.headerCache.GetItem(heightH2), "Header H+2 should be in cache")
	assert.NotNil(m.dataCache.GetItem(heightH2), "Data H+2 should be in cache")

	// --- Send H+1 Events Second ---
	headerInCh <- NewHeaderEvent{Header: headerH1, DAHeight: daHeight}
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
	mockExec.On("ExecuteTxs", mock.Anything, txsH1, heightH1, headerH1.Time(), initialState.AppHash).
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
	dataInCh <- NewDataEvent{Data: dataH1, DAHeight: daHeight}

	// Give the sync loop a chance to process duplicates (if it would)
	// Since we expect no processing, we just wait for the context timeout
	// The mock expectations will fail if duplicates are processed

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
	mockExec.On("ExecuteTxs", mock.Anything, txsH1, heightH1, headerH1.Time(), initialState.AppHash).
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

// TestHandleEmptyDataHash tests that handleEmptyDataHash correctly handles the case where a header has an empty data hash, ensuring the data cache is updated with the previous block's data hash and metadata.
func TestHandleEmptyDataHash(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	// Mock store and data cache
	store := mocks.NewMockStore(t)
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

// TestSyncLoop_MultipleHeadersArriveFirst_ThenData verifies that the sync loop correctly processes multiple blocks when all headers arrive first (without data), then their corresponding data arrives (or is handled as empty).
// 1. Headers for H+1 through H+5 arrive (no data yet).
// 2. State should not advance after only headers are received.
// 3. For non-empty blocks, data for H+1, H+2, H+4, and H+5 arrives in order and each block is processed as soon as both header and data are available.
// 4. For empty blocks (H+3 and H+4), no data is sent; the sync loop handles these using the empty data hash logic.
// 5. Final state is H+5, caches are cleared for all processed heights, and mocks are called as expected.
func TestSyncLoop_MultipleHeadersArriveFirst_ThenData(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	initialHeight := uint64(100)
	initialState := types.State{
		LastBlockHeight: initialHeight,
		AppHash:         []byte("initial_app_hash_multi_headers"),
		ChainID:         "syncLoopTest",
		DAHeight:        95,
	}
	numBlocks := 5
	blockHeights := make([]uint64, numBlocks)
	for i := 0; i < numBlocks; i++ {
		blockHeights[i] = initialHeight + uint64(i+1)
	}
	daHeight := initialState.DAHeight

	// Use a real in-memory store
	kv, err := store.NewDefaultInMemoryKVStore()
	require.NoError(err)
	store := store.New(kv)

	mockExec := mocks.NewMockExecutor(t)

	headerInCh := make(chan NewHeaderEvent, 5)
	dataInCh := make(chan NewDataEvent, 5)

	headers := make([]*types.SignedHeader, numBlocks)
	data := make([]*types.Data, numBlocks)
	privKeys := make([]any, numBlocks)
	expectedAppHashes := make([][]byte, numBlocks)
	expectedStates := make([]types.State, numBlocks)
	txs := make([][][]byte, numBlocks)
	syncChans := make([]chan struct{}, numBlocks)

	prevAppHash := initialState.AppHash
	prevState := initialState

	// Save initial state in the store
	require.NoError(store.UpdateState(context.Background(), initialState))
	require.NoError(store.SetHeight(context.Background(), initialHeight))

	for i := 0; i < numBlocks; i++ {
		// Make blocks 2 and 3 (H+3 and H+4) empty
		var nTxs int
		if i == 2 || i == 3 {
			nTxs = 0
		} else {
			nTxs = i + 1
		}
		headers[i], data[i], privKeys[i] = types.GenerateRandomBlockCustomWithAppHash(
			&types.BlockConfig{Height: blockHeights[i], NTxs: nTxs},
			prevState.ChainID, prevAppHash,
		)
		require.NotNil(headers[i])
		require.NotNil(data[i])

		txs[i] = make([][]byte, 0, len(data[i].Txs))
		for _, tx := range data[i].Txs {
			txs[i] = append(txs[i], tx)
		}

		expectedAppHashes[i] = []byte(fmt.Sprintf("app_hash_h%d", i+1))
		var err error
		expectedStates[i], err = prevState.NextState(headers[i], expectedAppHashes[i])
		require.NoError(err)

		syncChans[i] = make(chan struct{})

		// Set up mocks for each block
		mockExec.On("ExecuteTxs", mock.Anything, txs[i], blockHeights[i], headers[i].Time(), prevAppHash).
			Return(expectedAppHashes[i], uint64(100), nil).Run(func(args mock.Arguments) {
			close(syncChans[i])
		}).Once()

		prevAppHash = expectedAppHashes[i]
		prevState = expectedStates[i]
	}

	m := &Manager{
		store:        store,
		exec:         mockExec,
		config:       config.DefaultConfig,
		genesis:      genesis.Genesis{ChainID: initialState.ChainID},
		lastState:    initialState,
		lastStateMtx: new(sync.RWMutex),
		logger:       log.NewTestLogger(t),
		headerCache:  cache.NewCache[types.SignedHeader](),
		dataCache:    cache.NewCache[types.Data](),
		headerInCh:   headerInCh,
		dataInCh:     dataInCh,
		daHeight:     &atomic.Uint64{},
		metrics:      NopMetrics(),
	}
	m.daHeight.Store(initialState.DAHeight)

	ctx, loopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer loopCancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.SyncLoop(ctx, make(chan<- error))
	}()

	// 1. Send all headers first (no data yet)
	for i := 0; i < numBlocks; i++ {
		headerInCh <- NewHeaderEvent{Header: headers[i], DAHeight: daHeight}
	}

	// 2. Wait for headers to be processed and cached by polling the cache state
	// This replaces the flaky time.Sleep(50 * time.Millisecond)
	require.Eventually(func() bool {
		// Check that all headers are cached
		for i := 0; i < numBlocks; i++ {
			if m.headerCache.GetItem(blockHeights[i]) == nil {
				return false
			}
		}
		// Check that empty blocks have data cached, non-empty blocks don't yet
		for i := 0; i < numBlocks; i++ {
			dataInCache := m.dataCache.GetItem(blockHeights[i]) != nil
			if i == 2 || i == 3 {
				// Empty blocks should have data in cache
				if !dataInCache {
					return false
				}
			} else {
				// Non-empty blocks should not have data in cache yet
				if dataInCache {
					return false
				}
			}
		}
		return true
	}, 1*time.Second, 10*time.Millisecond, "Headers should be cached and empty blocks should have data cached")

	assert.Equal(initialHeight, m.GetLastState().LastBlockHeight, "Height should not have advanced yet after only headers")
	for i := 0; i < numBlocks; i++ {
		if i == 2 || i == 3 {
			assert.NotNil(m.dataCache.GetItem(blockHeights[i]), "Data should be in cache for empty block H+%d", i+1)
		} else {
			assert.Nil(m.dataCache.GetItem(blockHeights[i]), "Data should not be in cache for H+%d yet", i+1)
		}
		assert.NotNil(m.headerCache.GetItem(blockHeights[i]), "Header should be in cache for H+%d", i+1)
	}

	// 3. Send data for each block in order (skip empty blocks)
	for i := 0; i < numBlocks; i++ {
		if i == 2 || i == 3 {
			// Do NOT send data for empty blocks
			continue
		}
		dataInCh <- NewDataEvent{Data: data[i], DAHeight: daHeight}
		// Wait for block to be processed
		select {
		case <-syncChans[i]:
			// processed
		case <-time.After(2 * time.Second):
			t.Fatalf("Timeout waiting for sync of H+%d", i+1)
		}
	}

	wg.Wait()

	mockExec.AssertExpectations(t)

	finalState := m.GetLastState()
	assert.Equal(expectedStates[numBlocks-1].LastBlockHeight, finalState.LastBlockHeight)
	assert.Equal(expectedStates[numBlocks-1].AppHash, finalState.AppHash)
	assert.Equal(expectedStates[numBlocks-1].LastBlockTime, finalState.LastBlockTime)
	assert.Equal(expectedStates[numBlocks-1].DAHeight, finalState.DAHeight)

	// Assert caches are cleared for all processed heights
	for i := 0; i < numBlocks; i++ {
		assert.Nil(m.headerCache.GetItem(blockHeights[i]), "Header cache should be cleared for H+%d", i+1)
		assert.Nil(m.dataCache.GetItem(blockHeights[i]), "Data cache should be cleared for H+%d", i+1)
	}
}
