package block

import (
	"context"
	"encoding/binary"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/rollkit/rollkit/pkg/cache"
	"github.com/rollkit/rollkit/test/mocks"
	"github.com/rollkit/rollkit/types"
)

// newTestManager creates a Manager with mocked Store and Executor for testing DAIncluder logic.
func newTestManager(t *testing.T) (*Manager, *mocks.Store, *mocks.Executor, *MockLogger) {
	store := mocks.NewStore(t)
	exec := mocks.NewExecutor(t)
	logger := new(MockLogger)
	logger.On("Debug", mock.Anything, mock.Anything).Maybe()
	logger.On("Info", mock.Anything, mock.Anything).Maybe()
	logger.On("Warn", mock.Anything, mock.Anything).Maybe()
	logger.On("Error", mock.Anything, mock.Anything).Maybe()
	// Mock Height to always return a high value so IsDAIncluded works
	store.On("Height", mock.Anything).Return(uint64(100), nil).Maybe()

	// Create default hasher functions for testing
	validatorHasher := func(proposerAddress []byte, pubKey crypto.PubKey) (types.Hash, error) {
		return make(types.Hash, 32), nil
	}
	commitHashProvider := func(signature *types.Signature, header *types.Header, proposerAddress []byte) (types.Hash, error) {
		return make(types.Hash, 32), nil
	}
	signaturePayloadProvider := func(header *types.Header, data *types.Data) ([]byte, error) {
		return header.MarshalBinary()
	}

	m := &Manager{
		store:                    store,
		headerCache:              cache.NewCache[types.SignedHeader](),
		dataCache:                cache.NewCache[types.Data](),
		daIncluderCh:             make(chan struct{}, 1),
		logger:                   logger,
		exec:                     exec,
		lastStateMtx:             &sync.RWMutex{},
		metrics:                  NopMetrics(),
		validatorHasher:          validatorHasher,
		commitHashProvider:       commitHashProvider,
		signaturePayloadProvider: signaturePayloadProvider,
	}
	return m, store, exec, logger
}

// TestDAIncluderLoop_AdvancesHeightWhenBothDAIncluded verifies that the DAIncluderLoop advances the DA included height
// when both the header and data for the next block are marked as DA-included in the caches.
func TestDAIncluderLoop_AdvancesHeightWhenBothDAIncluded(t *testing.T) {
	t.Parallel()
	m, store, exec, _ := newTestManager(t)
	startDAIncludedHeight := uint64(4)
	expectedDAIncludedHeight := startDAIncludedHeight + 1
	m.daIncludedHeight.Store(startDAIncludedHeight)

	header, data := types.GetRandomBlock(5, 1, "testchain")
	headerHash := header.Hash().String()
	dataHash := data.DACommitment().String()
	m.headerCache.SetDAIncluded(headerHash)
	m.dataCache.SetDAIncluded(dataHash)

	store.On("GetBlockData", mock.Anything, uint64(5)).Return(header, data, nil).Once()
	store.On("GetBlockData", mock.Anything, uint64(6)).Return(nil, nil, assert.AnError).Once()
	heightBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(heightBytes, expectedDAIncludedHeight)
	store.On("SetMetadata", mock.Anything, DAIncludedHeightKey, heightBytes).Return(nil).Once()
	exec.On("SetFinal", mock.MatchedBy(func(ctx context.Context) bool { return true }), uint64(5)).Return(nil).Once()

	ctx, loopCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer loopCancel()

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		m.DAIncluderLoop(ctx, make(chan<- error))
	}()

	m.sendNonBlockingSignalToDAIncluderCh()

	wg.Wait()

	assert.Equal(t, expectedDAIncludedHeight, m.daIncludedHeight.Load())
	store.AssertExpectations(t)
	exec.AssertExpectations(t)
}

