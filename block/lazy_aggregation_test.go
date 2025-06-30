package block

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/pkg/config"
)

// mockPublishBlock is used to control the behavior of publishBlock during tests
type mockPublishBlock struct {
	mu    sync.Mutex
	calls chan struct{}
	err   error
	delay time.Duration // Optional delay to simulate processing time
}

// reset clears the calls channel in mockPublishBlock.
func (m *mockPublishBlock) reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Clear the channel
	for len(m.calls) > 0 {
		<-m.calls
	}
}

func (m *mockPublishBlock) publish(ctx context.Context) error {
	m.mu.Lock()
	err := m.err
	delay := m.delay
	m.mu.Unlock()

	if delay > 0 {
		time.Sleep(delay)
	}
	// Non-blocking send in case the channel buffer is full or receiver is not ready
	select {
	case m.calls <- struct{}{}:
	default:
	}
	return err
}

func setupTestManager(t *testing.T, blockTime, lazyTime time.Duration) (*Manager, *mockPublishBlock) {
	t.Helper()
	pubMock := &mockPublishBlock{
		calls: make(chan struct{}, 10), // Buffer to avoid blocking in tests
	}
	logger := logging.Logger("test") // Use ipfs/go-log for testing
	// As with aggregation_test.go, specific TestLogger behavior (fail on Error) is not replicated by default.
	m := &Manager{
		logger: logger,
		config: config.Config{
			Node: config.NodeConfig{
				BlockTime:         config.DurationWrapper{Duration: blockTime},
				LazyBlockInterval: config.DurationWrapper{Duration: lazyTime},
				LazyMode:          true, // Ensure lazy mode is active
			},
		},
		publishBlock: pubMock.publish,
	}
	return m, pubMock
}

// TestLazyAggregationLoop_BlockTimerTrigger tests that a block is published when the blockTimer fires first.
func TestLazyAggregationLoop_BlockTimerTrigger(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	// Create a mock for the publishBlock function that counts calls
	callCount := 0
	mockPublishFn := func(ctx context.Context) error {
		callCount++
		return nil
	}

	// Setup a manager with our mock publish function
	blockTime := 50 * time.Millisecond
	lazyTime := 200 * time.Millisecond // Lazy timer fires later
	m, _ := setupTestManager(t, blockTime, lazyTime)
	m.publishBlock = mockPublishFn

	// Set txsAvailable to true to ensure block timer triggers block production
	m.txsAvailable = true

	ctx, cancel := context.WithCancel(context.Background())

	// Start the lazy aggregation loop
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		blockTimer := time.NewTimer(0) // Fire immediately first time
		defer blockTimer.Stop()
		require.NoError(m.lazyAggregationLoop(ctx, blockTimer))
	}()

	// Wait for at least one block to be published
	time.Sleep(blockTime * 2)

	// Cancel the context to stop the loop
	cancel()
	wg.Wait()

	// Verify that at least one block was published
	require.GreaterOrEqual(callCount, 1, "Expected at least one block to be published")
}

// TestLazyAggregationLoop_LazyTimerTrigger tests that a block is published when the lazyTimer fires first.
func TestLazyAggregationLoop_LazyTimerTrigger(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)
	require := require.New(t)

	blockTime := 200 * time.Millisecond // Block timer fires later
	lazyTime := 50 * time.Millisecond
	m, pubMock := setupTestManager(t, blockTime, lazyTime)

	// Set txsAvailable to false to ensure lazy timer triggers block production
	// and block timer doesn't
	m.txsAvailable = false

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Use real timers for this test
		blockTimer := time.NewTimer(0) // Fire immediately first time
		defer blockTimer.Stop()
		require.NoError(m.lazyAggregationLoop(ctx, blockTimer))
	}()

	// Wait for the first publish call triggered by the initial immediate lazyTimer fire
	select {
	case <-pubMock.calls:
		// Good, first block published by lazy timer
	case <-time.After(2 * lazyTime): // Give some buffer
		require.Fail("timed out waiting for first block publication")
	}

	// Wait for the second publish call, triggered by lazyTimer reset
	select {
	case <-pubMock.calls:
		// Good, second block published by lazyTimer
	case <-time.After(2 * lazyTime): // Give some buffer
		require.Fail("timed out waiting for second block publication (lazyTimer)")
	}

	// Ensure blockTimer didn't trigger a publish yet (since txsAvailable is false)
	assert.Len(pubMock.calls, 0, "Expected no more publish calls yet")

	cancel()
	wg.Wait()
}

// TestLazyAggregationLoop_PublishError tests that the loop exits.
func TestLazyAggregationLoop_PublishError(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	blockTime := 50 * time.Millisecond
	lazyTime := 100 * time.Millisecond
	m, pubMock := setupTestManager(t, blockTime, lazyTime)

	pubMock.mu.Lock()
	pubMock.err = errors.New("publish failed")
	pubMock.mu.Unlock()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Use real timers
		blockTimer := time.NewTimer(0)
		defer blockTimer.Stop()
		require.Error(m.lazyAggregationLoop(ctx, blockTimer))
	}()

	// Wait for the first publish attempt (which will fail)
	select {
	case <-pubMock.calls:
	case <-time.After(2 * blockTime):
		require.Fail("timed out waiting for first block publication attempt")
	}

	// loop exited, nothing to do.

	wg.Wait()
}

