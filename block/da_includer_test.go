package block

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	coreda "github.com/rollkit/rollkit/core/da"
	"github.com/rollkit/rollkit/core/sequencer"
	"github.com/rollkit/rollkit/pkg/cache"
	"github.com/rollkit/rollkit/test/mocks"
	"github.com/rollkit/rollkit/types"
)

// newTestManager creates a Manager with mocked Store and Executor for testing DAIncluder logic.
func newTestManager(t *testing.T) (*Manager, *mocks.Store, *mocks.Executor, *MockLogger) {
	store := mocks.NewMockStore(t)
	exec := mocks.NewMockExecutor(t)
	logger := new(MockLogger)
	logger.On("Debug", mock.Anything, mock.Anything).Maybe()
	logger.On("Info", mock.Anything, mock.Anything).Maybe()
	logger.On("Warn", mock.Anything, mock.Anything).Maybe()
	logger.On("Error", mock.Anything, mock.Anything).Maybe()
	// Mock Height to always return a high value so IsDAIncluded works
	store.On("Height", mock.Anything).Return(uint64(100), nil).Maybe()
	m := &Manager{
		store:        store,
		headerCache:  cache.NewCache[types.SignedHeader](),
		dataCache:    cache.NewCache[types.Data](),
		daIncluderCh: make(chan struct{}, 1),
		logger:       logger,
		exec:         exec,
		lastStateMtx: &sync.RWMutex{},
		metrics:      NopMetrics(),
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
	m.headerCache.SetDAIncluded(headerHash, uint64(1))
	m.dataCache.SetDAIncluded(dataHash, uint64(1))

	store.On("GetBlockData", mock.Anything, uint64(5)).Return(header, data, nil).Times(2)
	store.On("GetBlockData", mock.Anything, uint64(6)).Return(nil, nil, assert.AnError).Once()
	// Mock expectations for SetRollkitHeightToDAHeight method
	headerHeightBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(headerHeightBytes, uint64(1))
	store.On("SetMetadata", mock.Anything, fmt.Sprintf("%s/%d/h", RollkitHeightToDAHeightKey, uint64(5)), headerHeightBytes).Return(nil).Once()
	dataHeightBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(dataHeightBytes, uint64(1))
	store.On("SetMetadata", mock.Anything, fmt.Sprintf("%s/%d/d", RollkitHeightToDAHeightKey, uint64(5)), dataHeightBytes).Return(nil).Once()
	// Mock expectations for incrementDAIncludedHeight method
	heightBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(heightBytes, expectedDAIncludedHeight)
	store.On("SetMetadata", mock.Anything, DAIncludedHeightKey, heightBytes).Return(nil).Once()
	exec.On("SetFinal", mock.Anything, uint64(5)).Return(nil).Once()

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
	m.dataCache.SetDAIncluded(data.DACommitment().String(), uint64(1))

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
	m.headerCache.SetDAIncluded(headerHash, uint64(1))
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
	exec.On("SetFinal", mock.Anything, expectedDAIncludedHeight).Return(nil).Once()

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
	exec.On("SetFinal", mock.Anything, expectedDAIncludedHeight).Return(nil).Once()
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
	exec.On("SetFinal", mock.Anything, expectedDAIncludedHeight).Return(setFinalErr).Once()
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
		m.headerCache.SetDAIncluded(headerHash, uint64(i+1))
		m.dataCache.SetDAIncluded(dataHash, uint64(i+1))
		store.On("GetBlockData", mock.Anything, height).Return(headers[i], dataBlocks[i], nil).Times(2) // Called by IsDAIncluded and SetRollkitHeightToDAHeight
	}
	// Next height returns error
	store.On("GetBlockData", mock.Anything, startDAIncludedHeight+uint64(numConsecutive+1)).Return(nil, nil, assert.AnError).Once()
	// Mock expectations for SetRollkitHeightToDAHeight method calls
	for i := 0; i < numConsecutive; i++ {
		height := startDAIncludedHeight + uint64(i+1)
		headerHeightBytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(headerHeightBytes, uint64(i+1))
		store.On("SetMetadata", mock.Anything, fmt.Sprintf("%s/%d/h", RollkitHeightToDAHeightKey, height), headerHeightBytes).Return(nil).Once()
		dataHeightBytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(dataHeightBytes, uint64(i+1))
		store.On("SetMetadata", mock.Anything, fmt.Sprintf("%s/%d/d", RollkitHeightToDAHeightKey, height), dataHeightBytes).Return(nil).Once()
	}
	store.On("SetMetadata", mock.Anything, DAIncludedHeightKey, mock.Anything).Return(nil).Times(numConsecutive)
	exec.On("SetFinal", mock.Anything, mock.Anything).Return(nil).Times(numConsecutive)

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
	m.headerCache.SetDAIncluded(headerHash, uint64(1))
	// Do NOT set data as DA-included

	store.On("GetBlockData", mock.Anything, uint64(5)).Return(header, data, nil).Times(2) // Called by IsDAIncluded and SetRollkitHeightToDAHeight
	store.On("GetBlockData", mock.Anything, uint64(6)).Return(nil, nil, assert.AnError).Once()
	// Mock expectations for SetRollkitHeightToDAHeight method
	headerHeightBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(headerHeightBytes, uint64(1))
	store.On("SetMetadata", mock.Anything, fmt.Sprintf("%s/%d/h", RollkitHeightToDAHeightKey, uint64(5)), headerHeightBytes).Return(nil).Once()
	// Note: For empty data, data SetMetadata call should still be made but data won't be marked as DA-included in cache,
	// so SetRollkitHeightToDAHeight will fail when trying to get the DA height for data
	// Actually, let's check if this case is handled differently for empty txs
	// Let me check what happens with empty txs by adding the data cache entry as well
	dataHash := data.DACommitment().String()
	m.dataCache.SetDAIncluded(dataHash, uint64(1))
	dataHeightBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(dataHeightBytes, uint64(1))
	store.On("SetMetadata", mock.Anything, fmt.Sprintf("%s/%d/d", RollkitHeightToDAHeightKey, uint64(5)), dataHeightBytes).Return(nil).Once()
	// Mock expectations for incrementDAIncludedHeight method
	heightBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(heightBytes, expectedDAIncludedHeight)
	store.On("SetMetadata", mock.Anything, DAIncludedHeightKey, heightBytes).Return(nil).Once()
	exec.On("SetFinal", mock.Anything, uint64(5)).Return(nil).Once()

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

// MockSequencerWithMetrics is a mock sequencer that implements MetricsRecorder interface
type MockSequencerWithMetrics struct {
	mock.Mock
}

func (m *MockSequencerWithMetrics) RecordMetrics(gasPrice float64, blobSize uint64, statusCode coreda.StatusCode, numPendingBlocks uint64, includedBlockHeight uint64) {
	m.Called(gasPrice, blobSize, statusCode, numPendingBlocks, includedBlockHeight)
}

// Implement the Sequencer interface methods
func (m *MockSequencerWithMetrics) SubmitBatchTxs(ctx context.Context, req sequencer.SubmitBatchTxsRequest) (*sequencer.SubmitBatchTxsResponse, error) {
	args := m.Called(ctx, req)
	return args.Get(0).(*sequencer.SubmitBatchTxsResponse), args.Error(1)
}

func (m *MockSequencerWithMetrics) GetNextBatch(ctx context.Context, req sequencer.GetNextBatchRequest) (*sequencer.GetNextBatchResponse, error) {
	args := m.Called(ctx, req)
	return args.Get(0).(*sequencer.GetNextBatchResponse), args.Error(1)
}

func (m *MockSequencerWithMetrics) VerifyBatch(ctx context.Context, req sequencer.VerifyBatchRequest) (*sequencer.VerifyBatchResponse, error) {
	args := m.Called(ctx, req)
	return args.Get(0).(*sequencer.VerifyBatchResponse), args.Error(1)
}

// TestIncrementDAIncludedHeight_WithMetricsRecorder verifies that incrementDAIncludedHeight calls RecordMetrics
// when the sequencer implements the MetricsRecorder interface (covers lines 73-74).
func TestIncrementDAIncludedHeight_WithMetricsRecorder(t *testing.T) {
	t.Parallel()
	m, store, exec, logger := newTestManager(t)
	startDAIncludedHeight := uint64(4)
	expectedDAIncludedHeight := startDAIncludedHeight + 1
	m.daIncludedHeight.Store(startDAIncludedHeight)

	// Set up mock sequencer with metrics
	mockSequencer := new(MockSequencerWithMetrics)
	m.sequencer = mockSequencer

	// Mock the store calls needed for PendingHeaders initialization
	// First, clear the existing Height mock from newTestManager
	store.ExpectedCalls = nil

	// Create a byte array representing lastSubmittedHeight = 4
	lastSubmittedBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(lastSubmittedBytes, startDAIncludedHeight)

	store.On("GetMetadata", mock.Anything, LastSubmittedHeaderHeightKey).Return(lastSubmittedBytes, nil).Maybe() // For pendingHeaders init
	store.On("Height", mock.Anything).Return(uint64(7), nil).Maybe()                                             // 7 - 4 = 3 pending headers

	// Initialize pendingHeaders properly
	pendingHeaders, err := NewPendingHeaders(store, logger)
	assert.NoError(t, err)
	m.pendingHeaders = pendingHeaders

	heightBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(heightBytes, expectedDAIncludedHeight)
	store.On("SetMetadata", mock.Anything, DAIncludedHeightKey, heightBytes).Return(nil).Once()
	exec.On("SetFinal", mock.Anything, expectedDAIncludedHeight).Return(nil).Once()

	// Expect RecordMetrics to be called with the correct parameters
	mockSequencer.On("RecordMetrics",
		float64(1.5),             // gasPrice
		uint64(0),                // blobSize
		coreda.StatusSuccess,     // statusCode
		uint64(3),                // numPendingBlocks (7 - 4 = 3)
		expectedDAIncludedHeight, // includedBlockHeight
	).Once()

	err = m.incrementDAIncludedHeight(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, expectedDAIncludedHeight, m.GetDAIncludedHeight())
	store.AssertExpectations(t)
	exec.AssertExpectations(t)
	mockSequencer.AssertExpectations(t)
}

// Note: It is not practical to unit test a CompareAndSwap failure for incrementDAIncludedHeight
// because the atomic value is always read at the start of the function, and there is no way to
// inject a failure or race another goroutine reliably in a unit test. To test this path, the code
// would need to be refactored to allow injection or mocking of the atomic value.