// TestDAIncluderLoop_StopsWhenHeaderNotDAIncluded verifies that the DAIncluderLoop does not advance the height
// if the header for the next block is not marked as DA-included in the cache.
func TestDAIncluderLoop_StopsWhenHeaderNotDAIncluded(t *testing.T) {
	t.Parallel()
	m, store, _, _ := newTestManager(t)
	startDAIncludedHeight := uint64(4)
	m.daIncludedHeight.Store(startDAIncludedHeight)

	header, data := types.GetRandomBlock(5, 1, "testchain")
	// m.headerCache.SetDAIncluded(headerHash) // Not set
	m.dataCache.SetDAIncluded(data.DACommitment().String())

	store.On("GetBlockData", mock.Anything, uint64(5)).Return(header, data, nil).Once()

	ctx, loopCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer loopCancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.DAIncluderLoop(ctx, make(chan<- error))
	}()

	m.sendNonBlockingSignalToDAIncluderCh()
	wg.Wait()

	assert.Equal(t, startDAIncludedHeight, m.GetDAIncludedHeight())
	store.AssertExpectations(t)
}

// TestDAIncluderLoop_StopsWhenDataNotDAIncluded verifies that the DAIncluderLoop does not advance the height
// if the data for the next block is not marked as DA-included in the cache.
func TestDAIncluderLoop_StopsWhenDataNotDAIncluded(t *testing.T) {
	t.Parallel()
	m, store, _, _ := newTestManager(t)
	startDAIncludedHeight := uint64(4)
	m.daIncludedHeight.Store(startDAIncludedHeight)

	header, data := types.GetRandomBlock(5, 1, "testchain")
	headerHash := header.Hash().String()
	m.headerCache.SetDAIncluded(headerHash)
	// m.dataCache.SetDAIncluded(data.DACommitment().String()) // Not set

	store.On("GetBlockData", mock.Anything, uint64(5)).Return(header, data, nil).Once()

	ctx, loopCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer loopCancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.DAIncluderLoop(ctx, make(chan<- error))
	}()

	m.sendNonBlockingSignalToDAIncluderCh()
	wg.Wait()

	assert.Equal(t, startDAIncludedHeight, m.GetDAIncludedHeight())
	store.AssertExpectations(t)
}

// TestDAIncluderLoop_StopsOnGetBlockDataError verifies that the DAIncluderLoop stops advancing
// if GetBlockData returns an error for the next block height.
func TestDAIncluderLoop_StopsOnGetBlockDataError(t *testing.T) {
	t.Parallel()
	m, store, _, mockLogger := newTestManager(t)
	startDAIncludedHeight := uint64(4)
	m.daIncludedHeight.Store(startDAIncludedHeight)

	store.On("GetBlockData", mock.Anything, uint64(5)).Return(nil, nil, assert.AnError).Once()

	// Expect the debug log for no more blocks to check
	mockLogger.ExpectedCalls = nil // Clear any previous expectations
	mockLogger.On("Debug", "no more blocks to check at this time", mock.Anything).Once()

	ctx, loopCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer loopCancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.DAIncluderLoop(ctx, make(chan<- error))
	}()

	m.sendNonBlockingSignalToDAIncluderCh()
	wg.Wait()

	assert.Equal(t, startDAIncludedHeight, m.GetDAIncludedHeight())
	store.AssertExpectations(t)
	mockLogger.AssertExpectations(t)
}

// TestIncrementDAIncludedHeight_Success verifies that incrementDAIncludedHeight increments the height
// and calls SetMetadata and SetFinal when CompareAndSwap succeeds.
func TestIncrementDAIncludedHeight_Success(t *testing.T) {
	t.Parallel()
	m, store, exec, _ := newTestManager(t)
	startDAIncludedHeight := uint64(4)
	expectedDAIncludedHeight := startDAIncludedHeight + 1
	m.daIncludedHeight.Store(startDAIncludedHeight)

	heightBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(heightBytes, expectedDAIncludedHeight)
	store.On("SetMetadata", mock.Anything, DAIncludedHeightKey, heightBytes).Return(nil).Once()
	exec.On("SetFinal", mock.MatchedBy(func(ctx context.Context) bool { return true }), expectedDAIncludedHeight).Return(nil).Once()

	err := m.incrementDAIncludedHeight(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, expectedDAIncludedHeight, m.GetDAIncludedHeight())
	store.AssertExpectations(t)
	exec.AssertExpectations(t)
}

