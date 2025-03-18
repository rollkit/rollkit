package block

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	cmstate "github.com/cometbft/cometbft/proto/tendermint/state"
	cmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	cmtypes "github.com/cometbft/cometbft/types"

	goheaderstore "github.com/celestiaorg/go-header/store"
	ds "github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p/core/crypto"

	"github.com/rollkit/rollkit/config"
	coreda "github.com/rollkit/rollkit/core/da"
	coreexecutor "github.com/rollkit/rollkit/core/execution"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	"github.com/rollkit/rollkit/events"
	"github.com/rollkit/rollkit/store"
	"github.com/rollkit/rollkit/third_party/log"
	"github.com/rollkit/rollkit/types"
)

const (
	// defaultLazySleepPercent is the percentage of block time to wait to accumulate transactions
	// in lazy mode.
	// A value of 10 for e.g. corresponds to 10% of the block time. Must be between 0 and 100.
	defaultLazySleepPercent = 10

	// defaultDABlockTime is used only if DABlockTime is not configured for manager
	defaultDABlockTime = 15 * time.Second

	// defaultBlockTime is used only if BlockTime is not configured for manager
	defaultBlockTime = 1 * time.Second

	// defaultLazyBlockTime is used only if LazyBlockTime is not configured for manager
	defaultLazyBlockTime = 60 * time.Second

	// defaultMempoolTTL is the number of blocks until transaction is dropped from mempool
	defaultMempoolTTL = 25

	// blockProtocolOverhead is the protocol overhead when marshaling the block to blob
	// see: https://gist.github.com/tuxcanfly/80892dde9cdbe89bfb57a6cb3c27bae2
	blockProtocolOverhead = 1 << 16

	// maxSubmitAttempts defines how many times Rollkit will re-try to publish block to DA layer.
	// This is temporary solution. It will be removed in future versions.
	maxSubmitAttempts = 30

	// Applies to most channels, 100 is a large enough buffer to avoid blocking
	channelLength = 100

	// Applies to the headerInCh, 10000 is a large enough number for headers per DA block.
	headerInChLength = 10000

	// initialBackoff defines initial value for block submission backoff
	initialBackoff = 100 * time.Millisecond

	// DAIncludedHeightKey is the key used for persisting the da included height in store.
	DAIncludedHeightKey = "d"

	// LastBatchHashKey is the key used for persisting the last batch hash in store.
	LastBatchHashKey = "l"
)

