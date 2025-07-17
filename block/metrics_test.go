package block

import (
	"testing"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetrics(t *testing.T) {
	t.Run("PrometheusMetrics", func(t *testing.T) {
		em := PrometheusMetrics("test", "chain_id", "test_chain")

		// Test that base metrics are initialized
		assert.NotNil(t, em.Height)
		assert.NotNil(t, em.NumTxs)

		// Test channel metrics initialization
		assert.Len(t, em.ChannelBufferUsage, 7)
		assert.NotNil(t, em.ChannelBufferUsage["header_in"])
		assert.NotNil(t, em.ChannelBufferUsage["data_in"])
		assert.NotNil(t, em.DroppedSignals)

		// Test error metrics initialization
		assert.Len(t, em.ErrorsByType, 5)
		assert.NotNil(t, em.ErrorsByType["block_production"])
		assert.NotNil(t, em.RecoverableErrors)
		assert.NotNil(t, em.NonRecoverableErrors)

		// Test performance metrics initialization
		assert.Len(t, em.OperationDuration, 5)
		assert.NotNil(t, em.OperationDuration["block_production"])
		assert.NotNil(t, em.GoroutineCount)

		// Test DA metrics initialization
		assert.NotNil(t, em.DASubmissionAttempts)
		assert.NotNil(t, em.DASubmissionSuccesses)
		assert.NotNil(t, em.DASubmissionFailures)
		assert.NotNil(t, em.DARetrievalAttempts)
		assert.NotNil(t, em.DARetrievalSuccesses)
		assert.NotNil(t, em.DARetrievalFailures)
		assert.NotNil(t, em.DAInclusionHeight)
		assert.NotNil(t, em.PendingHeadersCount)
		assert.NotNil(t, em.PendingDataCount)

		// Test sync metrics initialization
		assert.NotNil(t, em.SyncLag)
		assert.NotNil(t, em.HeadersSynced)
		assert.NotNil(t, em.DataSynced)
		assert.NotNil(t, em.BlocksApplied)
		assert.NotNil(t, em.InvalidHeadersCount)

		// Test block production metrics initialization
		assert.NotNil(t, em.BlockProductionTime)
		assert.NotNil(t, em.EmptyBlocksProduced)
		assert.NotNil(t, em.LazyBlocksProduced)
		assert.NotNil(t, em.NormalBlocksProduced)
		assert.NotNil(t, em.TxsPerBlock)

		// Test state transition metrics initialization
		assert.Len(t, em.StateTransitions, 3)
		assert.NotNil(t, em.StateTransitions["pending_to_submitted"])
		assert.NotNil(t, em.InvalidTransitions)
	})

	t.Run("NopMetrics", func(t *testing.T) {
		em := NopMetrics()

		// Test that all metrics are initialized with no-op implementations
		assert.NotNil(t, em.DroppedSignals)
		assert.NotNil(t, em.RecoverableErrors)
		assert.NotNil(t, em.NonRecoverableErrors)

		// Test maps are initialized
		assert.Len(t, em.ChannelBufferUsage, 7)
		assert.Len(t, em.ErrorsByType, 5)
		assert.Len(t, em.OperationDuration, 5)
		assert.Len(t, em.StateTransitions, 3)

		// Verify no-op metrics don't panic when used
		em.DroppedSignals.Add(1)
		em.RecoverableErrors.Add(1)
		em.GoroutineCount.Set(100)
		em.BlockProductionTime.Observe(0.5)
	})
}

func TestMetricsTimer(t *testing.T) {
	em := NopMetrics()

	timer := NewMetricsTimer("block_production", em)
	require.NotNil(t, timer)

	// Simulate some work
	time.Sleep(10 * time.Millisecond)

	// Stop should not panic
	timer.Stop()
}

func TestMetricsHelpers(t *testing.T) {
	// Create a test manager with extended metrics
	m := &Manager{
		metrics:       NopMetrics(),
		logger:        logging.Logger("test"),
		headerInCh:    make(chan NewHeaderEvent, 10),
		dataInCh:      make(chan NewDataEvent, 10),
		headerStoreCh: make(chan struct{}, 1),
		dataStoreCh:   make(chan struct{}, 1),
		retrieveCh:    make(chan struct{}, 1),
		daIncluderCh:  make(chan struct{}, 1),
		txNotifyCh:    make(chan struct{}, 1),
	}

	t.Run("sendNonBlockingSignalWithMetrics", func(t *testing.T) {
		// Test successful send
		ch := make(chan struct{}, 1)
		sent := m.sendNonBlockingSignalWithMetrics(ch, "test_channel")
		assert.True(t, sent)

		// Test dropped signal
		sent = m.sendNonBlockingSignalWithMetrics(ch, "test_channel")
		assert.False(t, sent)
	})

	t.Run("updateChannelMetrics", func(t *testing.T) {
		// Add some data to channels
		m.headerInCh <- NewHeaderEvent{}
		m.dataInCh <- NewDataEvent{}

		// Should not panic
		m.updateChannelMetrics()
	})

	t.Run("recordError", func(t *testing.T) {
		// Should not panic
		m.recordError("block_production", true)
		m.recordError("da_submission", false)
	})

	t.Run("recordDAMetrics", func(t *testing.T) {
		// Should not panic with three modes: retry, success, fail
		m.recordDAMetrics("submission", DAModeRetry)
		m.recordDAMetrics("submission", DAModeSuccess)
		m.recordDAMetrics("submission", DAModeFail)
		m.recordDAMetrics("retrieval", DAModeRetry)
		m.recordDAMetrics("retrieval", DAModeSuccess)
		m.recordDAMetrics("retrieval", DAModeFail)
	})

	t.Run("recordBlockProductionMetrics", func(t *testing.T) {
		// Should not panic
		m.recordBlockProductionMetrics(10, true, 100*time.Millisecond)
		m.recordBlockProductionMetrics(0, false, 50*time.Millisecond)
	})

	t.Run("recordSyncMetrics", func(t *testing.T) {
		// Should not panic
		m.recordSyncMetrics("header_synced")
		m.recordSyncMetrics("data_synced")
		m.recordSyncMetrics("block_applied")
		m.recordSyncMetrics("invalid_header")
	})
}

// Test integration with actual metrics recording
func TestMetricsIntegration(t *testing.T) {
	// This test verifies that metrics can be recorded without panics
	// when using Prometheus metrics
	em := PrometheusMetrics("test_integration")

	// Test various metric operations
	em.DroppedSignals.Add(1)
	em.RecoverableErrors.Add(1)
	em.NonRecoverableErrors.Add(1)
	em.GoroutineCount.Set(50)

	// Test channel metrics
	em.ChannelBufferUsage["header_in"].Set(5)
	em.ChannelBufferUsage["data_in"].Set(3)

	// Test error metrics
	em.ErrorsByType["block_production"].Add(1)
	em.ErrorsByType["da_submission"].Add(2)

	// Test operation duration
	em.OperationDuration["block_production"].Observe(0.05)
	em.OperationDuration["da_submission"].Observe(0.1)

	// Test DA metrics
	em.DASubmissionAttempts.Add(5)
	em.DASubmissionSuccesses.Add(4)
	em.DASubmissionFailures.Add(1)

	// Test sync metrics
	em.HeadersSynced.Add(10)
	em.DataSynced.Add(10)
	em.BlocksApplied.Add(10)

	// Test block production metrics
	em.BlockProductionTime.Observe(0.02)
	em.TxsPerBlock.Observe(100)
	em.EmptyBlocksProduced.Add(2)
	em.LazyBlocksProduced.Add(5)
	em.NormalBlocksProduced.Add(10)

	// Test state transitions
	em.StateTransitions["pending_to_submitted"].Add(15)
	em.InvalidTransitions.Add(0)
}
