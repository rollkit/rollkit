package block

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignalChannel_SendReceive(t *testing.T) {
	ch := NewSignalChannel("test", nil)
	defer ch.Close()

	ctx := context.Background()

	// Test successful send and receive
	err := ch.Send(ctx)
	require.NoError(t, err)

	err = ch.Receive(ctx)
	require.NoError(t, err)
}

func TestSignalChannel_SendNonBlocking(t *testing.T) {
	ch := NewSignalChannel("test", nil)
	defer ch.Close()

	// First send should succeed
	sent := ch.SendNonBlocking()
	assert.True(t, sent)

	// Second send should fail (channel is full)
	sent = ch.SendNonBlocking()
	assert.False(t, sent)

	// Receive to make space
	ctx := context.Background()
	err := ch.Receive(ctx)
	require.NoError(t, err)

	// Now send should succeed again
	sent = ch.SendNonBlocking()
	assert.True(t, sent)
}

func TestSignalChannel_Timeout(t *testing.T) {
	ch := NewSignalChannel("test", nil)
	defer ch.Close()

	// Test receive timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := ch.Receive(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")

	// Fill the channel
	err = ch.Send(context.Background())
	require.NoError(t, err)

	// Test send timeout
	ctx2, cancel2 := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel2()

	err = ch.Send(ctx2)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
}

func TestSignalChannel_Close(t *testing.T) {
	ch := NewSignalChannel("test", nil)

	// Close the channel
	ch.Close()
	assert.True(t, ch.IsClosed())

	// Operations on closed channel should fail
	err := ch.Send(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "closed")

	err = ch.Receive(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "closed")

	sent := ch.SendNonBlocking()
	assert.False(t, sent)
}

func TestEventChannel_SendReceive(t *testing.T) {
	ch := NewEventChannel[int]("test", 10, OverflowStrategyBlock, nil)
	defer ch.Close()

	ctx := context.Background()

	// Send and receive single event
	err := ch.Send(ctx, 42)
	require.NoError(t, err)

	val, err := ch.Receive(ctx)
	require.NoError(t, err)
	assert.Equal(t, 42, val)
}

func TestEventChannel_SendBatch(t *testing.T) {
	ch := NewEventChannel[int]("test", 10, OverflowStrategyBlock, nil)
	defer ch.Close()

	ctx := context.Background()

	// Send batch
	events := []int{1, 2, 3, 4, 5}
	err := ch.SendBatch(ctx, events)
	require.NoError(t, err)

	// Receive batch
	received := ch.ReceiveBatch(10)
	assert.Equal(t, events, received)
}

func TestEventChannel_OverflowStrategyDrop(t *testing.T) {
	ch := NewEventChannel[int]("test", 2, OverflowStrategyDrop, nil)
	defer ch.Close()

	ctx := context.Background()

	// Fill the channel
	err := ch.Send(ctx, 1)
	require.NoError(t, err)
	err = ch.Send(ctx, 2)
	require.NoError(t, err)

	// Next send should drop
	err = ch.Send(ctx, 3)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "dropping")

	// Verify channel still has original values
	val1, _ := ch.Receive(ctx)
	val2, _ := ch.Receive(ctx)
	assert.Equal(t, 1, val1)
	assert.Equal(t, 2, val2)
}

func TestEventChannel_OverflowStrategyBlock(t *testing.T) {
	ch := NewEventChannel[int]("test", 2, OverflowStrategyBlock, nil)
	defer ch.Close()

	ctx := context.Background()

	// Fill the channel
	err := ch.Send(ctx, 1)
	require.NoError(t, err)
	err = ch.Send(ctx, 2)
	require.NoError(t, err)

	// Next send should block until timeout
	ctx2, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err = ch.Send(ctx2, 3)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
}

