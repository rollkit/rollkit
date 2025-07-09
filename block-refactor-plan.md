# Rollkit Block Package Refactoring Plan

## Table of Contents

1. [Executive Summary](#executive-summary)
2. [Current Architecture Analysis](#current-architecture-analysis)
3. [Code Quality Assessment](#code-quality-assessment)
4. [Architectural Recommendations](#architectural-recommendations)
5. [Implementation Plan](#implementation-plan)
6. [Technical Reference](#technical-reference)

---

## Executive Summary

This document provides a comprehensive analysis and refactoring plan for the Rollkit block package. The block package is a critical component responsible for block management, including creation, validation, submission to data availability layers, retrieval, and synchronization.

### Key Findings

- The package follows good modular design principles but suffers from a "god object" anti-pattern in the Manager struct
- Several concurrency issues and potential race conditions exist
- Error handling is inconsistent across components
- The architecture would benefit from event-driven patterns and better separation of concerns
- An existing service package (`pkg/service`) provides a base service implementation that can be leveraged

### Proposed Solution

A 15-phase incremental refactoring plan that:

- Can be implemented over 8-10 weeks
- Each phase is independently mergeable
- Maintains backward compatibility
- Improves testability, performance, and maintainability
- Utilizes the existing `pkg/service.BaseService` for lifecycle management

---

## Current Architecture Analysis

### Overview

The block package implements a sophisticated pipeline for block management with the following main components:

```
Aggregation → Submission → DA Layer → Retrieval → Validation → Application
```

### Main Components

#### 1. **Manager** (`manager.go`)

The central orchestrator that coordinates all block operations.

**Responsibilities:**

- Manages blockchain state
- Coordinates between subsystems
- Interfaces with core dependencies (Executor, Sequencer, DA)
- Handles P2P broadcasting

**Key Issues:**

- Too many responsibilities (100+ methods)
- Complex initialization (136 lines)
- Direct field access creates tight coupling

#### 2. **Aggregation** (`aggregation.go`)

Handles block production in two modes:

- **Normal mode**: Regular interval block production
- **Lazy mode**: Transaction-triggered block production

**Key Features:**

- Timer-based scheduling
- Transaction availability detection
- Configurable block time

#### 3. **Submitter** (`submitter.go`)

Manages data availability layer submissions:

- Separate header and data submission loops
- Retry logic with exponential backoff
- Gas price adjustment
- Batch submission support

#### 4. **Retriever** (`retriever.go`)

Fetches data from the DA layer:

- Continuous retrieval loop
- Blob processing and validation
- Retry mechanisms

#### 5. **Store** (`store.go`)

Interfaces with local storage:

- Header and data retrieval loops
- P2P synchronization support
- Sequencer validation

#### 6. **Sync** (`sync.go`)

Coordinates block synchronization:

- Processes incoming headers and data
- Manages block application
- Handles empty blocks efficiently

### Core Interface Dependencies

```go
// From core/execution/execution.go
type Executor interface {
    InitChain(ctx context.Context, genesis time.Time, initialHeight uint64, chainID string) ([]byte, uint64, error)
    GetTxs(ctx context.Context) ([]types.Tx, error)
    ExecuteTxs(ctx context.Context, txs []types.Tx, blockHeight uint64, timestamp time.Time, prevStateRoot types.Hash) (types.Hash, uint64, error)
    SetFinal(ctx context.Context, height uint64) error
}

// From core/sequencer/sequencing.go
type Sequencer interface {
    SubmitBatchTxs(ctx context.Context, txs []types.Tx) (*Batch, error)
    GetNextBatch(ctx context.Context, lastBatch *Batch) (*Batch, error)
    VerifyBatch(ctx context.Context, batch *Batch) (bool, error)
}

// From core/da/da.go
type DA interface {
    Submit(ctx context.Context, blobs []types.Blob) ([]types.ID, error)
    Get(ctx context.Context, ids []types.ID) ([]types.Blob, error)
    GetIDs(ctx context.Context, height uint64) ([]types.ID, error)
}
```

### Communication Patterns

The package uses 7+ channels for coordination:

- `headerInCh`, `dataInCh`: Incoming block components
- `headerStoreCh`, `dataStoreCh`: Storage synchronization
- `retrieveCh`: DA retrieval triggers
- `daIncluderCh`: DA inclusion notifications
- `txNotifyCh`: Transaction availability

**Issue**: Non-blocking sends can drop critical signals:

```go
func (m *Manager) sendNonBlockingSignalToHeaderStoreCh() {
    select {
    case m.headerStoreCh <- struct{}{}:
    default:
        // Signal dropped silently
    }
}
```

---

## Code Quality Assessment

### 1. Race Conditions and Concurrency Issues

#### Critical Issues

**State Race Condition**

```go
// Potential race between state update and DA height
newState.DAHeight = m.daHeight.Load()
if err = m.updateState(ctx, newState); err != nil {
    return fmt.Errorf("failed to update state: %w", err)
}
```

**Cache Access**

- `headerCache` and `dataCache` accessed from multiple goroutines without apparent synchronization

#### Recommendations

- Use atomic operations consistently
- Add metrics for dropped signals
- Implement proper cache synchronization

### 2. Error Handling Issues

**Silent Error Conditions**

```go
// In publishBlockInternal()
if err != nil {
    if errors.Is(err, ErrNoBatch) {
        m.logger.Info("no batch retrieved from sequencer")
        return nil // Silent return
    }
}
```

**Inconsistent Error Wrapping**

- Some errors provide context, others don't
- No structured error types for different failure modes

### 3. Resource Management

**Goroutine Lifecycle**

- Multiple long-running loops without lifecycle management
- No graceful shutdown mechanism
- Potential for goroutine leaks

**Good Practices Found:**

- Proper timer cleanup with `defer timer.Stop()`
- Context cancellation support

### 4. Code Complexity

**High Complexity Functions:**

- `publishBlockInternal()`: 161 lines, multiple responsibilities
- `NewManager()`: 136 lines, complex initialization

**Metrics Needed:**

- Cyclomatic complexity reduction (target: < 10)
- Function size reduction (target: < 50 lines)

### 5. Testing Gaps

**Missing Test Scenarios:**

- Concurrent state access
- Channel overflow conditions
- Complex error paths
- Integration tests for full workflows

---

## Architectural Recommendations

### 1. Apply Domain-Driven Design

Create clear bounded contexts:

```go
// Separate interfaces for each domain
type BlockProducer interface {
    ProduceBlock(ctx context.Context) (*types.Block, error)
}

type BlockSubmitter interface {
    SubmitToDA(ctx context.Context, block *types.Block) error
    GetSubmissionStatus(height uint64) SubmissionStatus
}

type BlockSynchronizer interface {
    SyncBlock(ctx context.Context, height uint64) error
    GetSyncStatus() SyncStatus
}
```

### 2. Implement Event Sourcing

Replace channel proliferation with event bus:

```go
type BlockEvent interface {
    Type() EventType
    Height() uint64
    Timestamp() time.Time
}

type EventBus struct {
    subscribers map[EventType][]EventHandler
    metrics     *EventMetrics
}

// Event types
type BlockProducedEvent struct {
    Block     *types.Block
    Timestamp time.Time
}
```

### 3. Extract State Machine

Formalize block lifecycle:

```go
type BlockState int

const (
    BlockStatePending BlockState = iota
    BlockStateSubmitted
    BlockStateIncludedInDA
    BlockStateFinalized
)

type BlockStateMachine struct {
    transitions map[BlockState][]BlockState
    handlers    map[BlockState]StateHandler
}
```

### 4. Implement Coordinator Pattern

Simplify Manager to pure coordination using service.BaseService:

```go
type BlockCoordinator struct {
    *service.BaseService

    producer     BlockProducer
    submitter    BlockSubmitter
    synchronizer BlockSynchronizer
    eventBus     *EventBus
    stateTracker *BlockStateTracker

    // Service wrappers
    producerSvc     *BlockProducerService
    submitterSvc    *BlockSubmitterService
    synchronizerSvc *BlockSynchronizerService
}

func NewBlockCoordinator(logger logging.EventLogger, /* deps */) *BlockCoordinator {
    bc := &BlockCoordinator{
        // Initialize fields
    }
    bc.BaseService = service.NewBaseService(logger, "BlockCoordinator", bc)
    return bc
}

func (bc *BlockCoordinator) Run(ctx context.Context) error {
    // Start all sub-services
    g, ctx := errgroup.WithContext(ctx)

    g.Go(func() error { return bc.producerSvc.Run(ctx) })
    g.Go(func() error { return bc.submitterSvc.Run(ctx) })
    g.Go(func() error { return bc.synchronizerSvc.Run(ctx) })

    return g.Wait()
}
```

---

## Implementation Plan

### Overview

15 independent phases over 8-10 weeks. Each phase is:

- Independently implementable
- Separately testable
- Individually mergeable
- Backward compatible
- Leverages existing `pkg/service.BaseService` for lifecycle management

### Key Integration Points with Existing Code

- **Service Package**: All components will utilize `pkg/service.BaseService` for consistent lifecycle management
- **Logging**: Components will use the existing `ipfs/go-log/v2` logger from BaseService
- **Context Handling**: BaseService provides proper context propagation and cancellation

### Phase 1: Enhanced Metrics Infrastructure ✅ COMPLETED

**Goal**: Establish comprehensive metrics before making changes

**Status**: Successfully completed with improvements beyond the original plan.

**What Was Implemented**:

1. **Unified Metrics Structure**: Instead of creating a separate `ExtendedMetrics` struct that embeds the original, we merged all metrics into a single `Metrics` struct in `metrics.go`. This provides a cleaner, simpler API.

2. **Comprehensive Metric Types Added**:
   - **Channel metrics**: Buffer usage for all 7 channels, dropped signal counter
   - **Error metrics**: Counters by error type (5 types), recoverable vs non-recoverable errors
   - **Performance metrics**: Operation duration histograms (5 operations), goroutine count
   - **DA metrics**: Submission/retrieval attempts, successes, failures, inclusion height, pending counts
   - **Sync metrics**: Headers/data synced, blocks applied, invalid headers, sync lag
   - **Block production metrics**: Production time histogram, empty/lazy/normal blocks counters, transactions per block histogram
   - **State transition metrics**: Transition counters (3 types), invalid transitions

3. **Helper Functions** (`metrics_helpers.go`):
   - `MetricsTimer` for tracking operation durations
   - `sendNonBlockingSignalWithMetrics` to track dropped signals
   - `updateChannelMetrics` for periodic channel buffer monitoring
   - `recordError`, `recordDAMetrics`, `recordBlockProductionMetrics`, `recordSyncMetrics`
   - `updatePendingMetrics` for tracking pending headers/data counts

4. **Metrics Collection Points Added**:
   - **manager.go**: Block production timing, extended block production metrics
   - **sync.go**: Headers/data synced, blocks applied, periodic channel metrics updates
   - **submitter.go**: DA submission attempt/success tracking
   - **retriever.go**: DA retrieval attempt/success tracking
   - All channel signal functions updated to track dropped signals

5. **Testing**:
   - Comprehensive unit tests in `metrics_test.go`
   - Helper function tests
   - Integration tests with Prometheus metrics

**Key Improvements Over Original Plan**:

- Single unified `Metrics` struct instead of embedded design
- Simpler API - just `PrometheusMetrics()` and `NopMetrics()`
- No breaking changes - existing code continues to work
- Better maintainability with all metrics in one place

**Files Modified**:

- `metrics.go` - Unified all metrics into single struct
- `metrics_helpers.go` - Added helper functions for metric collection
- `metrics_test.go` - Comprehensive test coverage
- `manager.go` - Updated to use unified Metrics type
- `sync.go` - Added metric collection points
- `submitter.go` - Added DA submission metrics
- `retriever.go` - Added DA retrieval metrics
- Various test files updated to use `NopMetrics()`

**Merge Status**: ✅ Ready for production use

### Phase 2: Error Type System (2-3 days)

**Goal**: Implement structured error types

**Implementation**:

```go
// errors_extended.go
type BlockError interface {
    error
    Code() string
    Recoverable() bool
    Context() map[string]interface{}
}

type BlockProductionError struct {
    Height      uint64
    Cause       error
    Recoverable bool
    Context     map[string]interface{}
}

type DASubmissionError struct {
    Height      uint64
    Attempt     int
    StatusCode  coreda.StatusCode
    Cause       error
}
```

**Testing Strategy**:

- Error wrapping tests
- Error categorization tests

### Phase 3: Channel Management Abstraction (2-3 days)

**Goal**: Create safer channel handling

**Implementation**:

```go
// channels.go
type SignalChannel struct {
    ch       chan struct{}
    name     string
    metrics  *ChannelMetrics
    capacity int
}

func (s *SignalChannel) Send() bool {
    select {
    case s.ch <- struct{}{}:
        s.metrics.SentCount.Inc()
        return true
    default:
        s.metrics.DroppedCount.Inc()
        return false
    }
}

type EventChannel[T any] struct {
    ch       chan T
    name     string
    metrics  *ChannelMetrics
    overflow OverflowStrategy
}
```

### Phase 4: State Machine Implementation (3-4 days)

**Goal**: Formalize block lifecycle states

**Implementation**:

```go
// state_machine.go
type BlockStateMachine struct {
    mu          sync.RWMutex
    states      map[uint64]BlockState
    transitions map[BlockState]map[BlockState]TransitionHandler
    metrics     *StateMetrics
}

func (sm *BlockStateMachine) Transition(height uint64, from, to BlockState) error {
    sm.mu.Lock()
    defer sm.mu.Unlock()

    handler, valid := sm.transitions[from][to]
    if !valid {
        return ErrInvalidTransition{From: from, To: to}
    }

    if err := handler(height); err != nil {
        return err
    }

    sm.states[height] = to
    sm.metrics.RecordTransition(from, to)
    return nil
}
```

### Phase 5: Event Bus System (3-4 days)

**Goal**: Decouple components with pub/sub

**Implementation**:

```go
// events.go
type EventBus struct {
    mu          sync.RWMutex
    subscribers map[EventType][]EventHandler
    buffer      *ring.Buffer
    metrics     *EventMetrics
}

func (eb *EventBus) Publish(event Event) {
    eb.mu.RLock()
    handlers := eb.subscribers[event.Type()]
    eb.mu.RUnlock()

    for _, handler := range handlers {
        go func(h EventHandler) {
            if err := h.Handle(event); err != nil {
                eb.metrics.HandlerErrors.Inc()
            }
        }(handler)
    }
}
```

### Phase 6: Component Interface Extraction (2-3 days)

**Goal**: Define clear component interfaces that work with pkg/service

**Implementation**:

```go
// interfaces.go
// Components will embed service.BaseService and implement service.Service

type BlockProducer interface {
    ProduceBlock(ctx context.Context) (*types.Block, error)
    SetMode(mode ProducerMode) error
}

type BlockSubmitter interface {
    Submit(ctx context.Context, height uint64) error
    GetStatus(height uint64) SubmissionStatus
}

type BlockRetriever interface {
    Retrieve(ctx context.Context, height uint64) (*types.Block, error)
    GetAvailableRange() (start, end uint64)
}

type BlockSynchronizer interface {
    SyncBlock(ctx context.Context, height uint64) error
    GetSyncStatus() SyncStatus
}

// Each component will be wrapped in a service
// Example: BlockProducerService wraps BlockProducer and extends BaseService
```

### Phase 7-10: Component Refactoring (12-16 days total)

Extract each major component into its own package:

- Phase 7: Aggregation → `producer/`
- Phase 8: Submission → `submitter/`
- Phase 9: Retrieval → `retriever/`
- Phase 10: Synchronization → `sync/`

### Phase 11: Manager Simplification (3-4 days)

**Goal**: Refactor Manager to coordinator pattern

**Implementation**:

```go
// coordinator.go
type BlockCoordinator struct {
    components []Component
    eventBus   *EventBus
    state      *StateTracker
}

func (c *BlockCoordinator) Start(ctx context.Context) error {
    g, ctx := errgroup.WithContext(ctx)

    for _, component := range c.components {
        comp := component
        g.Go(func() error {
            return comp.Start(ctx)
        })
    }

    return g.Wait()
}
```

### Phase 12: Lifecycle Management (2-3 days)

**Goal**: Add proper service lifecycle using existing `pkg/service` package

**Implementation**:

```go
// Utilize existing pkg/service.BaseService
import "github.com/rollkit/rollkit/pkg/service"

// Example: BlockProducerService using BaseService
type BlockProducerService struct {
    *service.BaseService

    producer    BlockProducer
    eventBus    *EventBus
    config      ProductionConfig
}

func NewBlockProducerService(logger logging.EventLogger, producer BlockProducer, eventBus *EventBus) *BlockProducerService {
    bps := &BlockProducerService{
        producer: producer,
        eventBus: eventBus,
    }
    bps.BaseService = service.NewBaseService(logger, "BlockProducer", bps)
    return bps
}

func (bps *BlockProducerService) Run(ctx context.Context) error {
    bps.Logger.Info("starting block producer")

    ticker := time.NewTicker(bps.config.BlockTime)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            bps.Logger.Info("stopping block producer")
            return ctx.Err()
        case <-ticker.C:
            if err := bps.produceBlock(ctx); err != nil {
                bps.Logger.Error("failed to produce block", "error", err)
            }
        }
    }
}

// ServiceGroup to manage multiple BaseService instances
type ServiceGroup struct {
    services []service.Service
}

func (sg *ServiceGroup) Run(ctx context.Context) error {
    g, ctx := errgroup.WithContext(ctx)

    for _, svc := range sg.services {
        s := svc
        g.Go(func() error {
            return s.Run(ctx)
        })
    }

    return g.Wait()
}
```

### Phase 13: Configuration Management (2-3 days)

**Goal**: Extract all configuration

**Implementation**:

```go
// config_types.go
type BlockConfig struct {
    Production  ProductionConfig
    Submission  SubmissionConfig
    Retrieval   RetrievalConfig
    Sync        SyncConfig
}

// defaults.go
var DefaultBlockConfig = BlockConfig{
    Production: ProductionConfig{
        BlockTime:     1 * time.Second,
        LazyBlockTime: 60 * time.Second,
        MaxTxsPerBlock: 1000,
    },
    // ...
}
```

### Phase 14: Performance Optimizations (3-4 days)

**Goal**: Implement performance improvements

**Tasks**:

- Read/write path separation
- Adaptive batching
- Multi-tier caching
- Lock contention optimization

### Phase 15: Documentation and Cleanup (2-3 days)

**Goal**: Complete documentation

**Deliverables**:

- Architecture Decision Records (ADRs)
- Component interaction diagrams
- Migration guide
- Performance benchmarks

---

## Technical Reference

### Using pkg/service.BaseService

#### Creating a Service Component

```go
package producer

import (
    "github.com/rollkit/rollkit/pkg/service"
    logging "github.com/ipfs/go-log/v2"
)

type BlockProducerService struct {
    *service.BaseService

    config      ProductionConfig
    executor    coreexecutor.Executor
    sequencer   coresequencer.Sequencer
    eventBus    *EventBus

    // Internal state
    lastProducedHeight uint64
}

func NewBlockProducerService(
    logger logging.EventLogger,
    config ProductionConfig,
    executor coreexecutor.Executor,
    sequencer coresequencer.Sequencer,
    eventBus *EventBus,
) *BlockProducerService {
    bps := &BlockProducerService{
        config:    config,
        executor:  executor,
        sequencer: sequencer,
        eventBus:  eventBus,
    }
    bps.BaseService = service.NewBaseService(logger, "BlockProducer", bps)
    return bps
}

func (bps *BlockProducerService) Run(ctx context.Context) error {
    bps.Logger.Info("block producer service started")
    defer bps.Logger.Info("block producer service stopped")

    ticker := time.NewTicker(bps.config.BlockTime)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-ticker.C:
            if err := bps.produceBlock(ctx); err != nil {
                bps.Logger.Error("failed to produce block",
                    "error", err,
                    "height", bps.lastProducedHeight+1)
                // Continue on error
            }
        }
    }
}

func (bps *BlockProducerService) produceBlock(ctx context.Context) error {
    // Block production logic here
    return nil
}
```

### Testing Strategy

#### Unit Testing Template

```go
func TestComponent_Method(t *testing.T) {
    tests := []struct {
        name    string
        setup   func() *Component
        input   InputType
        want    OutputType
        wantErr bool
    }{
        // Test cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            c := tt.setup()
            got, err := c.Method(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("Method() error = %v, wantErr %v", err, tt.wantErr)
            }
            if !reflect.DeepEqual(got, tt.want) {
                t.Errorf("Method() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

#### Testing BaseService Components

```go
func TestBlockProducerService_Run(t *testing.T) {
    logger := logging.Logger("test")
    _ = logging.SetLogLevel("test", "DEBUG")

    mockExecutor := &MockExecutor{}
    mockSequencer := &MockSequencer{}
    eventBus := NewEventBus()

    config := ProductionConfig{
        BlockTime: 100 * time.Millisecond,
    }

    service := NewBlockProducerService(
        logger,
        config,
        mockExecutor,
        mockSequencer,
        eventBus,
    )

    ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
    defer cancel()

    // Run service in background
    errCh := make(chan error, 1)
    go func() {
        errCh <- service.Run(ctx)
    }()

    // Wait for some blocks to be produced
    time.Sleep(350 * time.Millisecond)

    // Cancel context to stop service
    cancel()

    // Check that service stopped cleanly
    err := <-errCh
    require.ErrorIs(t, err, context.Canceled)

    // Verify blocks were produced
    require.GreaterOrEqual(t, mockExecutor.CallCount(), 3)
}
```

#### Integration Testing

```go
func TestBlockFlow_EndToEnd(t *testing.T) {
    // Setup
    logger := logging.Logger("test")
    coordinator := setupTestCoordinator(t, logger)
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // Start coordinator service
    errCh := make(chan error, 1)
    go func() {
        errCh <- coordinator.Run(ctx)
    }()

    // Wait for services to start
    time.Sleep(100 * time.Millisecond)

    // Test block flow
    block := createTestBlock()
    require.NoError(t, coordinator.ProduceBlock(ctx, block))

    // Wait for block finalization
    require.Eventually(t, func() bool {
        status := coordinator.GetBlockStatus(block.Height)
        return status == BlockStateFinalized
    }, 10*time.Second, 100*time.Millisecond)

    // Shutdown
    cancel()
    require.ErrorIs(t, <-errCh, context.Canceled)
}
```

### Migration Guide

#### For External Consumers

```go
// Old API (still supported during transition)
manager := block.NewManager(...)
// Manager methods continue to work

// New API (using service.BaseService)
logger := logging.Logger("block")
config := block.DefaultConfig()
coordinator := block.NewBlockCoordinator(logger, config, executor, sequencer, da)

// Run the coordinator service
ctx := context.Background()
go func() {
    if err := coordinator.Run(ctx); err != nil {
        logger.Error("coordinator stopped", "error", err)
    }
}()

// Graceful shutdown
shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
coordinator.Stop(shutdownCtx)
```

#### Feature Flags

```go
// Enable new features gradually
type FeatureFlags struct {
    UseEventBus      bool
    UseStateMachine  bool
    UseNewSubmitter  bool
}
```

### Performance Benchmarks

#### Baseline Metrics

- Block production: < 10ms
- DA submission: < 100ms (excluding network)
- Block validation: < 5ms
- State update: < 20ms

#### Target Improvements

- 20% reduction in block production time
- 30% reduction in memory allocation
- 50% reduction in lock contention

### Monitoring and Observability

#### Key Metrics to Track

1. **Block Production Rate**: blocks/minute
2. **DA Submission Success Rate**: percentage
3. **Sync Lag**: blocks behind head
4. **Error Rates**: by error type
5. **Resource Usage**: goroutines, memory, CPU

#### Alerting Rules

```yaml
- alert: HighBlockProductionLatency
  expr: block_production_duration_seconds > 0.1
  for: 5m

- alert: DASubmissionFailures
  expr: rate(da_submission_errors_total[5m]) > 0.1
  for: 10m
```

---

## Appendix: Code Examples

### Example 1: Circuit Breaker for DA Submission

```go
type CircuitBreaker struct {
    failures    int
    maxFailures int
    timeout     time.Duration
    lastFailure time.Time
    mu          sync.Mutex
}

func (cb *CircuitBreaker) Call(fn func() error) error {
    cb.mu.Lock()
    defer cb.mu.Unlock()

    if cb.isOpen() {
        return ErrCircuitOpen
    }

    err := fn()
    if err != nil {
        cb.recordFailure()
    } else {
        cb.reset()
    }

    return err
}
```

### Example 2: Adaptive Batching

```go
type AdaptiveBatcher struct {
    minSize      int
    maxSize      int
    targetLatency time.Duration
    currentSize  int
}

func (ab *AdaptiveBatcher) NextBatchSize(lastLatency time.Duration) int {
    if lastLatency > ab.targetLatency {
        ab.currentSize = max(ab.minSize, ab.currentSize/2)
    } else {
        ab.currentSize = min(ab.maxSize, ab.currentSize*2)
    }
    return ab.currentSize
}
```

---

## Conclusion

This refactoring plan provides a path to transform the block package from a monolithic design to a modular, event-driven architecture. The incremental approach ensures that each change can be validated independently, reducing risk and allowing for continuous delivery of improvements.

### Key Benefits

- **Leverages Existing Infrastructure**: Uses the proven `pkg/service.BaseService` for consistent lifecycle management
- **Improved Testability**: Each component can be tested in isolation with proper context handling
- **Better Performance**: Reduced lock contention and optimized resource utilization
- **Enhanced Observability**: Comprehensive metrics and structured logging through BaseService
- **Future Scalability**: Modular design allows for easy extension and distribution

### Critical Success Factors

1. **Incremental Delivery**: Each phase must be independently mergeable
2. **Backward Compatibility**: Existing APIs remain functional during transition
3. **Comprehensive Testing**: Every phase includes unit and integration tests
4. **Service Pattern Consistency**: All components follow the BaseService pattern for uniformity

By utilizing the existing `pkg/service` package, we ensure consistency with other Rollkit components while significantly reducing the implementation complexity and risk.
