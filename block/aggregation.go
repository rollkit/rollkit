package block

import (
	"context"
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

	pendingTxs := false
	buildingBlock := false

	for {
		select {
		case <-ctx.Done():
			return

		case <-lazyTimer.C:
			pendingTxs = false
			buildingBlock = true
			m.produceBlockLazy(ctx, "lazy_timer", lazyTimer, blockTimer)
			buildingBlock = false

		case <-blockTimer.C:
			if pendingTxs {
				buildingBlock = true
				m.produceBlockLazy(ctx, "tx_collection_complete", lazyTimer, blockTimer)
				buildingBlock = false
				pendingTxs = false
			} else {
				// Reset the block timer if no pending transactions
				blockTimer.Reset(m.config.Node.BlockTime.Duration)
			}

		case <-m.txNotifyCh:
			if buildingBlock {
				continue
			}

			// Instead of immediately producing a block, mark that we have pending transactions
			// and let the block timer determine when to actually build the block
			if !pendingTxs {
				pendingTxs = true
				m.logger.Debug("Received transaction notification, waiting for more transactions",
					"collection_time", m.config.Node.BlockTime.Duration)
				// Reset the block timer to wait for more transactions
				blockTimer.Reset(m.config.Node.BlockTime.Duration)
			}

		}
	}
}

func (m *Manager) normalAggregationLoop(ctx context.Context, blockTimer *time.Timer) {

	for {
		select {
		case <-ctx.Done():
			return
		case <-blockTimer.C:
			start := time.Now()

			if err := m.publishBlock(ctx); err != nil && ctx.Err() == nil {
				m.logger.Error("error while publishing block", "error", err)
			}
			// Reset the blockTimer to signal the next block production
			blockTimer.Reset(getRemainingSleep(start, m.config.Node.BlockTime.Duration))

		case <-m.txNotifyCh:
			// Transaction notifications are intentionally ignored in normal mode
			// to avoid triggering block production outside the scheduled intervals.
		}
	}
}

// produceBlockLazy handles the common logic for producing a block and resetting timers in lazy mode
func (m *Manager) produceBlockLazy(ctx context.Context, trigger string, lazyTimer, blockTimer *time.Timer) {
	// Record the start time
	start := time.Now()

	// Attempt to publish the block
	if err := m.publishBlock(ctx); err != nil && ctx.Err() == nil {
		m.logger.Error("error while publishing block", "trigger", trigger, "error", err)
	} else {
		m.logger.Debug("Successfully published block", "trigger", trigger)
	}

	// Reset the lazy timer for the next aggregation window
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