// TestIncrementDAIncludedHeight_SetMetadataError verifies that incrementDAIncludedHeight returns an error
// if SetMetadata fails after SetFinal succeeds.
func TestIncrementDAIncludedHeight_SetMetadataError(t *testing.T) {
	t.Parallel()
	m, store, exec, mockLogger := newTestManager(t)
	startDAIncludedHeight := uint64(4)
	expectedDAIncludedHeight := startDAIncludedHeight + 1
	m.daIncludedHeight.Store(startDAIncludedHeight)

	heightBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(heightBytes, expectedDAIncludedHeight)
	exec.On("SetFinal", mock.MatchedBy(func(ctx context.Context) bool { return true }), expectedDAIncludedHeight).Return(nil).Once()
	store.On("SetMetadata", mock.Anything, DAIncludedHeightKey, heightBytes).Return(assert.AnError).Once()

	// Expect the error log for failed to set DA included height
	mockLogger.ExpectedCalls = nil // Clear any previous expectations
	mockLogger.On("Error", "failed to set DA included height", []interface{}{"height", expectedDAIncludedHeight, "error", assert.AnError}).Once()
	mockLogger.On("Debug", mock.Anything, mock.Anything).Maybe() // Allow other debug logs

	err := m.incrementDAIncludedHeight(context.Background())
	assert.Error(t, err)
	store.AssertExpectations(t)
	exec.AssertExpectations(t)
	mockLogger.AssertExpectations(t)
}

// TestIncrementDAIncludedHeight_SetFinalError verifies that incrementDAIncludedHeight returns an error
// if SetFinal fails before SetMetadata, and logs the error.
func TestIncrementDAIncludedHeight_SetFinalError(t *testing.T) {
	t.Parallel()
	m, store, exec, mockLogger := newTestManager(t)
	startDAIncludedHeight := uint64(4)
	expectedDAIncludedHeight := startDAIncludedHeight + 1
	m.daIncludedHeight.Store(startDAIncludedHeight)

	heightBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(heightBytes, expectedDAIncludedHeight)

	setFinalErr := assert.AnError
	exec.On("SetFinal", mock.MatchedBy(func(ctx context.Context) bool { return true }), expectedDAIncludedHeight).Return(setFinalErr).Once()
	// SetMetadata should NOT be called if SetFinal fails

	mockLogger.ExpectedCalls = nil // Clear any previous expectations
	// Expect the error log for failed to set final
	mockLogger.On("Error", "failed to set final", mock.Anything).Once()
	mockLogger.On("Debug", mock.Anything, mock.Anything).Maybe() // Allow other debug logs
	mockLogger.On("Error", mock.Anything, mock.Anything).Maybe() // Allow other error logs

	err := m.incrementDAIncludedHeight(context.Background())
	assert.Error(t, err)
	exec.AssertExpectations(t)
	store.AssertExpectations(t)
	mockLogger.AssertExpectations(t)
}

// TestDAIncluderLoop_MultipleConsecutiveHeightsDAIncluded verifies that DAIncluderLoop advances the height
// multiple times in a single run when several consecutive blocks are DA-included.
func TestDAIncluderLoop_MultipleConsecutiveHeightsDAIncluded(t *testing.T) {
	t.Parallel()
	m, store, exec, _ := newTestManager(t)
	startDAIncludedHeight := uint64(4)
	numConsecutive := 10
	numTxs := 5
	m.daIncludedHeight.Store(startDAIncludedHeight)

	headers := make([]*types.SignedHeader, numConsecutive)
	dataBlocks := make([]*types.Data, numConsecutive)

	for i := 0; i < numConsecutive; i++ {
		height := startDAIncludedHeight + uint64(i+1)
		headers[i], dataBlocks[i] = types.GetRandomBlock(height, numTxs, "testchain")
		headerHash := headers[i].Hash().String()
		dataHash := dataBlocks[i].DACommitment().String()
		m.headerCache.SetDAIncluded(headerHash)
		m.dataCache.SetDAIncluded(dataHash)
		store.On("GetBlockData", mock.Anything, height).Return(headers[i], dataBlocks[i], nil).Once()
	}
	// Next height returns error
	store.On("GetBlockData", mock.Anything, startDAIncludedHeight+uint64(numConsecutive+1)).Return(nil, nil, assert.AnError).Once()
	store.On("SetMetadata", mock.Anything, DAIncludedHeightKey, mock.Anything).Return(nil).Times(numConsecutive)
	exec.On("SetFinal", mock.MatchedBy(func(ctx context.Context) bool { return true }), mock.Anything).Return(nil).Times(numConsecutive)

	expectedDAIncludedHeight := startDAIncludedHeight + uint64(numConsecutive)

	ctx, loopCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer loopCancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.DAIncluderLoop(ctx, make(chan<- error))
	}()

	m.sendNonBlockingSignalToDAIncluderCh()
	wg.Wait()

	assert.Equal(t, expectedDAIncludedHeight, m.GetDAIncludedHeight())
	store.AssertExpectations(t)
	exec.AssertExpectations(t)
}

