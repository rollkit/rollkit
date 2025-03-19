package events

import (
	"context"
	"sync"
)

// Event represents a generic event in the system
type Event interface {
	Type() string
}

// Handler is a function that processes events
type Handler func(context.Context, Event)

// Bus is an in-memory event bus that distributes events to registered handlers
type Bus struct {
	handlers     map[string][]Handler
	handlersMu   sync.RWMutex
	ctx          context.Context
	cancel       context.CancelFunc
	eventChannel chan Event
	wg           sync.WaitGroup
	logger       Logger
}

// Logger interface for the event bus
type Logger interface {
	Debug(msg string, keyvals ...interface{})
	Info(msg string, keyvals ...interface{})
	Error(msg string, keyvals ...interface{})
}

// NewEventBus creates a new event bus
func NewEventBus(parentCtx context.Context, logger Logger) *Bus {
	ctx, cancel := context.WithCancel(parentCtx)

	bus := &Bus{
		handlers:     make(map[string][]Handler),
		ctx:          ctx,
		cancel:       cancel,
		eventChannel: make(chan Event, 100), // Buffer size can be configured
		logger:       logger,
	}

	// Start the event dispatcher
	bus.wg.Add(1)
	go bus.dispatch()

	return bus
}

// Subscribe registers a handler for a specific event type
func (b *Bus) Subscribe(eventType string, handler Handler) {
	b.handlersMu.Lock()
	defer b.handlersMu.Unlock()

	b.handlers[eventType] = append(b.handlers[eventType], handler)
	b.logger.Debug("Subscribed to event", "type", eventType)
}

// Publish sends an event to all subscribed handlers
func (b *Bus) Publish(event Event) {
	select {
	case b.eventChannel <- event:
		b.logger.Debug("Published event", "type", event.Type())
	case <-b.ctx.Done():
		b.logger.Debug("Cannot publish event: bus is shutting down", "type", event.Type())
	default:
		b.logger.Error("Event channel full, dropping event", "type", event.Type())
	}
}

// dispatch processes events from the channel and routes them to handlers
func (b *Bus) dispatch() {
	defer b.wg.Done()

	for {
		select {
		case event := <-b.eventChannel:
			b.handleEvent(event)
		case <-b.ctx.Done():
			b.logger.Info("Event bus dispatcher shutting down")
			return
		}
	}
}

// handleEvent sends an event to all registered handlers for its type
func (b *Bus) handleEvent(event Event) {
	eventType := event.Type()

	b.handlersMu.RLock()
	handlers := b.handlers[eventType]
	b.handlersMu.RUnlock()

	for _, handler := range handlers {
		// Create a copy of the handler to avoid closure issues
		h := handler
		b.wg.Add(1)
		go func(evt Event) {
			defer b.wg.Done()
			h(b.ctx, evt)
		}(event)
	}
}

// Shutdown stops the event bus
func (b *Bus) Shutdown() {
	b.cancel()
	b.wg.Wait()
	b.logger.Info("Event bus shutdown complete")
}
