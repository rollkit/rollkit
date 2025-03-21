package block

import (
	"context"
	"fmt"
	"sync"
	"time"

	goheaderstore "github.com/celestiaorg/go-header/store"
	"github.com/libp2p/go-libp2p/core/crypto"

	"github.com/rollkit/rollkit/config"
	coreda "github.com/rollkit/rollkit/core/da"
	coreexecutor "github.com/rollkit/rollkit/core/execution"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	"github.com/rollkit/rollkit/events"
	"github.com/rollkit/rollkit/store"
	"github.com/rollkit/rollkit/types"
)

// Manager is the facade that coordinates all block management components
type Manager struct {
	// Components
	eventBus     *events.Bus
	producer     *Producer
	publisher    *Publisher
	syncer       *Syncer
	stateManager *StateManager

	// External interfaces
	dalc           coreda.Client
	store          store.Store
	batchQueue     *BatchQueue
	pendingHeaders *PendingHeaders

	// Configuration
	config      config.Config
	genesis     *RollkitGenesis
	proposerKey crypto.PrivKey
	isProposer  bool

	// State
	logger Logger

	// Communication channels
	HeaderCh chan *types.SignedHeader
	DataCh   chan *types.Data

	// Stores for P2P sync
	headerStore *goheaderstore.Store[*types.SignedHeader]
	dataStore   *goheaderstore.Store[*types.Data]

	// Metrics
	metrics *Metrics

	// Sequencer client
	sequencer coresequencer.Sequencer

	// Executor
	exec coreexecutor.Executor

	// Mutex for state access
	stateMtx *sync.RWMutex
}

// RollkitGenesis is the genesis state of the rollup
type RollkitGenesis struct {
	GenesisTime     time.Time
	InitialHeight   uint64
	ChainID         string
	ProposerAddress []byte
}

// Logger interface
type Logger interface {
	Debug(msg string, keyvals ...interface{})
	Info(msg string, keyvals ...interface{})
	Error(msg string, keyvals ...interface{})
}

// NewManager creates a new block manager
func NewManager(
	ctx context.Context,
	proposerKey crypto.PrivKey,
	config config.Config,
	genesis *RollkitGenesis,
	store store.Store,
	exec coreexecutor.Executor,
	sequencer coresequencer.Sequencer,
	dalc coreda.Client,
	logger Logger,
	headerStore *goheaderstore.Store[*types.SignedHeader],
	dataStore *goheaderstore.Store[*types.Data],
	seqMetrics *Metrics,
	gasPrice float64,
	gasMultiplier float64,
) (*Manager, error) {
	// Initialize base state
	s, err := getInitialState(ctx, genesis, store, exec, logger)
	if err != nil {
		logger.Error("error while getting initial state", "error", err)
		return nil, err
	}

	// Set block height in store
	store.SetHeight(ctx, s.LastBlockHeight)

	// Initialize DA height
	daHeight := s.DAHeight
	if daHeight < config.DA.StartHeight {
		daHeight = config.DA.StartHeight
	}

	// Set default configurations if not provided
	if config.DA.BlockTime.Duration == 0 {
		logger.Info("Using default DA block time", "DABlockTime", defaultDABlockTime)
		config.DA.BlockTime.Duration = defaultDABlockTime
	}

	if config.Node.BlockTime.Duration == 0 {
		logger.Info("Using default block time", "BlockTime", defaultBlockTime)
		config.Node.BlockTime.Duration = defaultBlockTime
	}

	if config.Node.LazyBlockTime.Duration == 0 {
		logger.Info("Using default lazy block time", "LazyBlockTime", defaultLazyBlockTime)
		config.Node.LazyBlockTime.Duration = defaultLazyBlockTime
	}

	if config.DA.MempoolTTL == 0 {
		logger.Info("Using default mempool ttl", "MempoolTTL", defaultMempoolTTL)
		config.DA.MempoolTTL = defaultMempoolTTL
	}

	// Check if the node is a proposer
	isProposer, err := isProposer(proposerKey, s)
	if err != nil {
		logger.Error("error while checking if proposer", "error", err)
		return nil, err
	}

	// Create pending headers
	pendingHeaders, err := NewPendingHeaders(store, logger)
	if err != nil {
		return nil, err
	}

	// Create event bus
	eventBus := events.NewEventBus(ctx, logger)

	// Create batch queue
	batchQueue := NewBatchQueue()

	// Create stateMtx for synchronized state access
	stateMtx := new(sync.RWMutex)

	// Create manager with base components
	m := &Manager{
		proposerKey:    proposerKey,
		config:         config,
		genesis:        genesis,
		store:          store,
		dalc:           dalc,
		HeaderCh:       make(chan *types.SignedHeader, channelLength),
		DataCh:         make(chan *types.Data, channelLength),
		batchQueue:     batchQueue,
		pendingHeaders: pendingHeaders,
		logger:         logger,
		isProposer:     isProposer,
		metrics:        seqMetrics,
		eventBus:       eventBus,
		headerStore:    headerStore,
		dataStore:      dataStore,
		sequencer:      sequencer,
		exec:           exec,
		stateMtx:       stateMtx,
	}

	// Create producer component
	m.producer = NewProducer(ProducerOptions{
		Config:           config,
		Genesis:          genesis,
		ProposerKey:      proposerKey,
		Store:            store,
		EventBus:         eventBus,
		Exec:             exec,
		Logger:           logger,
		InitialState:     s,
		IsProposer:       isProposer,
		DAIncludedHeight: m.GetDAIncludedHeight(),
		BatchQueue:       batchQueue,
		StateMtx:         stateMtx,
	})

	// Create publisher component
	m.publisher, err = NewPublisher(PublisherOptions{
		EventBus:       eventBus,
		Store:          store,
		DALC:           dalc,
		Logger:         logger,
		Config:         config,
		GasPrice:       gasPrice,
		GasMultiplier:  gasMultiplier,
		PendingHeaders: pendingHeaders,
	})
	if err != nil {
		return nil, err
	}

	// Create syncer component
	m.syncer = NewSyncer(SyncerOptions{
		EventBus:         eventBus,
		Store:            store,
		DALC:             dalc,
		HeaderStore:      headerStore,
		DataStore:        dataStore,
		Logger:           logger,
		Config:           config,
		DAHeight:         daHeight,
		DAIncludedHeight: m.GetDAIncludedHeight(),
		StateMtx:         stateMtx,
		Exec:             exec,
	})

	// Create state manager component
	m.stateManager = NewStateManager(StateManagerOptions{
		EventBus:     eventBus,
		Store:        store,
		Exec:         exec,
		Logger:       logger,
		Metrics:      seqMetrics,
		InitialState: s,
		StateMtx:     stateMtx,
	})

	// Subscribe to events for forwarding to external channels
	eventBus.Subscribe(EventBlockCreated, m.forwardBlockCreatedEvent)
	eventBus.Subscribe(EventStateUpdated, m.handleStateUpdated)
	eventBus.Subscribe(EventDAHeightChanged, m.handleDAHeightChanged)
	eventBus.Subscribe(EventSequencerBatch, m.handleSequencerBatch)

	return m, nil
}

