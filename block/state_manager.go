package block

import (
	"context"
	"fmt"
	"sync"

	cmbytes "github.com/cometbft/cometbft/libs/bytes"
	cmtypes "github.com/cometbft/cometbft/types"

	coreexecutor "github.com/rollkit/rollkit/core/execution"
	"github.com/rollkit/rollkit/events"
	"github.com/rollkit/rollkit/store"
	"github.com/rollkit/rollkit/types"
)

// StateManager is responsible for managing state transitions
type StateManager struct {
	eventBus     *events.Bus
	store        store.Store
	exec         coreexecutor.Executor
	lastStateMtx *sync.RWMutex
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
}

// NewStateManager creates a new state manager
func NewStateManager(opts StateManagerOptions) *StateManager {
	return &StateManager{
		eventBus:     opts.EventBus,
		store:        opts.Store,
		exec:         opts.Exec,
		lastStateMtx: new(sync.RWMutex),
		lastState:    opts.InitialState,
		logger:       opts.Logger,
		metrics:      opts.Metrics,
	}
}

// Start starts the state manager
func (sm *StateManager) Start(ctx context.Context) error {
	// Subscribe to events
	sm.eventBus.Subscribe(EventBlockCreated, sm.handleBlockCreated)
	sm.eventBus.Subscribe(EventHeaderRetrieved, sm.handleHeaderRetrieved)
	sm.eventBus.Subscribe(EventDataRetrieved, sm.handleDataRetrieved)

	return nil
}

// handleBlockCreated handles BlockCreatedEvent
func (sm *StateManager) handleBlockCreated(ctx context.Context, evt events.Event) {
	blockEvent, ok := evt.(BlockCreatedEvent)
	if !ok {
		sm.logger.Error("invalid event type", "expected", "BlockCreatedEvent", "got", fmt.Sprintf("%T", evt))
		return
	}

	header := blockEvent.Header
	data := blockEvent.Data

	// Apply block to update state
	newState, err := sm.applyBlock(ctx, header, data)
	if err != nil {
		sm.logger.Error("failed to apply block", "error", err)
		return
	}

	// Update metrics
	sm.recordMetrics(data)

	// Publish state updated event
	sm.eventBus.Publish(StateUpdatedEvent{
		State:  newState,
		Height: header.Height(),
	})
}

// handleHeaderRetrieved handles HeaderRetrievedEvent
func (sm *StateManager) handleHeaderRetrieved(ctx context.Context, evt events.Event) {
	// In this refactored architecture, the Syncer component handles
	// synchronizing headers and data and publishes StateUpdatedEvent
	// directly, so this handler is mostly a placeholder.
}

// handleDataRetrieved handles DataRetrievedEvent
func (sm *StateManager) handleDataRetrieved(ctx context.Context, evt events.Event) {
	// In this refactored architecture, the Syncer component handles
	// synchronizing headers and data and publishes StateUpdatedEvent
	// directly, so this handler is mostly a placeholder.
}

// applyBlock applies a block and returns the new state
func (sm *StateManager) applyBlock(ctx context.Context, header *types.SignedHeader, data *types.Data) (types.State, error) {
	sm.lastStateMtx.RLock()
	defer sm.lastStateMtx.RUnlock()

	// Extract transactions from data
	rawTxs := make([][]byte, len(data.Txs))
	for i := range data.Txs {
		rawTxs[i] = data.Txs[i]
	}

	// Execute transactions to get new state root
	newStateRoot, _, err := sm.exec.ExecuteTxs(ctx, rawTxs, header.Height(), header.Time(), sm.lastState.AppHash)
	if err != nil {
		return types.State{}, err
	}

	// Create new state with updated block info
	newState := types.State{
		Version:         sm.lastState.Version,
		ChainID:         sm.lastState.ChainID,
		InitialHeight:   sm.lastState.InitialHeight,
		LastBlockHeight: header.Height(),
		LastBlockTime:   header.Time(),
		LastBlockID: cmtypes.BlockID{
			Hash: cmbytes.HexBytes(header.Hash()),
		},
		AppHash:  newStateRoot,
		DAHeight: sm.lastState.DAHeight,
	}

	// Finalize the block
	err = sm.exec.SetFinal(ctx, header.Height())
	if err != nil {
		return types.State{}, fmt.Errorf("failed to finalize block: %w", err)
	}

	// Save updated state to store
	if err := sm.updateState(ctx, newState); err != nil {
		return types.State{}, err
	}

	return newState, nil
}

// updateState updates the stored state
func (sm *StateManager) updateState(ctx context.Context, newState types.State) error {
	sm.lastStateMtx.Lock()
	defer sm.lastStateMtx.Unlock()

	if err := sm.store.UpdateState(ctx, newState); err != nil {
		return err
	}

	sm.lastState = newState
	return nil
}

// GetLastState returns the current state
func (sm *StateManager) GetLastState() types.State {
	sm.lastStateMtx.RLock()
	defer sm.lastStateMtx.RUnlock()
	return sm.lastState
}

// recordMetrics records metrics for a block
func (sm *StateManager) recordMetrics(data *types.Data) {
	if sm.metrics == nil {
		return
	}

	sm.metrics.NumTxs.Set(float64(len(data.Txs)))
	sm.metrics.TotalTxs.Add(float64(len(data.Txs)))
	sm.metrics.BlockSizeBytes.Set(float64(data.Size()))
	sm.metrics.CommittedHeight.Set(float64(data.Metadata.Height))
}
