package block

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	logging "github.com/ipfs/go-log/v2"
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

	mockStore := mocks.NewMockStore(t)
	mockStore.On("Height", mock.Anything).Return(uint64(1), nil).Maybe()
	mockStore.On("GetState", mock.Anything).Return(types.State{LastBlockTime: time.Now().Add(-blockTime)}, nil).Maybe()

	mockExec := mocks.NewMockExecutor(t)
	mockSeq := mocks.NewMockSequencer(t)
	mockDAC := mocks.NewMockDA(t)
	logger := logging.Logger("test") // Use ipfs/go-log for testing
	// To get similar behavior to NewTestLogger, ensure test output is configured if needed.
	// For now, a basic logger instance is created.
	// If specific test logging (like failing on Error) is needed, it requires custom setup or a test helper.

	m := &Manager{
		store:     mockStore,
		exec:      mockExec,
		sequencer: mockSeq,
		da:        mockDAC,
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
		m.AggregationLoop(ctx, make(chan<- error))
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
	require := require.New(t)

	blockTime := 50 * time.Millisecond
	waitTime := blockTime*4 + blockTime/2

	mockStore := mocks.NewMockStore(t)
	mockStore.On("Height", mock.Anything).Return(uint64(1), nil).Maybe()
	mockStore.On("GetState", mock.Anything).Return(types.State{LastBlockTime: time.Now().Add(-blockTime)}, nil).Maybe()

	mockExec := mocks.NewMockExecutor(t)
	mockSeq := mocks.NewMockSequencer(t)
	mockDAC := mocks.NewMockDA(t)

	logger := logging.Logger("test") // Use ipfs/go-log for testing

	// Create a basic Manager instance
	m := &Manager{
		store:     mockStore,
		exec:      mockExec,
		sequencer: mockSeq,
		da:        mockDAC,
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
	errCh := make(chan error, 1)

	wg.Add(1)
	go func() {
		defer wg.Done()
		m.AggregationLoop(ctx, errCh)
		m.logger.Info("AggregationLoop exited")
	}()

	time.Sleep(waitTime)

	cancel()
	wg.Wait()

	publishLock.Lock()
	defer publishLock.Unlock()

	calls := publishCalls.Load()
	require.Equal(calls, int64(1))
	require.ErrorContains(<-errCh, expectedErr.Error())
	require.Equal(len(publishTimes), 1, "Expected only one publish time after error")
}
