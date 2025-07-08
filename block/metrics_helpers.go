package block

import (
	"runtime"
	"time"
)

// MetricsTimer helps track operation duration
type MetricsTimer struct {
	start     time.Time
	operation string
	metrics   *Metrics
}

// NewMetricsTimer creates a new timer for tracking operation duration
func NewMetricsTimer(operation string, metrics *Metrics) *MetricsTimer {
	return &MetricsTimer{
		start:     time.Now(),
		operation: operation,
		metrics:   metrics,
	}
}

// Stop stops the timer and records the duration
func (t *MetricsTimer) Stop() {
	if t.metrics != nil && t.metrics.OperationDuration[t.operation] != nil {
		duration := time.Since(t.start).Seconds()
		t.metrics.OperationDuration[t.operation].Observe(duration)
	}
}

// sendNonBlockingSignalWithMetrics sends a signal to a channel and tracks if it was dropped
func (m *Manager) sendNonBlockingSignalWithMetrics(ch chan<- struct{}, channelName string) bool {
	select {
	case ch <- struct{}{}:
		return true
	default:
		m.metrics.DroppedSignals.Add(1)
		m.logger.Debug("dropped signal", "channel", channelName)
		return false
	}
}

// updateChannelMetrics updates the buffer usage metrics for all channels
func (m *Manager) updateChannelMetrics() {
	// Update channel buffer usage
	if m.metrics.ChannelBufferUsage["header_in"] != nil {
		m.metrics.ChannelBufferUsage["header_in"].Set(float64(len(m.headerInCh)))
	}
	if m.metrics.ChannelBufferUsage["data_in"] != nil {
		m.metrics.ChannelBufferUsage["data_in"].Set(float64(len(m.dataInCh)))
	}
	if m.metrics.ChannelBufferUsage["header_store"] != nil {
		m.metrics.ChannelBufferUsage["header_store"].Set(float64(len(m.headerStoreCh)))
	}
	if m.metrics.ChannelBufferUsage["data_store"] != nil {
		m.metrics.ChannelBufferUsage["data_store"].Set(float64(len(m.dataStoreCh)))
	}
	if m.metrics.ChannelBufferUsage["retrieve"] != nil {
		m.metrics.ChannelBufferUsage["retrieve"].Set(float64(len(m.retrieveCh)))
	}
	if m.metrics.ChannelBufferUsage["da_includer"] != nil {
		m.metrics.ChannelBufferUsage["da_includer"].Set(float64(len(m.daIncluderCh)))
	}
	if m.metrics.ChannelBufferUsage["tx_notify"] != nil {
		m.metrics.ChannelBufferUsage["tx_notify"].Set(float64(len(m.txNotifyCh)))
	}

	// Update goroutine count
	m.metrics.GoroutineCount.Set(float64(runtime.NumGoroutine()))

}

// recordError records an error in the appropriate metrics
func (m *Manager) recordError(errorType string, recoverable bool) {
	if m.metrics.ErrorsByType[errorType] != nil {
		m.metrics.ErrorsByType[errorType].Add(1)
	}

	if recoverable {
		m.metrics.RecoverableErrors.Add(1)
	} else {
		m.metrics.NonRecoverableErrors.Add(1)
	}
}

// recordDAMetrics records DA-related metrics
func (m *Manager) recordDAMetrics(operation string, success bool) {
	switch operation {
	case "submission":
		m.metrics.DASubmissionAttempts.Add(1)
		if success {
			m.metrics.DASubmissionSuccesses.Add(1)
		} else {
			m.metrics.DASubmissionFailures.Add(1)
		}
	case "retrieval":
		m.metrics.DARetrievalAttempts.Add(1)
		if success {
			m.metrics.DARetrievalSuccesses.Add(1)
		} else {
			m.metrics.DARetrievalFailures.Add(1)
		}
	}
}

// recordBlockProductionMetrics records block production metrics
func (m *Manager) recordBlockProductionMetrics(txCount int, isLazy bool, duration time.Duration) {
	// Record production time
	m.metrics.BlockProductionTime.Observe(duration.Seconds())

	// Record transactions per block
	m.metrics.TxsPerBlock.Observe(float64(txCount))

	// Record block type
	if txCount == 0 {
		m.metrics.EmptyBlocksProduced.Add(1)
	}

	if isLazy {
		m.metrics.LazyBlocksProduced.Add(1)
	} else {
		m.metrics.NormalBlocksProduced.Add(1)
	}

}

// recordSyncMetrics records synchronization metrics
func (m *Manager) recordSyncMetrics(operation string) {
	switch operation {
	case "header_synced":
		m.metrics.HeadersSynced.Add(1)
	case "data_synced":
		m.metrics.DataSynced.Add(1)
	case "block_applied":
		m.metrics.BlocksApplied.Add(1)
	case "invalid_header":
		m.metrics.InvalidHeadersCount.Add(1)
	}
}

// updatePendingMetrics updates pending counts for headers and data
func (m *Manager) updatePendingMetrics() {

	if m.pendingHeaders != nil {
		m.metrics.PendingHeadersCount.Set(float64(m.pendingHeaders.numPendingHeaders()))
	}
	if m.pendingData != nil {
		m.metrics.PendingDataCount.Set(float64(m.pendingData.numPendingData()))
	}
	// Update DA inclusion height
	m.metrics.DAInclusionHeight.Set(float64(m.daIncludedHeight.Load()))
}
