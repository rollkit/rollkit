package block

import (
	"context"
	"errors"
	"time"
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

	// Initialize the throttle timer but don't start it yet
	m.txNotifyThrottle = time.NewTimer(m.minTxNotifyInterval)
	if !m.txNotifyThrottle.Stop() {
		<-m.txNotifyThrottle.C // Drain the channel if it already fired
	}
	defer m.txNotifyThrottle.Stop()

	// Add a polling timer to periodically check for transactions
	// This is a fallback mechanism in case notifications fail
	txPollTimer := time.NewTimer(2 * time.Second)
	defer txPollTimer.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-lazyTimer.C:
			m.logger.Debug("Lazy timer triggered block production")
			m.produceBlock(ctx, "lazy_timer", lazyTimer, blockTimer)

		case <-blockTimer.C:
			m.logger.Debug("Block timer triggered block production")
			// m.produceBlock(ctx, "block_timer", lazyTimer, blockTimer)

		case <-m.txNotifyCh:
			// Only proceed if we're not being throttled
			if time.Since(m.lastTxNotifyTime) < m.minTxNotifyInterval {
				m.logger.Debug("Transaction notification throttled")
				continue
			}

			m.logger.Debug("Transaction notification triggered block production")
			m.produceBlock(ctx, "tx_notification", lazyTimer, blockTimer)

			// Update the last notification time
			m.lastTxNotifyTime = time.Now()

		case <-txPollTimer.C:
			// Check if there are transactions available
			hasTxs, err := m.checkForTransactions(ctx)
			if err != nil {
				m.logger.Error("Failed to check for transactions", "error", err)
			} else if hasTxs {
				m.logger.Debug("Transaction poll detected transactions")
				m.produceBlock(ctx, "tx_poll", lazyTimer, blockTimer)
			}
			txPollTimer.Reset(2 * time.Second)
		}
	}
}

func (m *Manager) normalAggregationLoop(ctx context.Context, blockTimer *time.Timer) {
	// Initialize the throttle timer but don't start it yet
	m.txNotifyThrottle = time.NewTimer(m.minTxNotifyInterval)
	if !m.txNotifyThrottle.Stop() {
		<-m.txNotifyThrottle.C // Drain the channel if it already fired
	}
	defer m.txNotifyThrottle.Stop()

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
			// clear notification channel
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

func getRemainingSleep(start time.Time, interval time.Duration) time.Duration {
	elapsed := time.Since(start)

	if elapsed < interval {
		return interval - elapsed
	}

	return time.Millisecond
}

// checkForTransactions checks if there are any transactions available for processing
func (m *Manager) checkForTransactions(ctx context.Context) (bool, error) {
	// Try to retrieve a batch without actually consuming it
	batchData, err := m.retrieveBatch(ctx)
	if err != nil {
		if errors.Is(err, ErrNoBatch) {
			// This is expected when there are no transactions
			return false, nil
		}
		return false, err
	}

	// If we got a batch with transactions, return true
	if batchData != nil && batchData.Batch != nil && len(batchData.Batch.Transactions) > 0 {
		return true, nil
	}

	return false, nil
}