// Start starts all components of the manager
func (m *Manager) Start(ctx context.Context) error {
	// Start components
	m.logger.Info("Starting producer")
	if err := m.producer.Start(ctx); err != nil {
		return err
	}

	m.logger.Info("Starting publisher")
	if err := m.publisher.Start(ctx); err != nil {
		return err
	}

	m.logger.Info("Starting syncer")
	if err := m.syncer.Start(ctx); err != nil {
		return err
	}

	m.logger.Info("Starting state manager")
	if err := m.stateManager.Start(ctx); err != nil {
		return err
	}

	return nil
}

// BatchRetrieveLoop retrieves batches from the sequencer
func (m *Manager) BatchRetrieveLoop(ctx context.Context) {
	m.logger.Info("Starting BatchRetrieveLoop")
	batchTimer := time.NewTimer(0)
	defer batchTimer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-batchTimer.C:
			start := time.Now()
			m.logger.Debug("Attempting to retrieve next batch",
				"chainID", m.genesis.ChainID,
				"lastBatchData", m.producer.GetLastBatchData())

			req := coresequencer.GetNextBatchRequest{
				RollupId:      []byte(m.genesis.ChainID),
				LastBatchData: m.producer.GetLastBatchData(),
			}

			res, err := m.sequencer.GetNextBatch(ctx, req)
			if err != nil {
				m.logger.Error("error while retrieving batch", "error", err)
				// Always reset timer on error
				batchTimer.Reset(m.config.Node.BlockTime.Duration)
				continue
			}

			if res != nil && res.Batch != nil {
				m.logger.Debug("Retrieved batch",
					"txCount", len(res.Batch.Transactions),
					"timestamp", res.Timestamp)

				// Publish sequencer batch event
				m.eventBus.Publish(SequencerBatchEvent{
					Batch: &BatchWithTime{
						Batch: res.Batch,
						Time:  res.Timestamp,
						Data:  res.BatchData,
					},
				})
			} else {
				m.logger.Debug("No batch available")
			}

			// Always reset timer
			elapsed := time.Since(start)
			remainingSleep := m.config.Node.BlockTime.Duration - elapsed
			if remainingSleep < 0 {
				remainingSleep = 0
			}
			batchTimer.Reset(remainingSleep)
		}
	}
}

