package block

import (
	"context"
	"fmt"
	"sync"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"github.com/prometheus/client_golang/prometheus"
)

// ChannelManager manages the lifecycle of all channels in the block package
type ChannelManager struct {
	logger   logging.EventLogger
	registry *prometheus.Registry

	// Signal channels
	headerStoreCh *SignalChannel
	dataStoreCh   *SignalChannel
	retrieveCh    *SignalChannel
	daIncluderCh  *SignalChannel
	txNotifyCh    *SignalChannel

	// Event channels
	headerInCh *EventChannel[NewHeaderEvent]
	dataInCh   *EventChannel[NewDataEvent]

	// Channel registry for health monitoring
	channels     []interface{} // Can be SignalChannel or EventChannel
	channelNames map[string]interface{}
	mu           sync.RWMutex

	// Health monitoring
	healthCheckInterval time.Duration
	stopHealthCheck     chan struct{}
	wg                  sync.WaitGroup
}

// ChannelManagerConfig holds configuration for the channel manager
type ChannelManagerConfig struct {
	EventChannelSize    int
	HealthCheckInterval time.Duration
	MetricsRegistry     *prometheus.Registry
}

// DefaultChannelManagerConfig returns default configuration
func DefaultChannelManagerConfig() ChannelManagerConfig {
	return ChannelManagerConfig{
		EventChannelSize:    10000,
		HealthCheckInterval: 30 * time.Second,
	}
}

// NewChannelManager creates a new channel manager
func NewChannelManager(logger logging.EventLogger, config ChannelManagerConfig) *ChannelManager {
	cm := &ChannelManager{
		logger:              logger,
		registry:            config.MetricsRegistry,
		channelNames:        make(map[string]interface{}),
		healthCheckInterval: config.HealthCheckInterval,
		stopHealthCheck:     make(chan struct{}),
	}

	// Create signal channels
	cm.headerStoreCh = cm.createSignalChannel("headerStore")
	cm.dataStoreCh = cm.createSignalChannel("dataStore")
	cm.retrieveCh = cm.createSignalChannel("retrieve")
	cm.daIncluderCh = cm.createSignalChannel("daIncluder")
	cm.txNotifyCh = cm.createSignalChannel("txNotify")

	// Create event channels
	cm.headerInCh = cm.createHeaderEventChannel("headerIn", config.EventChannelSize, OverflowStrategyDrop)
	cm.dataInCh = cm.createDataEventChannel("dataIn", config.EventChannelSize, OverflowStrategyDrop)

	return cm
}

// createSignalChannel creates a new signal channel with metrics
func (cm *ChannelManager) createSignalChannel(name string) *SignalChannel {
	metrics := cm.createChannelMetrics(name, 1)
	ch := NewSignalChannel(name, metrics)

	cm.mu.Lock()
	cm.channels = append(cm.channels, ch)
	cm.channelNames[name] = ch
	cm.mu.Unlock()

	return ch
}

// createHeaderEventChannel creates a new event channel for header events with metrics
func (cm *ChannelManager) createHeaderEventChannel(name string, capacity int, overflow OverflowStrategy) *EventChannel[NewHeaderEvent] {
	metrics := cm.createChannelMetrics(name, capacity)
	ch := NewEventChannel[NewHeaderEvent](name, capacity, overflow, metrics)
	
	cm.mu.Lock()
	cm.channels = append(cm.channels, ch)
	cm.channelNames[name] = ch
	cm.mu.Unlock()
	
	return ch
}

// createDataEventChannel creates a new event channel for data events with metrics
func (cm *ChannelManager) createDataEventChannel(name string, capacity int, overflow OverflowStrategy) *EventChannel[NewDataEvent] {
	metrics := cm.createChannelMetrics(name, capacity)
	ch := NewEventChannel[NewDataEvent](name, capacity, overflow, metrics)
	
	cm.mu.Lock()
	cm.channels = append(cm.channels, ch)
	cm.channelNames[name] = ch
	cm.mu.Unlock()
	
	return ch
}

