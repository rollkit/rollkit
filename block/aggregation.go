package block

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// AggregationLoop is responsible for aggregating transactions into rollup-blocks.
func (m *Manager) AggregationLoop(ctx context.Context) {
	initialHeight := m.genesis.InitialHeight //nolint:gosec
	height, err := m.store.Height(ctx)
	if err != nil {
		m.logger.Error("error while getting store height", "error", err)
		return
	}
	var delay time.Duration

	if height < initialHeight {
		delay = time.Until(m.genesis.GenesisDAStartTime.Add(m.config.Node.BlockTime.Duration))
	} else {
		lastBlockTime := m.getLastBlockTime()
		delay = time.Until(lastBlockTime.Add(m.config.Node.BlockTime.Duration))
	}

	if delay > 0 {
		m.logger.Info("Waiting to produce block", "delay", delay)
		time.Sleep(delay)
	}

	// blockTimer is used to signal when to build a block based on the
	// rollup block time. A timer is used so that the time to build a block
	// can be taken into account.
	blockTimer := time.NewTimer(0)
	defer blockTimer.Stop()

	// Lazy Aggregator mode.
	// In Lazy Aggregator mode, blocks are built only when there are
	// transactions or every LazyBlockTime.
	if m.config.Node.LazyMode {
		m.lazyAggregationLoop(ctx, blockTimer)
		return
	}

	m.normalAggregationLoop(ctx, blockTimer)
}

func (m *Manager) lazyAggregationLoop(ctx context.Context, blockTimer *time.Timer) {
	// lazyTimer triggers block publication even during inactivity
	lazyTimer := time.NewTimer(0)
	defer lazyTimer.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-lazyTimer.C:
			m.logger.Debug("Lazy timer triggered block production")
			m.produceBlock(ctx, "lazy_timer", lazyTimer, blockTimer)

		case <-blockTimer.C:
			if m.txsAvailable {
				m.produceBlock(ctx, "block_timer", lazyTimer, blockTimer)
				m.txsAvailable = false
			} else {
				// Ensure we keep ticking even when there are no txs
				blockTimer.Reset(m.config.Node.BlockTime.Duration)
			}
		case <-m.txNotifyCh:
			m.txsAvailable = true
		}
	}
}

// produceBlock handles the common logic for producing a block and resetting timers
func (m *Manager) produceBlock(ctx context.Context, trigger string, lazyTimer, blockTimer *time.Timer) {
	// Record the start time
	start := time.Now()

	// Attempt to publish the block
	if err := m.publishBlock(ctx); err != nil && ctx.Err() == nil {
		m.logger.Error("error while publishing block", "trigger", trigger, "error", err)
	} else {
		m.logger.Debug("Successfully published block", "trigger", trigger)
	}

	// Reset both timers for the next aggregation window
	lazyTimer.Reset(getRemainingSleep(start, m.config.Node.LazyBlockInterval.Duration))
	blockTimer.Reset(getRemainingSleep(start, m.config.Node.BlockTime.Duration))
}

func (m *Manager) normalAggregationLoop(ctx context.Context, blockTimer *time.Timer) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-blockTimer.C:
			// Define the start time for the block production period
			start := time.Now()

			if err := m.publishBlock(ctx); err != nil && ctx.Err() == nil {
				m.logger.Error("error while publishing block", "error", err)
			}
			// Reset the blockTimer to signal the next block production
			// period based on the block time.
			blockTimer.Reset(getRemainingSleep(start, m.config.Node.BlockTime.Duration))

		case <-m.txNotifyCh:
			// Transaction notifications are intentionally ignored in normal mode
			// to avoid triggering block production outside the scheduled intervals.
			// We just update the txsAvailable flag for tracking purposes
			m.txsAvailable = true
		}
	}
}

func getRemainingSleep(start time.Time, interval time.Duration) time.Duration {
	elapsed := time.Since(start)

	if elapsed < interval {
		return interval - elapsed
	}

	return time.Millisecond
}

// TestNormalAggregationLoop_TxNotification tests that transaction notifications are handled in normal mode
func TestNormalAggregationLoop_TxNotification(t *testing.T) {
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
		m.normalAggregationLoop(ctx, blockTimer)
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