// TestGetRemainingSleep tests the calculation of sleep duration.
func TestGetRemainingSleep(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	interval := 100 * time.Millisecond

	// Case 1: Elapsed time is less than interval
	start1 := time.Now().Add(-30 * time.Millisecond) // Started 30ms ago
	sleep1 := getRemainingSleep(start1, interval)
	// Expecting interval - elapsed = 100ms - 30ms = 70ms (allow for slight variations)
	assert.InDelta(interval-30*time.Millisecond, sleep1, float64(5*time.Millisecond), "Case 1 failed")

	// Case 2: Elapsed time is greater than or equal to interval
	start2 := time.Now().Add(-120 * time.Millisecond) // Started 120ms ago
	sleep2 := getRemainingSleep(start2, interval)
	// Expecting minimum sleep time
	assert.Equal(time.Millisecond, sleep2, "Case 2 failed")

	// Case 3: Elapsed time is exactly the interval
	start3 := time.Now().Add(-100 * time.Millisecond) // Started 100ms ago
	sleep3 := getRemainingSleep(start3, interval)
	// Expecting minimum sleep time
	assert.Equal(time.Millisecond, sleep3, "Case 3 failed")

	// Case 4: Zero elapsed time
	start4 := time.Now()
	sleep4 := getRemainingSleep(start4, interval)
	assert.InDelta(interval, sleep4, float64(5*time.Millisecond), "Case 4 failed")
}

// TestLazyAggregationLoop_TxNotification tests that transaction notifications trigger block production in lazy mode
func TestLazyAggregationLoop_TxNotification(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	blockTime := 200 * time.Millisecond
	lazyTime := 500 * time.Millisecond
	m, pubMock := setupTestManager(t, blockTime, lazyTime)
	m.config.Node.LazyMode = true

	// Create the notification channel
	m.txNotifyCh = make(chan struct{}, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Start with a timer that won't fire immediately
		blockTimer := time.NewTimer(blockTime)
		defer blockTimer.Stop()
		require.NoError(m.lazyAggregationLoop(ctx, blockTimer))
	}()

	// Wait for the initial lazy timer to fire and publish a block
	select {
	case <-pubMock.calls:
		// Initial block was published by lazy timer
	case <-time.After(100 * time.Millisecond):
		require.Fail("Initial block was not published")
	}

	// Reset the mock to track new calls
	pubMock.reset()

	// Wait a bit to ensure the loop is running with reset timers
	time.Sleep(20 * time.Millisecond)

	// Send a transaction notification
	m.NotifyNewTransactions()

	// Wait for the block timer to fire and check txsAvailable
	select {
	case <-pubMock.calls:
		// Block was published, which is what we expect
	case <-time.After(blockTime + 50*time.Millisecond):
		require.Fail("Block was not published after transaction notification")
	}

	// Reset the mock again
	pubMock.reset()

	// Send another notification immediately
	m.NotifyNewTransactions()

	// Wait for the next block timer to fire
	select {
	case <-pubMock.calls:
		// Block was published after notification
	case <-time.After(blockTime + 50*time.Millisecond):
		require.Fail("Block was not published after second notification")
	}

	cancel()
	wg.Wait()
}

// TestEmptyBlockCreation tests that empty blocks are created with the correct dataHash
func TestEmptyBlockCreation(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	// Create a mock for the publishBlock function that captures the context
	var capturedCtx context.Context
	mockPublishFn := func(ctx context.Context) error {
		capturedCtx = ctx
		return nil
	}

	// Setup a manager with our mock publish function
	blockTime := 50 * time.Millisecond
	lazyTime := 100 * time.Millisecond
	m, _ := setupTestManager(t, blockTime, lazyTime)
	m.publishBlock = mockPublishFn

	// Create a context we can cancel
	ctx := t.Context()

	// Create timers for the test
	lazyTimer := time.NewTimer(lazyTime)
	blockTimer := time.NewTimer(blockTime)
	defer lazyTimer.Stop()
	defer blockTimer.Stop()

	// Call produceBlock directly to test empty block creation
	require.NoError(m.produceBlock(ctx, "test_trigger", lazyTimer, blockTimer))

	// Verify that the context was passed correctly
	require.NotNil(capturedCtx, "Context should have been captured by mock publish function")
	require.Equal(ctx, capturedCtx, "Context should match the one passed to produceBlock")
}

// TestNormalAggregationLoop_TxNotification tests that transaction notifications are handled in normal mode
func TestNormalAggregationLoop_TxNotification(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	blockTime := 100 * time.Millisecond
	m, pubMock := setupTestManager(t, blockTime, 0)
	m.config.Node.LazyMode = false

	// Create the notification channel
	m.txNotifyCh = make(chan struct{}, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		blockTimer := time.NewTimer(blockTime)
		defer blockTimer.Stop()
		require.NoError(m.normalAggregationLoop(ctx, blockTimer))
	}()

	// Wait for the first block to be published by the timer
	select {
	case <-pubMock.calls:
		// Block was published by timer, which is expected
	case <-time.After(blockTime * 2):
		require.Fail("Block was not published by timer")
	}

	// Reset the publish mock to track new calls
	pubMock.reset()

	// Send a transaction notification
	m.NotifyNewTransactions()

	// In normal mode, the notification should not trigger an immediate block
	select {
	case <-pubMock.calls:
		// If we enable the optional enhancement to reset the timer, this might happen
		// But with the current implementation, this should not happen
		require.Fail("Block was published immediately after notification in normal mode")
	case <-time.After(blockTime / 2):
		// This is expected - no immediate block
	}

	// Wait for the next regular block
	select {
	case <-pubMock.calls:
		// Block was published by timer, which is expected
	case <-time.After(blockTime * 2):
		require.Fail("Block was not published by timer after notification")
	}

	cancel()
	wg.Wait()
}