// TestDAIncluderLoop_AdvancesHeightWhenDataHashIsEmptyAndHeaderDAIncluded verifies that the DAIncluderLoop advances the DA included height
// when the header is DA-included and the data hash is dataHashForEmptyTxs (empty txs), even if the data is not DA-included.
func TestDAIncluderLoop_AdvancesHeightWhenDataHashIsEmptyAndHeaderDAIncluded(t *testing.T) {
	t.Parallel()
	m, store, exec, _ := newTestManager(t)
	startDAIncludedHeight := uint64(4)
	expectedDAIncludedHeight := startDAIncludedHeight + 1
	m.daIncludedHeight.Store(startDAIncludedHeight)

	header, data := types.GetRandomBlock(5, 0, "testchain")
	headerHash := header.Hash().String()
	m.headerCache.SetDAIncluded(headerHash)
	// Do NOT set data as DA-included

	store.On("GetBlockData", mock.Anything, uint64(5)).Return(header, data, nil).Once()
	store.On("GetBlockData", mock.Anything, uint64(6)).Return(nil, nil, assert.AnError).Once()
	heightBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(heightBytes, expectedDAIncludedHeight)
	store.On("SetMetadata", mock.Anything, DAIncludedHeightKey, heightBytes).Return(nil).Once()
	exec.On("SetFinal", mock.MatchedBy(func(ctx context.Context) bool { return true }), uint64(5)).Return(nil).Once()

	ctx, loopCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer loopCancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.DAIncluderLoop(ctx, make(chan<- error))
	}()

	m.sendNonBlockingSignalToDAIncluderCh()
	wg.Wait()

	assert.Equal(t, expectedDAIncludedHeight, m.daIncludedHeight.Load())
	store.AssertExpectations(t)
	exec.AssertExpectations(t)
}

// TestDAIncluderLoop_DoesNotAdvanceWhenDataHashIsEmptyAndHeaderNotDAIncluded verifies that the DAIncluderLoop does not advance the DA included height
// when the header is NOT DA-included, even if the data hash is dataHashForEmptyTxs (empty txs).
func TestDAIncluderLoop_DoesNotAdvanceWhenDataHashIsEmptyAndHeaderNotDAIncluded(t *testing.T) {
	t.Parallel()
	m, store, _, _ := newTestManager(t)
	startDAIncludedHeight := uint64(4)
	m.daIncludedHeight.Store(startDAIncludedHeight)

	header, data := types.GetRandomBlock(5, 0, "testchain")
	// Do NOT set header as DA-included
	// Do NOT set data as DA-included

	store.On("GetBlockData", mock.Anything, uint64(5)).Return(header, data, nil).Once()

	ctx, loopCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer loopCancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.DAIncluderLoop(ctx, make(chan<- error))
	}()

	m.sendNonBlockingSignalToDAIncluderCh()
	wg.Wait()

	assert.Equal(t, startDAIncludedHeight, m.GetDAIncludedHeight())
	store.AssertExpectations(t)
}

// Note: It is not practical to unit test a CompareAndSwap failure for incrementDAIncludedHeight
// because the atomic value is always read at the start of the function, and there is no way to
// inject a failure or race another goroutine reliably in a unit test. To test this path, the code
// would need to be refactored to allow injection or mocking of the atomic value.
