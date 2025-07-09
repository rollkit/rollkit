package block

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// OverflowStrategy defines how to handle channel overflow
type OverflowStrategy int

const (
	// OverflowStrategyDrop drops the message if channel is full
	OverflowStrategyDrop OverflowStrategy = iota
	// OverflowStrategyBlock blocks until channel has space
	OverflowStrategyBlock
	// OverflowStrategyRing uses a ring buffer, dropping oldest messages
	OverflowStrategyRing
)

// ChannelMetrics holds metrics for a single channel
type ChannelMetrics struct {
	SendCount       prometheus.Counter
	ReceiveCount    prometheus.Counter
	DroppedCount    prometheus.Counter
	BufferUsage     prometheus.GaugeFunc
	TimeoutCount    prometheus.Counter
	SendDuration    prometheus.Histogram
	ReceiveDuration prometheus.Histogram
}

// SignalChannel provides a safe wrapper for signal channels (size 1)
type SignalChannel struct {
	ch      chan struct{}
	name    string
	metrics *ChannelMetrics
	closed  bool
	mu      sync.RWMutex
}

// NewSignalChannel creates a new signal channel
func NewSignalChannel(name string, metrics *ChannelMetrics) *SignalChannel {
	return &SignalChannel{
		ch:      make(chan struct{}, 1),
		name:    name,
		metrics: metrics,
	}
}

// Send sends a signal with optional timeout
func (s *SignalChannel) Send(ctx context.Context) error {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return fmt.Errorf("channel %s is closed", s.name)
	}
	ch := s.ch
	s.mu.RUnlock()

	start := time.Now()
	defer func() {
		if s.metrics != nil && s.metrics.SendDuration != nil {
			s.metrics.SendDuration.Observe(time.Since(start).Seconds())
		}
	}()

	select {
	case ch <- struct{}{}:
		if s.metrics != nil && s.metrics.SendCount != nil {
			s.metrics.SendCount.Inc()
		}
		return nil
	case <-ctx.Done():
		if s.metrics != nil && s.metrics.TimeoutCount != nil {
			s.metrics.TimeoutCount.Inc()
		}
		return fmt.Errorf("send timeout on channel %s: %w", s.name, ctx.Err())
	}
}

// SendNonBlocking sends a signal without blocking
func (s *SignalChannel) SendNonBlocking() bool {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return false
	}
	ch := s.ch
	s.mu.RUnlock()

	select {
	case ch <- struct{}{}:
		if s.metrics != nil && s.metrics.SendCount != nil {
			s.metrics.SendCount.Inc()
		}
		return true
	default:
		if s.metrics != nil && s.metrics.DroppedCount != nil {
			s.metrics.DroppedCount.Inc()
		}
		return false
	}
}

// Receive receives a signal with timeout
func (s *SignalChannel) Receive(ctx context.Context) error {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return fmt.Errorf("channel %s is closed", s.name)
	}
	ch := s.ch
	s.mu.RUnlock()

	start := time.Now()
	defer func() {
		if s.metrics != nil && s.metrics.ReceiveDuration != nil {
			s.metrics.ReceiveDuration.Observe(time.Since(start).Seconds())
		}
	}()

	select {
	case <-ch:
		if s.metrics != nil && s.metrics.ReceiveCount != nil {
			s.metrics.ReceiveCount.Inc()
		}
		return nil
	case <-ctx.Done():
		if s.metrics != nil && s.metrics.TimeoutCount != nil {
			s.metrics.TimeoutCount.Inc()
		}
		return fmt.Errorf("receive timeout on channel %s: %w", s.name, ctx.Err())
	}
}

// Close closes the channel
func (s *SignalChannel) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		close(s.ch)
		s.closed = true
	}
}

// IsClosed returns whether the channel is closed
func (s *SignalChannel) IsClosed() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.closed
}

// Ch returns the underlying channel for use in select statements
// This should only be used when necessary for backward compatibility
func (s *SignalChannel) Ch() <-chan struct{} {
	return s.ch
}

// Len returns the number of signals in the channel
func (s *SignalChannel) Len() int {
	return len(s.ch)
}

// EventChannel provides a safe wrapper for event channels with generic type support
type EventChannel[T any] struct {
	ch       chan T
	name     string
	metrics  *ChannelMetrics
	capacity int
	overflow OverflowStrategy
	closed   bool
	mu       sync.RWMutex

	// Ring buffer for OverflowStrategyRing
	ring     []T
	ringHead int
	ringTail int
	ringSize int
	ringMu   sync.Mutex
}