// dataHashForEmptyTxs to be used while only syncing headers from DA and no p2p to get the Data for no txs scenarios, the syncing can proceed without getting stuck forever.
var dataHashForEmptyTxs = []byte{110, 52, 11, 156, 255, 179, 122, 152, 156, 165, 68, 230, 187, 120, 10, 44, 120, 144, 29, 63, 179, 55, 56, 118, 133, 17, 163, 6, 23, 175, 160, 29}

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
	headerCache    *HeaderCache
	dataCache      *DataCache
	store          store.Store
	batchQueue     *BatchQueue
	pendingHeaders *PendingHeaders

	// Configuration
	config      config.Config
	genesis     *RollkitGenesis
	proposerKey crypto.PrivKey
	isProposer  bool

	// State
	daHeight         atomic.Uint64
	daIncludedHeight atomic.Uint64
	logger           Logger

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

	// Last batch hash
	lastBatchHash []byte

	// Executor
	exec coreexecutor.Executor
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

	// Create header and data caches
	headerCache := NewHeaderCache()
	dataCache := NewDataCache()

	// Create event bus
	eventBus := events.NewEventBus(ctx, logger)

	// Create batch queue
	batchQueue := NewBatchQueue()

	// Create manager with base components
	m := &Manager{
		proposerKey:    proposerKey,
		config:         config,
		genesis:        genesis,
		store:          store,
		dalc:           dalc,
		daHeight:       atomic.Uint64{},
		HeaderCh:       make(chan *types.SignedHeader, channelLength),
		DataCh:         make(chan *types.Data, channelLength),
		headerCache:    headerCache,
		dataCache:      dataCache,
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
	}

	// Set DA heights
	m.daHeight.Store(daHeight)

	// Retrieve last DA included height
	if height, err := m.store.GetMetadata(ctx, DAIncludedHeightKey); err == nil && len(height) == 8 {
		m.daIncludedHeight.Store(binary.BigEndian.Uint64(height))
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
		DAIncludedHeight: m.daIncludedHeight.Load(),
	})

	// Create publisher component - pass gasPrice and gasMultiplier only here
	m.publisher, err = NewPublisher(PublisherOptions{
		EventBus:      eventBus,
		Store:         store,
		DALC:          dalc,
		Logger:        logger,
		Config:        config,
		GasPrice:      gasPrice,
		GasMultiplier: gasMultiplier,
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
		DAIncludedHeight: m.daIncludedHeight.Load(),
	})

	// Create state manager component
	m.stateManager = NewStateManager(StateManagerOptions{
		EventBus:     eventBus,
		Store:        store,
		Exec:         exec,
		Logger:       logger,
		Metrics:      seqMetrics,
		InitialState: s,
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
	// Start event bus
	m.logger.Info("Starting event bus")

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

	m.daHeight.Store(daEvent.Height)
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
				"lastBatchHash", hex.EncodeToString(m.lastBatchHash))

			req := coresequencer.GetNextBatchRequest{
				RollupId:      []byte(m.genesis.ChainID),
				LastBatchHash: m.lastBatchHash,
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

				if h, err := res.Batch.Hash(); err == nil {
					// Publish sequencer batch event
					m.eventBus.Publish(SequencerBatchEvent{
						Batch: &BatchWithTime{
							Batch: res.Batch,
							Time:  res.Timestamp,
						},
					})

					if len(res.Batch.Transactions) != 0 {
						if err := m.store.SetMetadata(ctx, LastBatchHashKey, h); err != nil {
							m.logger.Error("error while setting last batch hash", "error", err)
						}
						m.lastBatchHash = h
					}
				}
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

// GetLastState returns the last recorded state
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
// Note: This is for backward compatibility
func (m *Manager) GetHeaderInCh() chan NewHeaderEvent {
	ch := make(chan NewHeaderEvent, headerInChLength)

	// Subscribe to header events and forward to channel
	m.eventBus.Subscribe(EventHeaderRetrieved, func(ctx context.Context, evt events.Event) {
		headerEvent, ok := evt.(HeaderRetrievedEvent)
		if !ok {
			return
		}

		ch <- NewHeaderEvent{
			Header:   headerEvent.Header,
			DAHeight: headerEvent.DAHeight,
		}
	})

	return ch
}

// GetDataInCh returns a channel for receiving data events
// Note: This is for backward compatibility
func (m *Manager) GetDataInCh() chan NewDataEvent {
	ch := make(chan NewDataEvent, headerInChLength)

	// Subscribe to data events and forward to channel
	m.eventBus.Subscribe(EventDataRetrieved, func(ctx context.Context, evt events.Event) {
		dataEvent, ok := evt.(DataRetrievedEvent)
		if !ok {
			return
		}

		ch <- NewDataEvent{
			Data:     dataEvent.Data,
			DAHeight: dataEvent.DAHeight,
		}
	})

	return ch
}

// IsBlockHashSeen returns true if the block with the given hash has been seen
func (m *Manager) IsBlockHashSeen(blockHash string) bool {
	return m.headerCache.isSeen(blockHash)
}

// IsDAIncluded returns true if the block with the given hash has been included in DA
func (m *Manager) IsDAIncluded(hash types.Hash) bool {
	return m.headerCache.isDAIncluded(hash.String())
}

// GetDAIncludedHeight returns the rollup height at which all blocks have been included in the DA
func (m *Manager) GetDAIncludedHeight() uint64 {
	return m.daIncludedHeight.Load()
}

// SetDALC is used to set DataAvailabilityLayerClient used by Manager
func (m *Manager) SetDALC(dalc coreda.Client) {
	m.dalc = dalc
}

// GetExecutor returns the executor used by the manager
func (m *Manager) GetExecutor() coreexecutor.Executor {
	return m.exec
}

// isProposer returns whether or not the manager is a proposer
func isProposer(_ crypto.PrivKey, _ types.State) (bool, error) {
	return true, nil
}

// getInitialState tries to load lastState from Store, and if it's not available it reads GenesisDoc.
func getInitialState(ctx context.Context, genesis *RollkitGenesis, store store.Store, exec coreexecutor.Executor, logger log.Logger) (types.State, error) {
	// Load the state from store.
	s, err := store.GetState(ctx)

	if errors.Is(err, ds.ErrNotFound) {
		logger.Info("No state found in store, initializing new state")

		// Initialize genesis block explicitly
		err = store.SaveBlockData(ctx,
			&types.SignedHeader{Header: types.Header{
				BaseHeader: types.BaseHeader{
					Height: genesis.InitialHeight,
					Time:   uint64(genesis.GenesisTime.UnixNano()),
				}}},
			&types.Data{},
			&types.Signature{},
		)
		if err != nil {
			return types.State{}, fmt.Errorf("failed to save genesis block: %w", err)
		}

		// If the user is starting a fresh chain (or hard-forking), we assume the stored state is empty.
		// TODO(tzdybal): handle max bytes
		stateRoot, _, err := exec.InitChain(ctx, genesis.GenesisTime, genesis.InitialHeight, genesis.ChainID)
		if err != nil {
			logger.Error("error while initializing chain", "error", err)
			return types.State{}, err
		}

		s := types.State{
			ChainID:         genesis.ChainID,
			InitialHeight:   genesis.InitialHeight,
			LastBlockHeight: genesis.InitialHeight - 1,
			LastBlockID:     cmtypes.BlockID{},
			LastBlockTime:   genesis.GenesisTime,
			AppHash:         stateRoot,
			DAHeight:        0,
			// TODO(tzdybal): we don't need fields below
			Version:                          cmstate.Version{},
			ConsensusParams:                  cmproto.ConsensusParams{},
			LastHeightConsensusParamsChanged: 0,
			LastResultsHash:                  nil,
			Validators:                       nil,
			NextValidators:                   nil,
			LastValidators:                   nil,
			LastHeightValidatorsChanged:      0,
		}
		return s, nil
	} else if err != nil {
		logger.Error("error while getting state", "error", err)
		return types.State{}, err
	} else {
		// Perform a sanity-check to stop the user from
		// using a higher genesis than the last stored state.
		// if they meant to hard-fork, they should have cleared the stored State
		if uint64(genesis.InitialHeight) > s.LastBlockHeight { //nolint:unconvert
			return types.State{}, fmt.Errorf("genesis.InitialHeight (%d) is greater than last stored state's LastBlockHeight (%d)", genesis.InitialHeight, s.LastBlockHeight)
		}
	}

	return s, nil
}