func TestEventChannel_OverflowStrategyRing(t *testing.T) {
	ch := NewEventChannel[int]("test", 2, OverflowStrategyRing, nil)
	defer ch.Close()

	ctx := context.Background()

	// Send more than capacity
	for i := 1; i <= 5; i++ {
		err := ch.Send(ctx, i)
		require.NoError(t, err)
	}

	// Should be able to receive the last values (ring buffer keeps recent)
	received := ch.ReceiveBatch(10)
	assert.GreaterOrEqual(t, len(received), 2)
}

func TestEventChannel_Close(t *testing.T) {
	ch := NewEventChannel[int]("test", 10, OverflowStrategyBlock, nil)

	// Send some events
	ctx := context.Background()
	err := ch.Send(ctx, 1)
	require.NoError(t, err)

	// Close the channel
	ch.Close()
	assert.True(t, ch.IsClosed())

	// Operations on closed channel should fail
	err = ch.Send(ctx, 2)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "closed")

	_, err = ch.Receive(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "closed")
}

func TestEventChannel_LenCap(t *testing.T) {
	ch := NewEventChannel[int]("test", 10, OverflowStrategyBlock, nil)
	defer ch.Close()

	assert.Equal(t, 0, ch.Len())
	assert.Equal(t, 10, ch.Cap())

	ctx := context.Background()
	err := ch.Send(ctx, 1)
	require.NoError(t, err)
	err = ch.Send(ctx, 2)
	require.NoError(t, err)

	assert.Equal(t, 2, ch.Len())
	assert.Equal(t, 10, ch.Cap())
}

func TestChannelHealth(t *testing.T) {
	// Test SignalChannel health
	sch := NewSignalChannel("test-signal", nil)
	health := sch.GetHealth()
	assert.Equal(t, "test-signal", health.Name)
	assert.True(t, health.IsHealthy)
	assert.Empty(t, health.Issues)

	// Fill the channel
	sch.SendNonBlocking()
	health = sch.GetHealth()
	assert.Equal(t, 1.0, health.BufferUsage)
	assert.Contains(t, health.Issues, "high buffer usage")

	sch.Close()
	health = sch.GetHealth()
	assert.False(t, health.IsHealthy)
	assert.Contains(t, health.Issues, "channel is closed")

	// Test EventChannel health
	ech := NewEventChannel[int]("test-event", 10, OverflowStrategyBlock, nil)
	health = ech.GetHealth()
	assert.Equal(t, "test-event", health.Name)
	assert.True(t, health.IsHealthy)
	assert.Empty(t, health.Issues)

	// Fill to 90%
	for i := 0; i < 9; i++ {
		_ = ech.Send(context.Background(), i)
	}
	health = ech.GetHealth()
	assert.Equal(t, 0.9, health.BufferUsage)
	assert.Contains(t, health.Issues, "high buffer usage")
	assert.False(t, health.IsHealthy)

	ech.Close()
}