// NewEventChannel creates a new event channel
func NewEventChannel[T any](name string, capacity int, overflow OverflowStrategy, metrics *ChannelMetrics) *EventChannel[T] {
	ec := &EventChannel[T]{
		ch:       make(chan T, capacity),
		name:     name,
		metrics:  metrics,
		capacity: capacity,
		overflow: overflow,
	}

	if overflow == OverflowStrategyRing {
		ec.ring = make([]T, capacity*2) // Double buffer for ring
		ec.ringSize = capacity * 2
	}

	return ec
}

// Send sends an event with the configured overflow strategy
func (ec *EventChannel[T]) Send(ctx context.Context, event T) error {
	ec.mu.RLock()
	if ec.closed {
		ec.mu.RUnlock()
		return fmt.Errorf("channel %s is closed", ec.name)
	}
	ch := ec.ch
	ec.mu.RUnlock()

	start := time.Now()
	defer func() {
		if ec.metrics != nil && ec.metrics.SendDuration != nil {
			ec.metrics.SendDuration.Observe(time.Since(start).Seconds())
		}
	}()

	switch ec.overflow {
	case OverflowStrategyDrop:
		select {
		case ch <- event:
			if ec.metrics != nil && ec.metrics.SendCount != nil {
				ec.metrics.SendCount.Inc()
			}
			return nil
		default:
			if ec.metrics != nil && ec.metrics.DroppedCount != nil {
				ec.metrics.DroppedCount.Inc()
			}
			return fmt.Errorf("channel %s is full, dropping event", ec.name)
		}

	case OverflowStrategyBlock:
		select {
		case ch <- event:
			if ec.metrics != nil && ec.metrics.SendCount != nil {
				ec.metrics.SendCount.Inc()
			}
			return nil
		case <-ctx.Done():
			if ec.metrics != nil && ec.metrics.TimeoutCount != nil {
				ec.metrics.TimeoutCount.Inc()
			}
			return fmt.Errorf("send timeout on channel %s: %w", ec.name, ctx.Err())
		}

	case OverflowStrategyRing:
		// Try to send directly first
		select {
		case ch <- event:
			if ec.metrics != nil && ec.metrics.SendCount != nil {
				ec.metrics.SendCount.Inc()
			}
			return nil
		default:
			// Channel is full, add to ring buffer
			ec.ringMu.Lock()
			ec.ring[ec.ringTail] = event
			ec.ringTail = (ec.ringTail + 1) % ec.ringSize
			if ec.ringTail == ec.ringHead {
				// Ring is full, drop oldest
				ec.ringHead = (ec.ringHead + 1) % ec.ringSize
				if ec.metrics != nil && ec.metrics.DroppedCount != nil {
					ec.metrics.DroppedCount.Inc()
				}
			}
			ec.ringMu.Unlock()

			if ec.metrics != nil && ec.metrics.SendCount != nil {
				ec.metrics.SendCount.Inc()
			}

			// Try to drain ring to channel
			ec.drainRingToChannel()
			return nil
		}

	default:
		return fmt.Errorf("unknown overflow strategy: %v", ec.overflow)
	}
}

// SendBatch sends multiple events as a batch
func (ec *EventChannel[T]) SendBatch(ctx context.Context, events []T) error {
	for _, event := range events {
		if err := ec.Send(ctx, event); err != nil {
			return fmt.Errorf("failed to send event in batch: %w", err)
		}
	}
	return nil
}

// Receive receives an event with timeout
func (ec *EventChannel[T]) Receive(ctx context.Context) (T, error) {
	var zero T

	ec.mu.RLock()
	if ec.closed {
		ec.mu.RUnlock()
		return zero, fmt.Errorf("channel %s is closed", ec.name)
	}
	ch := ec.ch
	ec.mu.RUnlock()

	start := time.Now()
	defer func() {
		if ec.metrics != nil && ec.metrics.ReceiveDuration != nil {
			ec.metrics.ReceiveDuration.Observe(time.Since(start).Seconds())
		}
	}()

	select {
	case event := <-ch:
		if ec.metrics != nil && ec.metrics.ReceiveCount != nil {
			ec.metrics.ReceiveCount.Inc()
		}

		// Try to refill from ring buffer if using ring strategy
		if ec.overflow == OverflowStrategyRing {
			ec.drainRingToChannel()
		}

		return event, nil
	case <-ctx.Done():
		if ec.metrics != nil && ec.metrics.TimeoutCount != nil {
			ec.metrics.TimeoutCount.Inc()
		}
		return zero, fmt.Errorf("receive timeout on channel %s: %w", ec.name, ctx.Err())
	}
}

