package block

import (
	"context"
	"fmt"
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
		if err := m.lazyAggregationLoop(ctx, blockTimer); err != nil {
			panic(fmt.Errorf("error in lazy aggregation loop: %w", err))
		}

		return
	}

	if err := m.normalAggregationLoop(ctx, blockTimer); err != nil {
		panic(fmt.Errorf("error in normal aggregation loop: %w", err))
	}
}

func (m *Manager) lazyAggregationLoop(ctx context.Context, blockTimer *time.Timer) error {
	// lazyTimer triggers block publication even during inactivity
	lazyTimer := time.NewTimer(0)
	defer lazyTimer.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil

		case <-lazyTimer.C:
			m.logger.Debug("Lazy timer triggered block production")

			err := m.produceBlock(ctx, "lazy_timer", lazyTimer, blockTimer)
			if err != nil {
				return err
			}
		case <-blockTimer.C:
			if m.txsAvailable {
				if err := m.produceBlock(ctx, "block_timer", lazyTimer, blockTimer); err != nil {
					return err
				}

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
func (m *Manager) produceBlock(ctx context.Context, mode string, lazyTimer, blockTimer *time.Timer) error {
	start := time.Now()

	// Attempt to publish the block
	if err := m.publishBlock(ctx); err != nil && ctx.Err() == nil {
		return fmt.Errorf("error while publishing block: %w", err)
	}

	m.logger.Debug("Successfully published block", "mode", mode)

	// Reset both timers for the next aggregation window
	lazyTimer.Reset(getRemainingSleep(start, m.config.Node.LazyBlockInterval.Duration))
	blockTimer.Reset(getRemainingSleep(start, m.config.Node.BlockTime.Duration))

	return nil
}

func (m *Manager) normalAggregationLoop(ctx context.Context, blockTimer *time.Timer) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-blockTimer.C:
			// Define the start time for the block production period
			start := time.Now()

			if err := m.publishBlock(ctx); err != nil && ctx.Err() == nil {
				return fmt.Errorf("error while publishing block: %w", err)
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