// createChannelMetrics creates metrics for a channel
func (cm *ChannelManager) createChannelMetrics(name string, capacity int) *ChannelMetrics {
	if cm.registry == nil {
		return nil
	}

	metrics := &ChannelMetrics{
		SendCount: prometheus.NewCounter(prometheus.CounterOpts{
			Name: fmt.Sprintf("rollkit_block_channel_%s_send_total", name),
			Help: fmt.Sprintf("Total number of sends to %s channel", name),
		}),
		ReceiveCount: prometheus.NewCounter(prometheus.CounterOpts{
			Name: fmt.Sprintf("rollkit_block_channel_%s_receive_total", name),
			Help: fmt.Sprintf("Total number of receives from %s channel", name),
		}),
		DroppedCount: prometheus.NewCounter(prometheus.CounterOpts{
			Name: fmt.Sprintf("rollkit_block_channel_%s_dropped_total", name),
			Help: fmt.Sprintf("Total number of dropped messages in %s channel", name),
		}),
		TimeoutCount: prometheus.NewCounter(prometheus.CounterOpts{
			Name: fmt.Sprintf("rollkit_block_channel_%s_timeout_total", name),
			Help: fmt.Sprintf("Total number of timeouts in %s channel", name),
		}),
		SendDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    fmt.Sprintf("rollkit_block_channel_%s_send_duration_seconds", name),
			Help:    fmt.Sprintf("Send operation duration for %s channel", name),
			Buckets: prometheus.ExponentialBuckets(0.0001, 2, 10),
		}),
		ReceiveDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    fmt.Sprintf("rollkit_block_channel_%s_receive_duration_seconds", name),
			Help:    fmt.Sprintf("Receive operation duration for %s channel", name),
			Buckets: prometheus.ExponentialBuckets(0.0001, 2, 10),
		}),
	}

	// Register metrics
	cm.registry.MustRegister(
		metrics.SendCount,
		metrics.ReceiveCount,
		metrics.DroppedCount,
		metrics.TimeoutCount,
		metrics.SendDuration,
		metrics.ReceiveDuration,
	)

	// Add buffer usage gauge
	metrics.BufferUsage = prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: fmt.Sprintf("rollkit_block_channel_%s_buffer_usage_ratio", name),
		Help: fmt.Sprintf("Buffer usage ratio for %s channel", name),
	}, func() float64 {
		cm.mu.RLock()
		ch, exists := cm.channelNames[name]
		cm.mu.RUnlock()

		if !exists {
			return 0
		}

		switch c := ch.(type) {
		case *SignalChannel:
			c.mu.RLock()
			usage := float64(len(c.ch)) / float64(cap(c.ch))
			c.mu.RUnlock()
			return usage
		case *EventChannel[NewHeaderEvent]:
			return float64(c.Len()) / float64(c.Cap())
		case *EventChannel[NewDataEvent]:
			return float64(c.Len()) / float64(c.Cap())
		default:
			return 0
		}
	})

	cm.registry.MustRegister(metrics.BufferUsage)

	return metrics
}

// Start starts the channel manager's background tasks
func (cm *ChannelManager) Start(ctx context.Context) {
	cm.wg.Add(1)
	go cm.healthMonitor(ctx)
}

// Stop stops the channel manager and closes all channels
func (cm *ChannelManager) Stop() {
	close(cm.stopHealthCheck)
	cm.wg.Wait()

	// Close all channels
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.headerStoreCh.Close()
	cm.dataStoreCh.Close()
	cm.retrieveCh.Close()
	cm.daIncluderCh.Close()
	cm.txNotifyCh.Close()
	cm.headerInCh.Close()
	cm.dataInCh.Close()
}

// healthMonitor periodically checks channel health
func (cm *ChannelManager) healthMonitor(ctx context.Context) {
	defer cm.wg.Done()

	ticker := time.NewTicker(cm.healthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-cm.stopHealthCheck:
			return
		case <-ticker.C:
			cm.checkHealth()
		}
	}
}

// checkHealth checks the health of all channels
func (cm *ChannelManager) checkHealth() {
	cm.mu.RLock()
	channels := make([]interface{}, len(cm.channels))
	copy(channels, cm.channels)
	cm.mu.RUnlock()

	unhealthyChannels := []string{}

	for _, ch := range channels {
		var health ChannelHealth

		switch c := ch.(type) {
		case *SignalChannel:
			health = c.GetHealth()
		case *EventChannel[NewHeaderEvent]:
			health = c.GetHealth()
		case *EventChannel[NewDataEvent]:
			health = c.GetHealth()
		}

		if !health.IsHealthy {
			unhealthyChannels = append(unhealthyChannels, health.Name)
			cm.logger.Warn("unhealthy channel detected",
				"channel", health.Name,
				"bufferUsage", health.BufferUsage,
				"issues", health.Issues)
		}

		// Log high buffer usage even if healthy
		if health.BufferUsage > 0.5 {
			cm.logger.Debug("channel buffer usage high",
				"channel", health.Name,
				"bufferUsage", health.BufferUsage)
		}
	}

	if len(unhealthyChannels) > 0 {
		cm.logger.Error("unhealthy channels detected",
			"count", len(unhealthyChannels),
			"channels", unhealthyChannels)
	}
}