func TestChannelManager(t *testing.T) {
	logger := logging.Logger("test")
	_ = logging.SetLogLevel("test", "DEBUG")

	reg := prometheus.NewRegistry()
	config := DefaultChannelManagerConfig()
	config.MetricsRegistry = reg
	config.HealthCheckInterval = 100 * time.Millisecond

	cm := NewChannelManager(logger, config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cm.Start(ctx)
	defer cm.Stop()

	// Test getting channels
	assert.NotNil(t, cm.HeaderStoreCh())
	assert.NotNil(t, cm.DataStoreCh())
	assert.NotNil(t, cm.RetrieveCh())
	assert.NotNil(t, cm.DAIncluderCh())
	assert.NotNil(t, cm.TxNotifyCh())
	assert.NotNil(t, cm.HeaderInCh())
	assert.NotNil(t, cm.DataInCh())

	// Test sending signal
	err := cm.SendSignal(ctx, "headerStore", 2)
	require.NoError(t, err)

	// Test getting non-existent channel
	_, err = cm.GetSignalChannel("nonexistent")
	assert.Error(t, err)

	// Test channel health
	health := cm.GetChannelHealth()
	assert.Len(t, health, 7) // 5 signal + 2 event channels

	// Test draining channel
	cm.HeaderStoreCh().SendNonBlocking()
	count := cm.DrainChannel("headerStore")
	assert.Equal(t, 1, count)

	// Wait for health check to run
	time.Sleep(150 * time.Millisecond)
}

func TestChannelConcurrency(t *testing.T) {
	ch := NewEventChannel[int]("test", 100, OverflowStrategyBlock, nil)
	defer ch.Close()

	ctx := context.Background()
	var wg sync.WaitGroup
	const numGoroutines = 10
	const numOperations = 100

	// Concurrent sends
	var sendCount atomic.Int32
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				if err := ch.Send(ctx, id*numOperations+j); err == nil {
					sendCount.Add(1)
				}
			}
		}(i)
	}

	// Concurrent receives
	var receiveCount atomic.Int32
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				if _, err := ch.Receive(ctx); err == nil {
					receiveCount.Add(1)
				}
			}
		}()
	}

	wg.Wait()

	// Verify all operations completed
	assert.Equal(t, int32(numGoroutines*numOperations), sendCount.Load())
	assert.Equal(t, int32(numGoroutines*numOperations), receiveCount.Load())
}

func TestChannelMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()

	// Create channel metrics
	metrics := &ChannelMetrics{
		SendCount: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "test_send_total",
		}),
		ReceiveCount: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "test_receive_total",
		}),
		DroppedCount: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "test_dropped_total",
		}),
		TimeoutCount: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "test_timeout_total",
		}),
		SendDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "test_send_duration_seconds",
		}),
		ReceiveDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "test_receive_duration_seconds",
		}),
	}

	reg.MustRegister(
		metrics.SendCount,
		metrics.ReceiveCount,
		metrics.DroppedCount,
		metrics.TimeoutCount,
		metrics.SendDuration,
		metrics.ReceiveDuration,
	)

	// Test with metrics
	ch := NewSignalChannel("test", metrics)
	defer ch.Close()

	ctx := context.Background()

	// Send and receive
	err := ch.Send(ctx)
	require.NoError(t, err)

	err = ch.Receive(ctx)
	require.NoError(t, err)

	// Non-blocking send that drops
	ch.SendNonBlocking() // fills channel
	ch.SendNonBlocking() // should drop

	// Timeout
	ctx2, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()
	_ = ch.Receive(ctx2) // should timeout

	// Gather metrics
	metricFamilies, err := reg.Gather()
	require.NoError(t, err)

	// Verify metrics were recorded
	metricsFound := make(map[string]bool)
	for _, mf := range metricFamilies {
		metricsFound[*mf.Name] = true
	}

	assert.True(t, metricsFound["test_send_total"])
	assert.True(t, metricsFound["test_receive_total"])
	assert.True(t, metricsFound["test_dropped_total"])
	assert.True(t, metricsFound["test_timeout_total"])
}

func BenchmarkSignalChannel_SendReceive(b *testing.B) {
	ch := NewSignalChannel("bench", nil)
	defer ch.Close()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ch.Send(ctx)
		_ = ch.Receive(ctx)
	}
}

func BenchmarkEventChannel_SendReceive(b *testing.B) {
	ch := NewEventChannel[int]("bench", 1000, OverflowStrategyBlock, nil)
	defer ch.Close()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ch.Send(ctx, i)
		_, _ = ch.Receive(ctx)
	}
}

func BenchmarkEventChannel_BatchOperations(b *testing.B) {
	ch := NewEventChannel[int]("bench", 1000, OverflowStrategyBlock, nil)
	defer ch.Close()

	ctx := context.Background()
	batch := make([]int, 100)
	for i := range batch {
		batch[i] = i
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ch.SendBatch(ctx, batch)
		_ = ch.ReceiveBatch(100)
	}
}