// ReceiveBatch receives up to maxItems events without blocking
func (ec *EventChannel[T]) ReceiveBatch(maxItems int) []T {
	ec.mu.RLock()
	if ec.closed {
		ec.mu.RUnlock()
		return nil
	}
	ch := ec.ch
	ec.mu.RUnlock()

	events := make([]T, 0, maxItems)
	for i := 0; i < maxItems; i++ {
		select {
		case event := <-ch:
			events = append(events, event)
			if ec.metrics != nil && ec.metrics.ReceiveCount != nil {
				ec.metrics.ReceiveCount.Inc()
			}
		default:
			// No more events available
			break
		}
	}

	// Try to refill from ring buffer if using ring strategy
	if ec.overflow == OverflowStrategyRing && len(events) > 0 {
		ec.drainRingToChannel()
	}

	return events
}

// Close closes the channel
func (ec *EventChannel[T]) Close() {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	if !ec.closed {
		close(ec.ch)
		ec.closed = true
	}
}

// IsClosed returns whether the channel is closed
func (ec *EventChannel[T]) IsClosed() bool {
	ec.mu.RLock()
	defer ec.mu.RUnlock()
	return ec.closed
}

// Len returns the current number of events in the channel
func (ec *EventChannel[T]) Len() int {
	ec.mu.RLock()
	defer ec.mu.RUnlock()
	return len(ec.ch)
}

// Cap returns the channel capacity
func (ec *EventChannel[T]) Cap() int {
	return ec.capacity
}

// Ch returns the underlying channel for use in select statements
// This should only be used when necessary for backward compatibility
func (ec *EventChannel[T]) Ch() <-chan T {
	return ec.ch
}

// drainRingToChannel tries to move events from ring buffer to channel
func (ec *EventChannel[T]) drainRingToChannel() {
	if ec.overflow != OverflowStrategyRing {
		return
	}

	ec.ringMu.Lock()
	defer ec.ringMu.Unlock()

	for ec.ringHead != ec.ringTail {
		select {
		case ec.ch <- ec.ring[ec.ringHead]:
			ec.ringHead = (ec.ringHead + 1) % ec.ringSize
		default:
			// Channel is full again
			return
		}
	}
}

// ChannelHealth represents the health status of a channel
type ChannelHealth struct {
	Name         string
	BufferUsage  float64
	DroppedCount uint64
	TimeoutCount uint64
	IsHealthy    bool
	Issues       []string
}

// GetHealth returns the health status of the channel
func (s *SignalChannel) GetHealth() ChannelHealth {
	health := ChannelHealth{
		Name:      s.name,
		IsHealthy: true,
	}

	// Check if channel is closed
	if s.IsClosed() {
		health.IsHealthy = false
		health.Issues = append(health.Issues, "channel is closed")
	}

	// Check buffer usage
	s.mu.RLock()
	bufferUsage := float64(len(s.ch)) / float64(cap(s.ch))
	s.mu.RUnlock()

	health.BufferUsage = bufferUsage
	if bufferUsage > 0.8 {
		health.Issues = append(health.Issues, "high buffer usage")
	}

	return health
}

// GetHealth returns the health status of the event channel
func (ec *EventChannel[T]) GetHealth() ChannelHealth {
	health := ChannelHealth{
		Name:      ec.name,
		IsHealthy: true,
	}

	// Check if channel is closed
	if ec.IsClosed() {
		health.IsHealthy = false
		health.Issues = append(health.Issues, "channel is closed")
	}

	// Check buffer usage
	ec.mu.RLock()
	bufferUsage := float64(len(ec.ch)) / float64(cap(ec.ch))
	ec.mu.RUnlock()

	health.BufferUsage = bufferUsage
	if bufferUsage > 0.8 {
		health.Issues = append(health.Issues, "high buffer usage")
	}

	// Check ring buffer if applicable
	if ec.overflow == OverflowStrategyRing {
		ec.ringMu.Lock()
		ringUsage := 0
		if ec.ringTail >= ec.ringHead {
			ringUsage = ec.ringTail - ec.ringHead
		} else {
			ringUsage = ec.ringSize - ec.ringHead + ec.ringTail
		}
		ec.ringMu.Unlock()

		if ringUsage > ec.ringSize/2 {
			health.Issues = append(health.Issues, "high ring buffer usage")
		}
	}

	if len(health.Issues) > 0 {
		health.IsHealthy = false
	}

	return health
}
