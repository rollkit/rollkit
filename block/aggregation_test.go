package block

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cosmossdk.io/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/pkg/cache"
	"github.com/rollkit/rollkit/pkg/config"
	genesispkg "github.com/rollkit/rollkit/pkg/genesis"
	"github.com/rollkit/rollkit/test/mocks"
	"github.com/rollkit/rollkit/types"
)

// TestAggregationLoop_Normal_BasicInterval verifies that the aggregation loop publishes blocks at the expected interval under normal conditions.
func TestAggregationLoop_Normal_BasicInterval(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)
	require := require.New(t)

	blockTime := 50 * time.Millisecond
	waitTime := blockTime*4 + blockTime/2

	mockStore := mocks.NewStore(t)
	mockStore.On("Height", mock.Anything).Return(uint64(1), nil).Maybe()
	mockStore.On("GetState", mock.Anything).Return(types.State{LastBlockTime: time.Now().Add(-blockTime)}, nil).Maybe()

	mockExec := mocks.NewExecutor(t)
	mockSeq := mocks.NewSequencer(t)
	mockDAC := mocks.NewClient(t)
	logger := log.NewTestLogger(t)

	m := &Manager{
		store:     mockStore,
		exec:      mockExec,
		sequencer: mockSeq,
		dalc:      mockDAC,
		logger:    logger,
		config: config.Config{
			Node: config.NodeConfig{
				BlockTime: config.DurationWrapper{Duration: blockTime},
				LazyMode:  false,
			},
			DA: config.DAConfig{
				BlockTime: config.DurationWrapper{Duration: 1 * time.Second},
			},
		},
		genesis: genesispkg.Genesis{
			InitialHeight: 1,
		},
		lastState: types.State{
			LastBlockTime: time.Now().Add(-blockTime),
		},
		lastStateMtx: &sync.RWMutex{},
		metrics:      NopMetrics(),
		headerCache:  cache.NewCache[types.SignedHeader](),
		dataCache:    cache.NewCache[types.Data](),
	}

	var publishTimes []time.Time
	var publishLock sync.Mutex
	mockPublishBlock := func(ctx context.Context) error {
		publishLock.Lock()
		defer publishLock.Unlock()
		publishTimes = append(publishTimes, time.Now())
		m.logger.Debug("Mock publishBlock called", "time", publishTimes[len(publishTimes)-1])
		return nil
	}
	m.publishBlock = mockPublishBlock

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.AggregationLoop(ctx)
		m.logger.Info("AggregationLoop exited")
	}()

	m.logger.Info("Waiting for blocks...", "duration", waitTime)
	time.Sleep(waitTime)

	m.logger.Info("Cancelling context")
	cancel()
	m.logger.Info("Waiting for WaitGroup")
	wg.Wait()
	m.logger.Info("WaitGroup finished")

	publishLock.Lock()
	defer publishLock.Unlock()

	m.logger.Info("Recorded publish times", "count", len(publishTimes), "times", publishTimes)

	expectedCallsLow := int(waitTime/blockTime) - 1
	expectedCallsHigh := int(waitTime/blockTime) + 1
	require.GreaterOrEqualf(len(publishTimes), expectedCallsLow, "Expected at least %d calls, got %d", expectedCallsLow, len(publishTimes))
	require.LessOrEqualf(len(publishTimes), expectedCallsHigh, "Expected at most %d calls, got %d", expectedCallsHigh, len(publishTimes))

	if len(publishTimes) > 1 {
		for i := 1; i < len(publishTimes); i++ {
			interval := publishTimes[i].Sub(publishTimes[i-1])
			m.logger.Debug("Checking interval", "index", i, "interval", interval)
			tolerance := blockTime / 2
			assert.True(WithinDuration(t, blockTime, interval, tolerance), "Interval %d (%v) not within tolerance (%v) of blockTime (%v)", i, interval, tolerance, blockTime)
		}
	}
}

// TestAggregationLoop_Normal_PublishBlockError verifies that the aggregation loop handles errors from publishBlock gracefully.
func TestAggregationLoop_Normal_PublishBlockError(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)
	require := require.New(t)

	blockTime := 50 * time.Millisecond
	waitTime := blockTime*4 + blockTime/2
	tolerance := blockTime / 2

	mockStore := mocks.NewStore(t)
	mockStore.On("Height", mock.Anything).Return(uint64(1), nil).Maybe()
	mockStore.On("GetState", mock.Anything).Return(types.State{LastBlockTime: time.Now().Add(-blockTime)}, nil).Maybe()

	mockExec := mocks.NewExecutor(t)
	mockSeq := mocks.NewSequencer(t)
	mockDAC := mocks.NewClient(t)

	mockLogger := log.NewTestLogger(t)

	// Create a basic Manager instance
	m := &Manager{
		store:     mockStore,
		exec:      mockExec,
		sequencer: mockSeq,
		dalc:      mockDAC,
		logger:    mockLogger,
		config: config.Config{
			Node: config.NodeConfig{
				BlockTime: config.DurationWrapper{Duration: blockTime},
				LazyMode:  false,
			},
			DA: config.DAConfig{
				BlockTime: config.DurationWrapper{Duration: 1 * time.Second},
			},
		},
		genesis: genesispkg.Genesis{
			InitialHeight: 1,
		},
		lastState: types.State{
			LastBlockTime: time.Now().Add(-blockTime),
		},
		lastStateMtx: &sync.RWMutex{},
		metrics:      NopMetrics(),
		headerCache:  cache.NewCache[types.SignedHeader](),
		dataCache:    cache.NewCache[types.Data](),
	}

	var publishCalls atomic.Int64
	var publishTimes []time.Time
	var publishLock sync.Mutex
	expectedErr := errors.New("failed to publish block")

	mockPublishBlock := func(ctx context.Context) error {
		callNum := publishCalls.Add(1)
		publishLock.Lock()
		publishTimes = append(publishTimes, time.Now())
		publishLock.Unlock()

		if callNum == 1 {
			m.logger.Debug("Mock publishBlock returning error", "call", callNum)
			return expectedErr
		}
		m.logger.Debug("Mock publishBlock returning nil", "call", callNum)
		return nil
	}
	m.publishBlock = mockPublishBlock

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.AggregationLoop(ctx)
		m.logger.Info("AggregationLoop exited")
	}()

	time.Sleep(waitTime)

	cancel()
	wg.Wait()

	publishLock.Lock()
	defer publishLock.Unlock()

	calls := publishCalls.Load()
	assert.GreaterOrEqualf(calls, int64(4), "publishBlock should have been called multiple times (around 4), but was called %d times", calls)
	assert.LessOrEqualf(calls, int64(5), "publishBlock should have been called multiple times (around 4-5), but was called %d times", calls)

	require.GreaterOrEqual(len(publishTimes), 3, "Need at least 3 timestamps to check intervals after error")
	for i := 2; i < len(publishTimes); i++ {
		interval := publishTimes[i].Sub(publishTimes[i-1])
		WithinDuration(t, blockTime, interval, tolerance)
	}
}