// forwardBlockCreatedEvent forwards block created events to external channels
func (m *Manager) forwardBlockCreatedEvent(ctx context.Context, evt events.Event) {
	blockEvent, ok := evt.(BlockCreatedEvent)
	if !ok {
		m.logger.Error("invalid event type", "expected", "BlockCreatedEvent", "got", fmt.Sprintf("%T", evt))
		return
	}

	// Forward to external channels
	select {
	case m.HeaderCh <- blockEvent.Header:
	default:
		m.logger.Error("HeaderCh channel full, dropping event")
	}

	select {
	case m.DataCh <- blockEvent.Data:
	default:
		m.logger.Error("DataCh channel full, dropping event")
	}
}

// handleStateUpdated updates the manager's state
func (m *Manager) handleStateUpdated(ctx context.Context, evt events.Event) {
	stateEvent, ok := evt.(StateUpdatedEvent)
	if !ok {
		m.logger.Error("invalid event type", "expected", "StateUpdatedEvent", "got", fmt.Sprintf("%T", evt))
		return
	}

	if stateEvent.State.LastBlockHeight > 0 {
		m.metrics.Height.Set(float64(stateEvent.State.LastBlockHeight))
	}
}

// handleDAHeightChanged updates the manager's DA height
func (m *Manager) handleDAHeightChanged(ctx context.Context, evt events.Event) {
	daEvent, ok := evt.(DAHeightChangedEvent)
	if !ok {
		m.logger.Error("invalid event type", "expected", "DAHeightChangedEvent", "got", fmt.Sprintf("%T", evt))
		return
	}

	m.syncer.SetDAHeight(daEvent.Height)
}

// handleSequencerBatch handles sequencer batch events
func (m *Manager) handleSequencerBatch(ctx context.Context, evt events.Event) {
	batchEvent, ok := evt.(SequencerBatchEvent)
	if !ok {
		m.logger.Error("invalid event type", "expected", "SequencerBatchEvent", "got", fmt.Sprintf("%T", evt))
		return
	}

	m.batchQueue.AddBatch(*batchEvent.Batch)
}

// Public interface methods (compatible with existing API)

// DALCInitialized returns true if DALC is initialized
func (m *Manager) DALCInitialized() bool {
	return m.dalc != nil
}

// PendingHeaders returns the pending headers
func (m *Manager) PendingHeaders() *PendingHeaders {
	return m.pendingHeaders
}

// IsProposer returns true if the manager is acting as proposer
func (m *Manager) IsProposer() bool {
	return m.isProposer
}

// SeqClient returns the sequencing client
func (m *Manager) SeqClient() coresequencer.Sequencer {
	return m.sequencer
}

// GetLastState returns the current state
func (m *Manager) GetLastState() types.State {
	return m.stateManager.GetLastState()
}

// SetLastState is used to set lastState used by Manager
func (m *Manager) SetLastState(state types.State) {
	// In the refactored architecture, this would create a StateUpdatedEvent
	m.eventBus.Publish(StateUpdatedEvent{
		State:  state,
		Height: state.LastBlockHeight,
	})
}

// GetStoreHeight returns the manager's store height
func (m *Manager) GetStoreHeight() uint64 {
	return m.store.Height()
}

// GetHeaderInCh returns a channel for receiving header events
func (m *Manager) GetHeaderInCh() chan NewHeaderEvent {
	return m.syncer.GetHeaderInCh()
}

// GetDataInCh returns a channel for receiving data events
func (m *Manager) GetDataInCh() chan NewDataEvent {
	return m.syncer.GetDataInCh()
}

// IsBlockHashSeen returns true if the block with the given hash has been seen
func (m *Manager) IsBlockHashSeen(blockHash string) bool {
	return m.syncer.IsBlockHashSeen(blockHash)
}

// IsDAIncluded returns true if the block with the given hash has been included in DA
func (m *Manager) IsDAIncluded(hash types.Hash) bool {
	return m.syncer.IsDAIncluded(hash)
}

// GetDAIncludedHeight returns the rollup height at which all blocks have been included in the DA
func (m *Manager) GetDAIncludedHeight() uint64 {
	return m.syncer.GetDAIncludedHeight()
}

// SetDALC is used to set DataAvailabilityLayerClient used by Manager
func (m *Manager) SetDALC(dalc coreda.Client) {
	m.dalc = dalc
	m.publisher.SetDALC(dalc)
	m.syncer.SetDALC(dalc)
}

// GetExecutor returns the executor used by the manager
func (m *Manager) GetExecutor() coreexecutor.Executor {
	return m.exec
}