// GetChannelHealth returns health information for all channels
func (cm *ChannelManager) GetChannelHealth() map[string]ChannelHealth {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	health := make(map[string]ChannelHealth)

	for name, ch := range cm.channelNames {
		switch c := ch.(type) {
		case *SignalChannel:
			health[name] = c.GetHealth()
		case *EventChannel[NewHeaderEvent]:
			health[name] = c.GetHealth()
		case *EventChannel[NewDataEvent]:
			health[name] = c.GetHealth()
		}
	}

	return health
}

// GetSignalChannel returns a signal channel by name
func (cm *ChannelManager) GetSignalChannel(name string) (*SignalChannel, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	ch, exists := cm.channelNames[name]
	if !exists {
		return nil, fmt.Errorf("channel %s not found", name)
	}

	signalCh, ok := ch.(*SignalChannel)
	if !ok {
		return nil, fmt.Errorf("channel %s is not a signal channel", name)
	}

	return signalCh, nil
}

// Getters for specific channels

// HeaderStoreCh returns the header store signal channel
func (cm *ChannelManager) HeaderStoreCh() *SignalChannel {
	return cm.headerStoreCh
}

// DataStoreCh returns the data store signal channel
func (cm *ChannelManager) DataStoreCh() *SignalChannel {
	return cm.dataStoreCh
}

// RetrieveCh returns the retrieve signal channel
func (cm *ChannelManager) RetrieveCh() *SignalChannel {
	return cm.retrieveCh
}

// DAIncluderCh returns the DA includer signal channel
func (cm *ChannelManager) DAIncluderCh() *SignalChannel {
	return cm.daIncluderCh
}

// TxNotifyCh returns the transaction notify signal channel
func (cm *ChannelManager) TxNotifyCh() *SignalChannel {
	return cm.txNotifyCh
}

// HeaderInCh returns the header input event channel
func (cm *ChannelManager) HeaderInCh() *EventChannel[NewHeaderEvent] {
	return cm.headerInCh
}

// DataInCh returns the data input event channel
func (cm *ChannelManager) DataInCh() *EventChannel[NewDataEvent] {
	return cm.dataInCh
}

// SendSignal sends a signal to the specified channel with retry logic
func (cm *ChannelManager) SendSignal(ctx context.Context, channelName string, retries int) error {
	ch, err := cm.GetSignalChannel(channelName)
	if err != nil {
		return err
	}

	var lastErr error
	for i := 0; i <= retries; i++ {
		if err := ch.Send(ctx); err != nil {
			lastErr = err
			if i < retries {
				cm.logger.Debug("retrying signal send",
					"channel", channelName,
					"attempt", i+1,
					"error", err)
				// Small backoff between retries
				select {
				case <-time.After(time.Millisecond * 10 * time.Duration(i+1)):
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		} else {
			return nil
		}
	}

	return fmt.Errorf("failed to send signal after %d retries: %w", retries, lastErr)
}

// DrainChannel drains all pending events from a channel
func (cm *ChannelManager) DrainChannel(channelName string) int {
	cm.mu.RLock()
	ch, exists := cm.channelNames[channelName]
	cm.mu.RUnlock()

	if !exists {
		return 0
	}

	count := 0
	switch c := ch.(type) {
	case *SignalChannel:
		// Drain signal channel
		for {
			ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
			err := c.Receive(ctx)
			cancel()
			if err != nil {
				break
			}
			count++
		}
	case *EventChannel[NewHeaderEvent]:
		events := c.ReceiveBatch(c.Len())
		count = len(events)
	case *EventChannel[NewDataEvent]:
		events := c.ReceiveBatch(c.Len())
		count = len(events)
	}

	if count > 0 {
		cm.logger.Info("drained channel",
			"channel", channelName,
			"count", count)
	}

	return count
}
