package block

import (
	"context"
	"fmt"
	"sync"

	"github.com/rollkit/rollkit/events"
	"github.com/rollkit/rollkit/store"
	"github.com/rollkit/rollkit/types"
)

// StateManager is responsible for managing state transitions
type StateManager struct {
	eventBus     *events.Bus
	store        store.Store
	exec         coreexecutor.Executor
	stateMtx     *sync.RWMutex
	lastState    types.State
	logger       Logger
	metrics      *Metrics
}

// StateManagerOptions contains options for creating a new StateManager
type StateManagerOptions struct {
	EventBus     *events.Bus
	Store        store.Store
	Exec         coreexecutor.Executor
	Logger       Logger
	Metrics      *Metrics
	InitialState types.State
	StateMtx     *sync.RWMutex
}

// NewStateManager creates a new state manager
func NewStateManager(opts StateManagerOptions) *StateManager {
	return &StateManager{
		eventBus:     opts.EventBus,
		store:        opts.Store,
		exec:         opts.Exec,
		stateMtx:     opts.StateMtx,
		lastState:    opts.InitialState,
		logger:       opts.Logger,
		metrics:      opts.Metrics,
	}
}

// Start starts the state manager
func (sm *StateManager) Start(ctx context.Context) error {
	// Subscribe to events
	sm.eventBus.Subscribe(EventStateUpdated, sm.handleStateUpdated)

	return nil
}

// handleStateUpdated handles StateUpdatedEvent
func (sm *StateManager) handleStateUpdated(ctx context.Context, evt events.Event) {
	stateEvent, ok := evt.(StateUpdatedEvent)
	if !ok {
		sm.logger.Error("invalid event type", "expected", "StateUpdatedEvent", "got", fmt.Sprintf("%T", evt))
		return
	}

	// Update the stored state
	sm.updateState(ctx, stateEvent.State)
}

// updateState updates the state stored in the state manager and persists it to store
func (sm *StateManager) updateState(ctx context.Context, newState types.State) error {
	sm.logger.Debug("updating state", "newState", newState)
	
	sm.stateMtx.Lock()
	defer sm.stateMtx.Unlock()
	
	err := sm.store.UpdateState(ctx, newState)
	if err != nil {
		return err
	}
	
	sm.lastState = newState
	
	if sm.metrics != nil && newState.LastBlockHeight > 0 {
		sm.metrics.Height.Set(float64(newState.LastBlockHeight))
	}
	
	return nil
}

// GetLastState returns the current state
func (sm *StateManager) GetLastState() types.State {
	sm.stateMtx.RLock()
	defer sm.stateMtx.RUnlock()
	return sm.lastState
}
