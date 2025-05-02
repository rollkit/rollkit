package block

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"cosmossdk.io/log"
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
	logger := log.NewTestLogger(t)
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
	assert := assert.New(t)
	require := require.New(t)

	blockTime := 50 * time.Millisecond
	lazyTime := 200 * time.Millisecond // Lazy timer fires later
	m, pubMock := setupTestManager(t, blockTime, lazyTime)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Use real timers for this test to simulate actual timing
		blockTimer := time.NewTimer(0) // Fire immediately first time
		defer blockTimer.Stop()
		m.lazyAggregationLoop(ctx, blockTimer)
	}()

	// Wait for the first publish call triggered by the initial immediate blockTimer fire
	select {
	case <-pubMock.calls:
		// Good, first block published
	case <-time.After(2 * blockTime): // Give some buffer
		require.Fail("timed out waiting for first block publication")
	}

	// Wait for the second publish call, triggered by blockTimer reset
	select {
	case <-pubMock.calls:
		// Good, second block published by blockTimer
	case <-time.After(2 * blockTime): // Give some buffer
		require.Fail("timed out waiting for second block publication (blockTimer)")
	}

	// Ensure lazyTimer didn't trigger a publish yet
	assert.Len(pubMock.calls, 0, "Expected no more publish calls yet")

	cancel()
	wg.Wait()
}

// TestLazyAggregationLoop_LazyTimerTrigger tests that a block is published when the lazyTimer fires first.
func TestLazyAggregationLoop_LazyTimerTrigger(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	blockTime := 200 * time.Millisecond // Block timer fires later
	lazyTime := 50 * time.Millisecond
	m, pubMock := setupTestManager(t, blockTime, lazyTime)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Use real timers for this test
		blockTimer := time.NewTimer(0) // Fire immediately first time
		defer blockTimer.Stop()
		m.lazyAggregationLoop(ctx, blockTimer)
	}()

	// Wait for the first publish call triggered by the initial immediate blockTimer fire
	select {
	case <-pubMock.calls:
		// Good, first block published
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

	// Ensure blockTimer didn't trigger a publish yet
	assert.Len(pubMock.calls, 0, "Expected no more publish calls yet")

	cancel()
	wg.Wait()
}

// TestLazyAggregationLoop_PublishError tests that the loop continues after a publish error.
func TestLazyAggregationLoop_PublishError(t *testing.T) {
	require := require.New(t)

	blockTime := 50 * time.Millisecond
	lazyTime := 100 * time.Millisecond
	m, pubMock := setupTestManager(t, blockTime, lazyTime)

	pubMock.mu.Lock()
	pubMock.err = errors.New("publish failed")
	pubMock.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Use real timers
		blockTimer := time.NewTimer(0)
		defer blockTimer.Stop()
		m.lazyAggregationLoop(ctx, blockTimer)
	}()

	// Wait for the first publish attempt (which will fail)
	select {
	case <-pubMock.calls:
	case <-time.After(2 * blockTime):
		require.Fail("timed out waiting for first block publication attempt")
	}

	// Remove the error for subsequent calls
	pubMock.mu.Lock()
	pubMock.err = nil
	pubMock.mu.Unlock()

	// Wait for the second publish attempt (should succeed)
	select {
	case <-pubMock.calls:
	case <-time.After(2 * blockTime):
		require.Fail("timed out waiting for second block publication attempt after error")
	}

	cancel()
	wg.Wait()
}

// TestGetRemainingSleep tests the calculation of sleep duration.
func TestGetRemainingSleep(t *testing.T) {
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
