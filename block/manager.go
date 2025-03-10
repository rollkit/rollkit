package block

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	goheaderstore "github.com/celestiaorg/go-header/store"
	abci "github.com/cometbft/cometbft/abci/types"
	cmtbytes "github.com/cometbft/cometbft/libs/bytes"
	cmtypes "github.com/cometbft/cometbft/types"
	ds "github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p/core/crypto"

	execTypes "github.com/rollkit/go-execution/types"

	"github.com/rollkit/rollkit/config"
	coreexecutor "github.com/rollkit/rollkit/core/execution"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	"github.com/rollkit/rollkit/da"
	"github.com/rollkit/rollkit/store"
	"github.com/rollkit/rollkit/third_party/log"
	"github.com/rollkit/rollkit/types"
)

// defaultLazySleepPercent is the percentage of block time to wait to accumulate transactions
// in lazy mode.
// A value of 10 for e.g. corresponds to 10% of the block time. Must be between 0 and 100.
const defaultLazySleepPercent = 10

// defaultDABlockTime is used only if DABlockTime is not configured for manager
const defaultDABlockTime = 15 * time.Second

// defaultBlockTime is used only if BlockTime is not configured for manager
const defaultBlockTime = 1 * time.Second

// defaultLazyBlockTime is used only if LazyBlockTime is not configured for manager
const defaultLazyBlockTime = 60 * time.Second

// defaultMempoolTTL is the number of blocks until transaction is dropped from mempool
const defaultMempoolTTL = 25

// blockProtocolOverhead is the protocol overhead when marshaling the block to blob
// see: https://gist.github.com/tuxcanfly/80892dde9cdbe89bfb57a6cb3c27bae2
const blockProtocolOverhead = 1 << 16

// maxSubmitAttempts defines how many times Rollkit will re-try to publish block to DA layer.
// This is temporary solution. It will be removed in future versions.
const maxSubmitAttempts = 30

// Applies to most channels, 100 is a large enough buffer to avoid blocking
const channelLength = 100

// Applies to the headerInCh, 10000 is a large enough number for headers per DA block.
const headerInChLength = 10000

// initialBackoff defines initial value for block submission backoff
var initialBackoff = 100 * time.Millisecond

// DAIncludedHeightKey is the key used for persisting the da included height in store.
const DAIncludedHeightKey = "d"

// LastBatchHashKey is the key used for persisting the last batch hash in store.
const LastBatchHashKey = "l"

// dataHashForEmptyTxs to be used while only syncing headers from DA and no p2p to get the Data for no txs scenarios, the syncing can proceed without getting stuck forever.
var dataHashForEmptyTxs = []byte{110, 52, 11, 156, 255, 179, 122, 152, 156, 165, 68, 230, 187, 120, 10, 44, 120, 144, 29, 63, 179, 55, 56, 118, 133, 17, 163, 6, 23, 175, 160, 29}

// ErrNoBatch indicate no batch is available for creating block
var ErrNoBatch = errors.New("no batch to process")

// ErrHeightFromFutureStr is the error message for height from future returned by da
var ErrHeightFromFutureStr = "given height is from the future"

// NewHeaderEvent is used to pass header and DA height to headerInCh
type NewHeaderEvent struct {
	Header   *types.SignedHeader
	DAHeight uint64
}

// NewDataEvent is used to pass header and DA height to headerInCh
type NewDataEvent struct {
	Data     *types.Data
	DAHeight uint64
}

// BatchWithTime is used to pass batch and time to BatchQueue
type BatchWithTime struct {
	*coresequencer.Batch
	time.Time
}

// Manager orchestrates block creation, validation, and synchronization
type Manager struct {
	// Core components
	state    *StateManager
	sync     *SyncManager
	creator  *BlockCreator
	executor *ExecutionManager
	da       *DAManager

	// Configuration
	conf    config.BlockManagerConfig
	genesis *RollkitGenesis

	// Channels for communication
	HeaderCh   chan NewHeaderEvent
	DataCh     chan NewDataEvent
	retrieveCh chan struct{}

	// Core services
	logger     log.Logger
	metrics    *Metrics
	isProposer bool

	// State for lazy aggregation
	buildingBlock bool
	lastBatchHash []byte
	batchQueue    *BatchQueue
	sequencer     coresequencer.Sequencer
}

// StateManager handles all state-related operations
type StateManager struct {
	store        store.Store
	currentState types.State
	stateMtx     *sync.RWMutex
}

// getCurrentState returns the current state
func (sm *StateManager) getCurrentState() types.State {
	sm.stateMtx.RLock()
	defer sm.stateMtx.RUnlock()
	return sm.currentState
}

// getHeight returns the current height from store
func (sm *StateManager) getHeight() uint64 {
	return sm.store.Height()
}

// updateState updates both the in-memory and persisted state
func (sm *StateManager) updateState(ctx context.Context, newState types.State) error {
	sm.stateMtx.Lock()
	defer sm.stateMtx.Unlock()
	if err := sm.store.UpdateState(ctx, newState); err != nil {
		return err
	}
	sm.currentState = newState
	return nil
}

// SyncManager handles block synchronization
type SyncManager struct {
	headerStore    *goheaderstore.Store[*types.SignedHeader]
	dataStore      *goheaderstore.Store[*types.Data]
	headerCache    *HeaderCache
	dataCache      *DataCache
	pendingHeaders *PendingHeaders
	headerStoreCh  chan struct{}
	dataStoreCh    chan struct{}
}

// checkPendingBlocksLimit checks if we've reached the pending blocks limit
func (sm *SyncManager) checkPendingBlocksLimit(maxPendingBlocks uint64) error {
	if maxPendingBlocks != 0 && sm.pendingHeaders.numPendingHeaders() >= maxPendingBlocks {
		return fmt.Errorf("refusing to create block: pending blocks [%d] reached limit [%d]",
			sm.pendingHeaders.numPendingHeaders(), maxPendingBlocks)
	}
	return nil
}

// BlockCreator handles block creation and validation
type BlockCreator struct {
	proposerKey     crypto.PrivKey
	proposerAddress []byte
	batchQueue      *BatchQueue
	sequencer       coresequencer.Sequencer
}

// createBlock creates a new block with the given parameters
func (bc *BlockCreator) createBlock(ctx context.Context, height uint64, state types.State, exec *ExecutionManager) (*types.SignedHeader, *types.Data, error) {
	// Get transactions from the batch queue
	rawTxs := []execTypes.Tx{}
	if bc.batchQueue != nil && bc.sequencer != nil {
		// If we have a batch queue and sequencer, get transactions from there
		batch := bc.batchQueue.Next()
		if batch != nil {
			for _, tx := range batch.Transactions {
				rawTxs = append(rawTxs, execTypes.Tx(tx))
			}
		}
	}

	// Create header
	header := &types.SignedHeader{
		Header: types.Header{
			Version: types.Version{
				Block: state.Version.Consensus.Block,
				App:   state.Version.Consensus.App,
			},
			BaseHeader: types.BaseHeader{
				ChainID: state.ChainID,
				Height:  height,
				Time:    uint64(time.Now().UnixNano()), // Use current time
			},
			DataHash:        make(types.Hash, 32),
			ConsensusHash:   make(types.Hash, 32),
			AppHash:         state.AppHash,
			ProposerAddress: bc.proposerAddress,
		},
		// The signature will be computed later when the block is signed
		Signature: types.Signature{},
	}

	// Create data
	data := &types.Data{
		Txs: make(types.Txs, len(rawTxs)),
	}
	for i := range rawTxs {
		data.Txs[i] = types.Tx(rawTxs[i])
	}

	return header, data, nil
}

// ExecutionManager handles interaction with execution layer
type ExecutionManager struct {
	executor coreexecutor.Executor
}

// execValidate validates the block
func (em *ExecutionManager) execValidate(state types.State, header *types.SignedHeader, data *types.Data) error {
	// TODO: Implement validation logic
	// Basic validation checks

	// 1. Check that the header height is valid (greater than or equal to the last block height)
	if header.Height() <= state.LastBlockHeight {
		return fmt.Errorf("invalid header height: got %d, expected > %d", header.Height(), state.LastBlockHeight)
	}

	// 2. Check that the header chain ID matches the state chain ID
	if header.ChainID() != state.ChainID {
		return fmt.Errorf("invalid header chain ID: got %s, expected %s", header.ChainID(), state.ChainID)
	}

	// 3. Check that the header time is after the last block time
	headerTime := header.Time()
	if headerTime.Before(state.LastBlockTime) || headerTime.Equal(state.LastBlockTime) {
		return fmt.Errorf("invalid header time: got %v, expected > %v", headerTime, state.LastBlockTime)
	}

	// 4. Verify the signature if needed
	// This would require access to the proposer's public key

	return nil
}

// execApplyBlock applies the block
func (em *ExecutionManager) execApplyBlock(ctx context.Context, state types.State, header *types.SignedHeader, data *types.Data) (types.State, *abci.ResponseFinalizeBlock, error) {
	// TODO: Implement block application logic

	// First validate the block
	if err := em.execValidate(state, header, data); err != nil {
		return types.State{}, nil, err
	}

	// Convert data.Txs to raw byte slices for execution
	rawTxs := make([][]byte, len(data.Txs))
	for i := range data.Txs {
		rawTxs[i] = data.Txs[i]
	}

	// Execute the transactions
	newStateRoot, err := em.execExecuteTxs(ctx, rawTxs, header.Height(), header.Time(), state.AppHash)
	if err != nil {
		return types.State{}, nil, err
	}

	// Create the new state
	newState := types.State{
		Version:         state.Version,
		ChainID:         state.ChainID,
		InitialHeight:   state.InitialHeight,
		LastBlockHeight: header.Height(),
		LastBlockTime:   header.Time(),
		LastBlockID: cmtypes.BlockID{
			Hash: cmtbytes.HexBytes(header.Hash()),
			// for now, we don't care about part set headers
		},
		AppHash: newStateRoot,
	}

	// For now, return nil for ResponseFinalizeBlock
	// In a real implementation, this would contain the results of executing the transactions
	return newState, nil, nil
}

// execCommit commits the block
func (em *ExecutionManager) execCommit(ctx context.Context, state types.State, header *types.SignedHeader, data *types.Data, responses *abci.ResponseFinalizeBlock) ([]byte, error) {
	err := em.executor.SetFinal(ctx, header.Height())
	return state.AppHash, err
}

// execCreateBlock creates a new block
func (em *ExecutionManager) execCreateBlock(ctx context.Context, height uint64, lastSignature *types.Signature, extendedCommit abci.ExtendedCommitInfo, lastHeaderHash types.Hash, lastState types.State, txs cmtypes.Txs, timestamp time.Time) (*types.SignedHeader, *types.Data, error) {
	// Convert cmtypes.Txs to execTypes.Tx
	rawTxs := make([]execTypes.Tx, len(txs))
	for i := range txs {
		rawTxs[i] = execTypes.Tx(txs[i])
	}

	// Create header
	header := &types.SignedHeader{
		Header: types.Header{
			Version: types.Version{
				Block: lastState.Version.Consensus.Block,
				App:   lastState.Version.Consensus.App,
			},
			BaseHeader: types.BaseHeader{
				ChainID: lastState.ChainID,
				Height:  height,
				Time:    uint64(timestamp.UnixNano()), //nolint:gosec
			},
			DataHash:      make(types.Hash, 32),
			ConsensusHash: make(types.Hash, 32),
			AppHash:       lastState.AppHash,
			// Note: ProposerAddress will be set by the Manager
		},
		Signature: *lastSignature,
	}

	// Create data
	data := &types.Data{
		Txs: make(types.Txs, len(txs)),
		// IntermediateStateRoots: types.IntermediateStateRoots{RawRootsList: nil},
		// Note: Temporarily remove Evidence #896
		// Evidence:               types.EvidenceData{Evidence: nil},
	}
	for i := range txs {
		data.Txs[i] = types.Tx(txs[i])
	}

	return header, data, nil
}

// execSetFinal sets the block as final in the execution layer
func (em *ExecutionManager) execSetFinal(ctx context.Context, height uint64) error {
	return em.executor.SetFinal(ctx, height)
}

// execExecuteTxs executes transactions in the execution layer
func (em *ExecutionManager) execExecuteTxs(ctx context.Context, txs [][]byte, height uint64, timestamp time.Time, lastStateRoot []byte) ([]byte, error) {
	stateRoot, _, err := em.executor.ExecuteTxs(ctx, txs, height, timestamp, lastStateRoot)
	return stateRoot, err
}

// DAManager handles interaction with DA layer
type DAManager struct {
	client    *da.DAClient
	daHeight  uint64
	heightMtx sync.RWMutex
}

// RollkitGenesis is the genesis state of the rollup
type RollkitGenesis struct {
	GenesisTime     time.Time
	InitialHeight   uint64
	ChainID         string
	ProposerAddress []byte
}

// NewManager creates a new block Manager with separated concerns
func NewManager(
	ctx context.Context,
	proposerKey crypto.PrivKey,
	conf config.BlockManagerConfig,
	genesis *RollkitGenesis,
	store store.Store,
	exec coreexecutor.Executor,
	sequencer coresequencer.Sequencer,
	dalc *da.DAClient,
	logger log.Logger,
	headerStore *goheaderstore.Store[*types.SignedHeader],
	dataStore *goheaderstore.Store[*types.Data],
	seqMetrics *Metrics,
) (*Manager, error) {
	// Initialize state manager
	stateManager, err := newStateManager(ctx, genesis, store, exec, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create state manager: %w", err)
	}

	// Initialize sync manager
	pendingHeaders, err := NewPendingHeaders(store, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create pending headers: %w", err)
	}

	syncManager := &SyncManager{
		headerStore:    headerStore,
		dataStore:      dataStore,
		headerCache:    NewHeaderCache(),
		dataCache:      NewDataCache(),
		pendingHeaders: pendingHeaders,
		headerStoreCh:  make(chan struct{}, 1),
		dataStoreCh:    make(chan struct{}, 1),
	}

	// Initialize block creator
	blockCreator := &BlockCreator{
		proposerKey:     proposerKey,
		proposerAddress: genesis.ProposerAddress,
		batchQueue:      NewBatchQueue(),
		sequencer:       sequencer,
	}

	// Initialize execution manager
	execManager := &ExecutionManager{
		executor: exec,
	}

	// Initialize DA manager
	daManager := &DAManager{
		client:   dalc,
		daHeight: conf.DAStartHeight,
	}

	isProposer, err := isProposer(proposerKey, stateManager.currentState)
	if err != nil {
		return nil, fmt.Errorf("failed to check proposer status: %w", err)
	}

	m := &Manager{
		state:         stateManager,
		sync:          syncManager,
		creator:       blockCreator,
		executor:      execManager,
		da:            daManager,
		conf:          conf,
		genesis:       genesis,
		HeaderCh:      make(chan NewHeaderEvent, channelLength),
		DataCh:        make(chan NewDataEvent, channelLength),
		retrieveCh:    make(chan struct{}, 1),
		logger:        logger,
		metrics:       seqMetrics,
		isProposer:    isProposer,
		buildingBlock: false,
		batchQueue:    NewBatchQueue(),
		sequencer:     sequencer,
	}

	return m, nil
}

// newStateManager creates a new state manager
func newStateManager(ctx context.Context, genesis *RollkitGenesis, store store.Store, exec coreexecutor.Executor, logger log.Logger) (*StateManager, error) {
	initialState, err := getInitialState(ctx, genesis, store, exec, logger)
	if err != nil {
		return nil, err
	}

	return &StateManager{
		store:        store,
		currentState: initialState,
		stateMtx:     new(sync.RWMutex),
	}, nil
}

// publishBlock creates and publishes a new block
func (m *Manager) publishBlock(ctx context.Context) error {
	if !m.isProposer {
		return ErrNotProposer
	}

	if err := m.sync.checkPendingBlocksLimit(m.conf.MaxPendingBlocks); err != nil {
		return err
	}

	// Get current state and height
	state := m.state.getCurrentState()
	height := m.state.getHeight()
	newHeight := height + 1

	// Create block
	header, data, err := m.creator.createBlock(ctx, newHeight, state, m.executor)
	if err != nil {
		return fmt.Errorf("failed to create block: %w", err)
	}

	// Apply and validate block
	newState, _, err := m.executor.execApplyBlock(ctx, state, header, data)
	if err != nil {
		return fmt.Errorf("failed to apply block: %w", err)
	}

	// Update state and store
	if err := m.state.updateState(ctx, newState); err != nil {
		return fmt.Errorf("failed to update state: %w", err)
	}

	// Record metrics and broadcast
	m.recordMetrics(data)
	m.broadcastBlock(header, data)

	return nil
}

// broadcastBlock sends the block to appropriate channels
func (m *Manager) broadcastBlock(header *types.SignedHeader, data *types.Data) {
	daHeight := m.da.daHeight
	select {
	case m.HeaderCh <- NewHeaderEvent{Header: header, DAHeight: daHeight}:
	default:
		m.logger.Error("failed to broadcast header - channel full")
	}

	select {
	case m.DataCh <- NewDataEvent{Data: data, DAHeight: daHeight}:
	default:
		m.logger.Error("failed to broadcast data - channel full")
	}
}

// DALCInitialized returns true if DALC is initialized.
func (m *Manager) DALCInitialized() bool {
	return m.da.client != nil
}

// PendingHeaders returns the pending headers.
func (m *Manager) PendingHeaders() *PendingHeaders {
	return m.sync.pendingHeaders
}

// IsProposer returns true if the manager is acting as proposer.
func (m *Manager) IsProposer() bool {
	return m.isProposer
}

// SeqClient returns the grpc sequencing client.
func (m *Manager) SeqClient() coresequencer.Sequencer {
	return m.sequencer
}

// GetLastState returns the last recorded state.
func (m *Manager) GetLastState() types.State {
	return m.state.getCurrentState()
}

func (m *Manager) init(ctx context.Context) {
	// initialize da included height
	if height, err := m.state.store.GetMetadata(ctx, DAIncludedHeightKey); err == nil && len(height) == 8 {
		m.da.daHeight = binary.BigEndian.Uint64(height)
	}
}

func (m *Manager) setDAIncludedHeight(ctx context.Context, newHeight uint64) error {
	for {
		currentHeight := m.da.daHeight
		if newHeight <= currentHeight {
			break
		}
		if atomic.CompareAndSwapUint64(&m.da.daHeight, currentHeight, newHeight) {
			heightBytes := make([]byte, 8)
			binary.BigEndian.PutUint64(heightBytes, newHeight)
			return m.state.store.SetMetadata(ctx, DAIncludedHeightKey, heightBytes)
		}
	}
	return nil
}

// GetDAIncludedHeight returns the rollup height at which all blocks have been
// included in the DA
func (m *Manager) GetDAIncludedHeight() uint64 {
	return m.da.daHeight
}

// SetDALC is used to set DataAvailabilityLayerClient used by Manager.
func (m *Manager) SetDALC(dalc *da.DAClient) {
	m.da.client = dalc
}

// isProposer returns whether or not the manager is a proposer
func isProposer(_ crypto.PrivKey, _ types.State) (bool, error) {
	return true, nil
}

// SetLastState is used to set lastState used by Manager.
func (m *Manager) SetLastState(state types.State) {
	m.state.stateMtx.Lock()
	defer m.state.stateMtx.Unlock()
	m.state.currentState = state
}

// GetStoreHeight returns the manager's store height
func (m *Manager) GetStoreHeight() uint64 {
	return m.state.store.Height()
}

// GetHeaderInCh returns the manager's blockInCh
func (m *Manager) GetHeaderInCh() chan NewHeaderEvent {
	// Create a new channel for header events
	headerCh := make(chan NewHeaderEvent, channelLength)

	// Start a goroutine to convert SignedHeader to NewHeaderEvent
	go func() {
		for header := range m.HeaderCh {
			headerCh <- header
		}
		close(headerCh)
	}()

	return headerCh
}

// GetDataInCh returns the manager's dataInCh
func (m *Manager) GetDataInCh() chan NewDataEvent {
	// Create a new channel for data events
	dataCh := make(chan NewDataEvent, channelLength)

	// Start a goroutine to convert Data to NewDataEvent
	go func() {
		for data := range m.DataCh {
			dataCh <- data
		}
		close(dataCh)
	}()

	return dataCh
}

// IsBlockHashSeen returns true if the block with the given hash has been seen.
func (m *Manager) IsBlockHashSeen(blockHash string) bool {
	return m.sync.headerCache.isSeen(blockHash)
}

// IsDAIncluded returns true if the block with the given hash has been seen on DA.
func (m *Manager) IsDAIncluded(hash types.Hash) bool {
	return m.sync.headerCache.isDAIncluded(hash.String())
}

// getRemainingSleep calculates the remaining sleep time based on config and a start time.
func (m *Manager) getRemainingSleep(start time.Time) time.Duration {
	elapsed := time.Since(start)
	interval := m.conf.BlockTime

	if m.conf.LazyAggregator {
		if m.buildingBlock && elapsed >= interval {
			// Special case to give time for transactions to accumulate if we
			// are coming out of a period of inactivity.
			return (interval * time.Duration(defaultLazySleepPercent) / 100)
		} else if !m.buildingBlock {
			interval = m.conf.LazyBlockTime
		}
	}

	if elapsed < interval {
		return interval - elapsed
	}

	return 0
}

// GetExecutor returns the executor used by the manager.
//
// Note: this is a temporary method to allow testing the manager.
// It will be removed once the manager is fully integrated with the execution client.
func (m *Manager) GetExecutor() coreexecutor.Executor {
	return m.executor.executor
}

// BatchRetrieveLoop is responsible for retrieving batches from the sequencer.
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
				batchTimer.Reset(m.conf.BlockTime)
				continue
			}

			if res != nil && res.Batch != nil {
				m.logger.Debug("Retrieved batch",
					"txCount", len(res.Batch.Transactions),
					"timestamp", res.Timestamp)

				if h, err := res.Batch.Hash(); err == nil {
					m.batchQueue.AddBatch(BatchWithTime{Batch: res.Batch, Time: res.Timestamp})
					if len(res.Batch.Transactions) != 0 {
						if err := m.state.store.SetMetadata(ctx, LastBatchHashKey, h); err != nil {
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
			remainingSleep := m.conf.BlockTime - elapsed
			if remainingSleep < 0 {
				remainingSleep = 0
			}
			batchTimer.Reset(remainingSleep)
		}
	}
}

// AggregationLoop is responsible for aggregating transactions into rollup-blocks.
func (m *Manager) AggregationLoop(ctx context.Context) {
	initialHeight := m.genesis.InitialHeight //nolint:gosec
	height := m.state.store.Height()
	var delay time.Duration

	// TODO(tzdybal): double-check when https://github.com/celestiaorg/rollmint/issues/699 is resolved
	if height < initialHeight {
		delay = time.Until(m.genesis.GenesisTime)
	} else {
		lastBlockTime := m.getLastBlockTime()
		delay = time.Until(lastBlockTime.Add(m.conf.BlockTime))
	}

	if delay > 0 {
		m.logger.Info("Waiting to produce block", "delay", delay)
		time.Sleep(delay)
	}

	// blockTimer is used to signal when to build a block based on the
	// rollup block time. A timer is used so that the time to build a block
	// can be taken into account.
	blockTimer := time.NewTimer(0)
	defer blockTimer.Stop()

	// Check sequencing mode from configuration
	switch m.conf.SequencingMode {
	case "based":
		m.logger.Info("Starting based sequencing mode")
		m.basedSequencingLoop(ctx)
		return
	case "lazy":
		m.logger.Info("Starting lazy sequencing mode")
		m.lazyAggregationLoop(ctx, blockTimer)
		return
	case "normal":
		m.logger.Info("Starting normal sequencing mode")
		m.normalAggregationLoop(ctx, blockTimer)
		return
	default:
		// For backward compatibility, check the LazyAggregator flag
		if m.conf.LazyAggregator {
			m.logger.Info("Starting lazy sequencing mode (legacy config)")
			m.lazyAggregationLoop(ctx, blockTimer)
			return
		}

		// Default to normal mode
		m.logger.Info("Starting normal sequencing mode (default)")
		m.normalAggregationLoop(ctx, blockTimer)
	}
}

func (m *Manager) lazyAggregationLoop(ctx context.Context, blockTimer *time.Timer) {
	// start is used to track the start time of the block production period
	start := time.Now()
	// lazyTimer is used to signal when a block should be built in
	// lazy mode to signal that the chain is still live during long
	// periods of inactivity.
	lazyTimer := time.NewTimer(0)
	defer lazyTimer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		// the m.batchQueue.notifyCh channel is signalled when batch becomes available in the batch queue
		case _, ok := <-m.batchQueue.notifyCh:
			if ok && !m.buildingBlock {
				// set the buildingBlock flag to prevent multiple calls to reset the time
				m.buildingBlock = true
				// Reset the block timer based on the block time.
				blockTimer.Reset(m.getRemainingSleep(start))
			}
			continue
		case <-lazyTimer.C:
		case <-blockTimer.C:
		}
		// Define the start time for the block production period
		start = time.Now()
		if err := m.publishBlock(ctx); err != nil && ctx.Err() == nil {
			m.logger.Error("error while publishing block", "error", err)
		}
		// unset the buildingBlocks flag
		m.buildingBlock = false
		// Reset the lazyTimer to produce a block even if there
		// are no transactions as a way to signal that the chain
		// is still live.
		lazyTimer.Reset(m.getRemainingSleep(start))
	}
}

func (m *Manager) normalAggregationLoop(ctx context.Context, blockTimer *time.Timer) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-blockTimer.C:
			// Define the start time for the block production period
			start := time.Now()
			if err := m.publishBlock(ctx); err != nil && ctx.Err() == nil {
				m.logger.Error("error while publishing block", "error", err)
			}
			// Reset the blockTimer to signal the next block production
			// period based on the block time.
			blockTimer.Reset(m.getRemainingSleep(start))
		}
	}
}

// HeaderSubmissionLoop is responsible for submitting blocks to the DA layer.
func (m *Manager) HeaderSubmissionLoop(ctx context.Context) {
	timer := time.NewTicker(m.conf.DABlockTime)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		if m.sync.pendingHeaders.isEmpty() {
			continue
		}
		err := m.submitHeadersToDA(ctx)
		if err != nil {
			m.logger.Error("error while submitting block to DA", "error", err)
		}
	}
}

func (m *Manager) handleEmptyDataHash(ctx context.Context, header *types.Header) {
	headerHeight := header.Height()
	if bytes.Equal(header.DataHash, dataHashForEmptyTxs) {
		var lastDataHash types.Hash
		var err error
		var lastData *types.Data
		if headerHeight > 1 {
			_, lastData, err = m.state.store.GetBlockData(ctx, headerHeight-1)
			if lastData != nil {
				lastDataHash = lastData.Hash()
			}
		}
		// if no error then populate data, otherwise just skip and wait for Data to be synced
		if err == nil {
			metadata := &types.Metadata{
				ChainID:      header.ChainID(),
				Height:       headerHeight,
				Time:         header.BaseHeader.Time,
				LastDataHash: lastDataHash,
			}
			d := &types.Data{
				Metadata: metadata,
			}
			m.sync.dataCache.setData(headerHeight, d)
		}
	}
}

// SyncLoop processes headers gossiped in P2P network
func (m *Manager) SyncLoop(ctx context.Context) {
	daTicker := time.NewTicker(m.conf.DABlockTime)
	defer daTicker.Stop()
	blockTicker := time.NewTicker(m.conf.BlockTime)
	defer blockTicker.Stop()
	for {
		select {
		case <-daTicker.C:
			m.sendNonBlockingSignalToRetrieveCh()
		case <-blockTicker.C:
			m.sendNonBlockingSignalToHeaderStoreCh()
			m.sendNonBlockingSignalToDataStoreCh()
		case headerEvent := <-m.HeaderCh:
			header := headerEvent.Header
			daHeight := headerEvent.DAHeight
			headerHash := header.Hash().String()
			headerHeight := header.Height()
			m.logger.Debug("header retrieved",
				"height", headerHash,
				"daHeight", daHeight,
				"hash", headerHash,
			)
			if headerHeight <= m.state.store.Height() || m.sync.headerCache.isSeen(headerHash) {
				m.logger.Debug("header already seen", "height", headerHeight, "block hash", headerHash)
				continue
			}
			m.sync.headerCache.setHeader(headerHeight, header)

			m.sendNonBlockingSignalToHeaderStoreCh()
			m.sendNonBlockingSignalToRetrieveCh()

			// check if the dataHash is dataHashForEmptyTxs
			m.handleEmptyDataHash(ctx, &header.Header)

			err := m.trySyncNextBlock(ctx, daHeight)
			if err != nil {
				m.logger.Info("failed to sync next block", "error", err)
				continue
			}
			m.sync.headerCache.setSeen(headerHash)
		case dataEvent := <-m.DataCh:
			data := dataEvent.Data
			daHeight := dataEvent.DAHeight
			dataHash := data.Hash().String()
			dataHeight := data.Metadata.Height
			m.logger.Debug("data retrieved",
				"height", dataHash,
				"daHeight", daHeight,
				"hash", dataHash,
			)
			if dataHeight <= m.state.store.Height() || m.sync.dataCache.isSeen(dataHash) {
				m.logger.Debug("data already seen", "height", dataHeight, "data hash", dataHash)
				continue
			}
			m.sync.dataCache.setData(dataHeight, data)

			m.sendNonBlockingSignalToDataStoreCh()

			err := m.trySyncNextBlock(ctx, daHeight)
			if err != nil {
				m.logger.Info("failed to sync next block", "error", err)
				continue
			}
			m.sync.dataCache.setSeen(dataHash)
		case <-ctx.Done():
			return
		}
	}
}

func (m *Manager) sendNonBlockingSignalToHeaderStoreCh() {
	select {
	case m.sync.headerStoreCh <- struct{}{}:
	default:
	}
}

func (m *Manager) sendNonBlockingSignalToDataStoreCh() {
	select {
	case m.sync.dataStoreCh <- struct{}{}:
	default:
	}
}

func (m *Manager) sendNonBlockingSignalToRetrieveCh() {
	select {
	case m.retrieveCh <- struct{}{}:
	default:
	}
}

// handleHeader processes a header from DA
// If isDirect is true, the header was retrieved directly from DA
// (as opposed to being retrieved from P2P)
func (m *Manager) handleHeader(ctx context.Context, header *types.SignedHeader, daHeight uint64, isDirect bool) error {
	headerHash := header.Hash().String()
	headerHeight := header.Height()

	// Mark header as seen
	m.sync.headerCache.setSeen(headerHash)

	// Store header in cache
	m.sync.headerCache.setHeader(headerHeight, header)

	// If direct from DA, mark as DA included
	if isDirect {
		m.sync.headerCache.setDAIncluded(headerHash)
	}

	// Send header to the HeaderCh channel
	m.HeaderCh <- NewHeaderEvent{
		Header:   header,
		DAHeight: daHeight,
	}

	m.logger.Debug("header handled",
		"height", headerHeight,
		"hash", headerHash,
		"from_da", isDirect,
	)

	return nil
}

// trySyncNextBlock attempts to sync blocks from DA
func (m *Manager) trySyncNextBlock(ctx context.Context, daHeight uint64) error {
	// Check if we have a valid DALC
	if !m.DALCInitialized() {
		return errors.New("DA client not initialized")
	}

	// Fetch headers from DA height
	result, err := m.fetchHeaders(ctx, daHeight)
	if err != nil {
		return fmt.Errorf("failed to fetch headers from DA height %d: %w", daHeight, err)
	}

	// If there are no headers (unlikely, since result.Code would be StatusNotFound)
	// or the code is not success, just increment DA height and return
	if len(result.Headers) == 0 || result.Code != da.StatusSuccess {
		err = m.setDAIncludedHeight(ctx, daHeight)
		if err != nil {
			m.logger.Error("error while setting DA included height",
				"height", daHeight,
				"err", err,
			)
		}
		return nil
	}

	// Process the retrieved headers
	for _, header := range result.Headers {
		headerHash := header.Hash().String()
		headerHeight := header.Height()

		// Check if the header is already seen
		if m.sync.headerCache.isSeen(headerHash) {
			// Mark the header as DA included
			if !m.sync.headerCache.isDAIncluded(headerHash) {
				m.sync.headerCache.setDAIncluded(headerHash)

				// Notify other components about DA inclusion
				m.logger.Debug("marking block as DA included", "height", headerHeight, "hash", headerHash)
			}
			continue
		}

		// Attempt to handle the header
		err = m.handleHeader(ctx, header, daHeight, true)
		if err != nil {
			m.logger.Error("error while handling header from DA",
				"err", err,
				"height", headerHeight,
				"hash", headerHash,
			)
			return fmt.Errorf("error while handling header from DA: %w", err)
		}

		// Check if this is a based sequencing batch
		if err := m.tryHandleBasedSequencingBatch(ctx, header, daHeight); err != nil {
			m.logger.Error("error handling based sequencing batch",
				"err", err,
				"height", headerHeight,
				"hash", headerHash,
			)
			// We continue processing even if a based batch fails
			// as it might be a regular header, not a based batch
		}
	}

	// Update the DA included height
	err = m.setDAIncludedHeight(ctx, daHeight)
	if err != nil {
		m.logger.Error("error while setting DA included height",
			"height", daHeight,
			"err", err,
		)
		return err
	}

	return nil
}

// tryHandleBasedSequencingBatch attempts to handle a header as a based sequencing batch
// This is specifically for batches that were posted directly to DA via based sequencing
func (m *Manager) tryHandleBasedSequencingBatch(ctx context.Context, header *types.SignedHeader, daHeight uint64) error {
	headerHeight := header.Height()
	headerHash := header.Hash().String()

	// Check if we need to retrieve the batch data
	// We need to check if this is actually a based sequencing batch
	// by looking at the DataHash which should point to batch data in DA

	// For now, we always try to retrieve potential batch data for all headers
	// In a production implementation, we might have a way to identify based batches

	// Retrieve the batch data from DA
	// This would need additional DA client method to fetch arbitrary data by reference
	// Since we don't have that yet, this is a placeholder
	data, err := m.retrieveBatchDataFromDA(ctx, header, daHeight)
	if err != nil {
		// This might not be a based batch, so we log but don't error
		m.logger.Debug("Failed to retrieve batch data, might not be a based batch",
			"height", headerHeight,
			"hash", headerHash,
			"err", err,
		)
		return nil
	}

	if data == nil {
		// Not a based batch or no data found
		return nil
	}

	// Apply the batch data
	m.logger.Info("Applying based sequencing batch",
		"height", headerHeight,
		"hash", headerHash,
		"txs", len(data.Txs),
	)

	// Apply the batch similarly to how we apply regular blocks
	state, _, err := m.applyBlock(ctx, header, data)
	if err != nil {
		return fmt.Errorf("failed to apply based batch: %w", err)
	}

	// Update state with the new batch data applied
	err = m.updateState(ctx, state)
	if err != nil {
		return fmt.Errorf("failed to update state after batch application: %w", err)
	}

	return nil
}

// retrieveBatchDataFromDA is a placeholder for retrieving batch data from DA
// In a real implementation, this would use the DA client to fetch the data
// associated with the header's DataHash or other reference
func (m *Manager) retrieveBatchDataFromDA(ctx context.Context, header *types.SignedHeader, daHeight uint64) (*types.Data, error) {
	// This is a placeholder implementation
	// In a real implementation, this would:
	// 1. Use the DA client to fetch the data blob using header.DataHash as reference
	// 2. Decode the blob to extract the Data portion
	// 3. Return it for processing

	// For now, we return nil to indicate no batch data was found
	// This will need to be implemented properly when the DA client
	// supports retrieving arbitrary data
	return nil, nil
}

// HeaderStoreRetrieveLoop is responsible for retrieving headers from the Header Store.
func (m *Manager) HeaderStoreRetrieveLoop(ctx context.Context) {
	lastHeaderStoreHeight := uint64(0)
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.sync.headerStoreCh:
		}
		headerStoreHeight := m.sync.headerStore.Height()
		if headerStoreHeight > lastHeaderStoreHeight {
			headers, err := m.getHeadersFromHeaderStore(ctx, lastHeaderStoreHeight+1, headerStoreHeight)
			if err != nil {
				m.logger.Error("failed to get headers from Header Store", "lastHeaderHeight", lastHeaderStoreHeight, "headerStoreHeight", headerStoreHeight, "errors", err.Error())
				continue
			}
			daHeight := atomic.LoadUint64(&m.da.daHeight)
			for _, header := range headers {
				// Check for shut down event prior to logging
				// and sending header to headerInCh. The reason
				// for checking for the shutdown event
				// separately is due to the inconsistent nature
				// of the select statement when multiple cases
				// are satisfied.
				select {
				case <-ctx.Done():
					return
				default:
				}
				// early validation to reject junk headers
				if !m.isUsingExpectedCentralizedSequencer(header) {
					continue
				}
				m.logger.Debug("header retrieved from p2p header sync", "headerHeight", header.Height(), "daHeight", daHeight)
				m.HeaderCh <- NewHeaderEvent{header, daHeight}
			}
		}
		lastHeaderStoreHeight = headerStoreHeight
	}
}

// DataStoreRetrieveLoop is responsible for retrieving data from the Data Store.
func (m *Manager) DataStoreRetrieveLoop(ctx context.Context) {
	lastDataStoreHeight := uint64(0)
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.sync.dataStoreCh:
		}
		dataStoreHeight := m.sync.dataStore.Height()
		if dataStoreHeight > lastDataStoreHeight {
			data, err := m.getDataFromDataStore(ctx, lastDataStoreHeight+1, dataStoreHeight)
			if err != nil {
				m.logger.Error("failed to get data from Data Store", "lastDataStoreHeight", lastDataStoreHeight, "dataStoreHeight", dataStoreHeight, "errors", err.Error())
				continue
			}
			daHeight := atomic.LoadUint64(&m.da.daHeight)
			for _, d := range data {
				// Check for shut down event prior to logging
				// and sending header to dataInCh. The reason
				// for checking for the shutdown event
				// separately is due to the inconsistent nature
				// of the select statement when multiple cases
				// are satisfied.
				select {
				case <-ctx.Done():
					return
				default:
				}
				//TODO: remove junk if possible
				m.logger.Debug("data retrieved from p2p data sync", "dataHeight", d.Metadata.Height, "daHeight", daHeight)
				m.DataCh <- NewDataEvent{d, daHeight}
			}
		}
		lastDataStoreHeight = dataStoreHeight
	}
}

func (m *Manager) getHeadersFromHeaderStore(ctx context.Context, startHeight, endHeight uint64) ([]*types.SignedHeader, error) {
	if startHeight > endHeight {
		return nil, fmt.Errorf("startHeight (%d) is greater than endHeight (%d)", startHeight, endHeight)
	}
	headers := make([]*types.SignedHeader, endHeight-startHeight+1)
	for i := startHeight; i <= endHeight; i++ {
		header, err := m.sync.headerStore.GetByHeight(ctx, i)
		if err != nil {
			return nil, err
		}
		headers[i-startHeight] = header
	}
	return headers, nil
}

func (m *Manager) getDataFromDataStore(ctx context.Context, startHeight, endHeight uint64) ([]*types.Data, error) {
	if startHeight > endHeight {
		return nil, fmt.Errorf("startHeight (%d) is greater than endHeight (%d)", startHeight, endHeight)
	}
	data := make([]*types.Data, endHeight-startHeight+1)
	for i := startHeight; i <= endHeight; i++ {
		d, err := m.sync.dataStore.GetByHeight(ctx, i)
		if err != nil {
			return nil, err
		}
		data[i-startHeight] = d
	}
	return data, nil
}

// RetrieveLoop is responsible for interacting with DA layer.
func (m *Manager) RetrieveLoop(ctx context.Context) {
	// blockFoundCh is used to track when we successfully found a block so
	// that we can continue to try and find blocks that are in the next DA height.
	// This enables syncing faster than the DA block time.
	headerFoundCh := make(chan struct{}, 1)
	defer close(headerFoundCh)
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.retrieveCh:
		case <-headerFoundCh:
		}
		daHeight := atomic.LoadUint64(&m.da.daHeight)
		err := m.processNextDAHeader(ctx)
		if err != nil && ctx.Err() == nil {
			// if the requested da height is not yet available, wait silently, otherwise log the error and wait
			if !strings.Contains(err.Error(), ErrHeightFromFutureStr) {
				m.logger.Error("failed to retrieve block from DALC", "daHeight", daHeight, "errors", err.Error())
			}
			continue
		}
		// Signal the headerFoundCh to try and retrieve the next block
		select {
		case headerFoundCh <- struct{}{}:
		default:
		}
		atomic.AddUint64(&m.da.daHeight, 1)
	}
}

func (m *Manager) processNextDAHeader(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// TODO(tzdybal): extract configuration option
	maxRetries := 10
	daHeight := atomic.LoadUint64(&m.da.daHeight)

	var err error
	m.logger.Debug("trying to retrieve block from DA", "daHeight", daHeight)
	for r := 0; r < maxRetries; r++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		headerResp, fetchErr := m.fetchHeaders(ctx, daHeight)
		if fetchErr == nil {
			if headerResp.Code == da.StatusNotFound {
				m.logger.Debug("no header found", "daHeight", daHeight, "reason", headerResp.Message)
				return nil
			}
			m.logger.Debug("retrieved potential headers", "n", len(headerResp.Headers), "daHeight", daHeight)
			for _, header := range headerResp.Headers {
				// early validation to reject junk headers
				if !m.isUsingExpectedCentralizedSequencer(header) {
					m.logger.Debug("skipping header from unexpected sequencer",
						"headerHeight", header.Height(),
						"headerHash", header.Hash().String())
					continue
				}
				blockHash := header.Hash().String()
				m.sync.headerCache.setDAIncluded(blockHash)
				err = m.setDAIncludedHeight(ctx, header.Height())
				if err != nil {
					return err
				}
				m.logger.Info("block marked as DA included", "blockHeight", header.Height(), "blockHash", blockHash)
				if !m.sync.headerCache.isSeen(blockHash) {
					// Check for shut down event prior to logging
					// and sending block to blockInCh. The reason
					// for checking for the shutdown event
					// separately is due to the inconsistent nature
					// of the select statement when multiple cases
					// are satisfied.
					select {
					case <-ctx.Done():
						return fmt.Errorf("unable to send block to blockInCh, context done: %w", ctx.Err())
					default:
					}
					m.HeaderCh <- NewHeaderEvent{header, daHeight}
				}
			}
			return nil
		}

		// Track the error
		err = errors.Join(err, fetchErr)
		// Delay before retrying
		select {
		case <-ctx.Done():
			return err
		case <-time.After(100 * time.Millisecond):
		}
	}
	return err
}

func (m *Manager) isUsingExpectedCentralizedSequencer(header *types.SignedHeader) bool {
	return bytes.Equal(header.ProposerAddress, m.genesis.ProposerAddress) && header.ValidateBasic() == nil
}

func (m *Manager) fetchHeaders(ctx context.Context, daHeight uint64) (da.ResultRetrieveHeaders, error) {
	var err error
	headerRes := m.da.client.RetrieveHeaders(ctx, daHeight)
	if headerRes.Code == da.StatusError {
		err = fmt.Errorf("failed to retrieve block: %s", headerRes.Message)
	}
	return headerRes, err
}

func (m *Manager) getSignature(header types.Header) (types.Signature, error) {
	b, err := header.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return m.creator.proposerKey.Sign(b)
}

func (m *Manager) getTxsFromBatch() (cmtypes.Txs, *time.Time, error) {
	batch := m.batchQueue.Next()
	if batch == nil {
		// batch is nil when there is nothing to process
		return nil, nil, ErrNoBatch
	}
	txs := make(cmtypes.Txs, 0, len(batch.Transactions))
	for _, tx := range batch.Transactions {
		txs = append(txs, tx)
	}
	return txs, &batch.Time, nil
}

func (m *Manager) recordMetrics(data *types.Data) {
	m.metrics.NumTxs.Set(float64(len(data.Txs)))
	m.metrics.TotalTxs.Add(float64(len(data.Txs)))
	m.metrics.BlockSizeBytes.Set(float64(data.Size()))
	m.metrics.CommittedHeight.Set(float64(data.Metadata.Height))
}

func (m *Manager) submitHeadersToDA(ctx context.Context) error {
	submittedAllHeaders := false
	var backoff time.Duration
	headersToSubmit, err := m.sync.pendingHeaders.getPendingHeaders(ctx)
	if len(headersToSubmit) == 0 {
		// There are no pending headers; return because there's nothing to do, but:
		// - it might be caused by error, then err != nil
		// - all pending headers are processed, then err == nil
		// whatever the reason, error information is propagated correctly to the caller
		return err
	}
	if err != nil {
		// There are some pending blocks but also an error. It's very unlikely case - probably some error while reading
		// headers from the store.
		// The error is logged and normal processing of pending blocks continues.
		m.logger.Error("error while fetching blocks pending DA", "err", err)
	}
	numSubmittedHeaders := 0
	attempt := 0
	maxBlobSize, err := m.da.client.DA.MaxBlobSize(ctx)
	if err != nil {
		return err
	}
	initialMaxBlobSize := maxBlobSize
	initialGasPrice := m.da.client.GasPrice
	gasPrice := m.da.client.GasPrice

daSubmitRetryLoop:
	for !submittedAllHeaders && attempt < maxSubmitAttempts {
		select {
		case <-ctx.Done():
			break daSubmitRetryLoop
		case <-time.After(backoff):
		}

		res := m.da.client.SubmitHeaders(ctx, headersToSubmit, maxBlobSize, gasPrice)
		switch res.Code {
		case da.StatusSuccess:
			m.logger.Info("successfully submitted Rollkit headers to DA layer", "gasPrice", gasPrice, "daHeight", res.DAHeight, "headerCount", res.SubmittedCount)
			if res.SubmittedCount == uint64(len(headersToSubmit)) {
				submittedAllHeaders = true
			}
			submittedBlocks, notSubmittedBlocks := headersToSubmit[:res.SubmittedCount], headersToSubmit[res.SubmittedCount:]
			numSubmittedHeaders += len(submittedBlocks)
			for _, block := range submittedBlocks {
				m.sync.headerCache.setDAIncluded(block.Hash().String())
				err = m.setDAIncludedHeight(ctx, block.Height())
				if err != nil {
					return err
				}
			}
			lastSubmittedHeight := uint64(0)
			if l := len(submittedBlocks); l > 0 {
				lastSubmittedHeight = submittedBlocks[l-1].Height()
			}
			m.sync.pendingHeaders.setLastSubmittedHeight(ctx, lastSubmittedHeight)
			headersToSubmit = notSubmittedBlocks
			// reset submission options when successful
			// scale back gasPrice gradually
			backoff = 0
			maxBlobSize = initialMaxBlobSize
			if m.da.client.GasMultiplier > 0 && gasPrice != -1 {
				gasPrice = gasPrice / m.da.client.GasMultiplier
				if gasPrice < initialGasPrice {
					gasPrice = initialGasPrice
				}
			}
			m.logger.Debug("resetting DA layer submission options", "backoff", backoff, "gasPrice", gasPrice, "maxBlobSize", maxBlobSize)
		case da.StatusNotIncludedInBlock, da.StatusAlreadyInMempool:
			m.logger.Error("DA layer submission failed", "error", res.Message, "attempt", attempt)
			backoff = m.conf.DABlockTime * time.Duration(m.conf.DAMempoolTTL) //nolint:gosec
			if m.da.client.GasMultiplier > 0 && gasPrice != -1 {
				gasPrice = gasPrice * m.da.client.GasMultiplier
			}
			m.logger.Info("retrying DA layer submission with", "backoff", backoff, "gasPrice", gasPrice, "maxBlobSize", maxBlobSize)

		case da.StatusTooBig:
			maxBlobSize = maxBlobSize / 4
			fallthrough
		default:
			m.logger.Error("DA layer submission failed", "error", res.Message, "attempt", attempt)
			backoff = m.exponentialBackoff(backoff)
		}

		attempt += 1
	}

	if !submittedAllHeaders {
		return fmt.Errorf(
			"failed to submit all blocks to DA layer, submitted %d blocks (%d left) after %d attempts",
			numSubmittedHeaders,
			len(headersToSubmit),
			attempt,
		)
	}
	return nil
}

func (m *Manager) exponentialBackoff(backoff time.Duration) time.Duration {
	backoff *= 2
	if backoff == 0 {
		backoff = initialBackoff
	}
	if backoff > m.conf.DABlockTime {
		backoff = m.conf.DABlockTime
	}
	return backoff
}

// Updates the state stored in manager's store along the manager's lastState
func (m *Manager) updateState(ctx context.Context, s types.State) error {
	m.logger.Debug("updating state", "newState", s)
	m.state.stateMtx.Lock()
	defer m.state.stateMtx.Unlock()
	err := m.state.store.UpdateState(ctx, s)
	if err != nil {
		return err
	}
	m.state.currentState = s
	m.metrics.Height.Set(float64(s.LastBlockHeight))
	return nil
}

func (m *Manager) getLastBlockTime() time.Time {
	m.state.stateMtx.RLock()
	defer m.state.stateMtx.RUnlock()
	return m.state.currentState.LastBlockTime
}

func (m *Manager) createBlock(ctx context.Context, height uint64, lastSignature *types.Signature, extendedCommit abci.ExtendedCommitInfo, lastHeaderHash types.Hash, lastState types.State, txs cmtypes.Txs, timestamp time.Time) (*types.SignedHeader, *types.Data, error) {
	m.state.stateMtx.RLock()
	defer m.state.stateMtx.RUnlock()

	// First, create a basic block/header with the BlockCreator
	header, data, err := m.creator.createBlock(ctx, height, lastState, m.executor)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create block with BlockCreator: %w", err)
	}

	// If we don't have transactions from the BlockCreator, try to get them from the batch queue
	if len(data.Txs) == 0 {
		// Check if we have txs from the parameters
		if len(txs) > 0 {
			data.Txs = make(types.Txs, len(txs))
			for i, tx := range txs {
				data.Txs[i] = types.Tx(tx)
			}
		} else {
			// Try to get from batch queue
			batchTxs, batchTime, err := m.getTxsFromBatch()
			if err != nil {
				return nil, nil, fmt.Errorf("failed to get transactions from batch: %w", err)
			}

			// If we got transactions from the batch, use them
			if len(batchTxs) > 0 {
				data.Txs = make(types.Txs, len(batchTxs))
				for i, tx := range batchTxs {
					data.Txs[i] = types.Tx(tx)
				}

				// Use the batch timestamp if available
				if batchTime != nil {
					header.Header.BaseHeader.Time = uint64(batchTime.UnixNano())
				}
			}
		}
	}

	// Ensure timestamp is set correctly
	if timestamp.UnixNano() > 0 {
		header.Header.BaseHeader.Time = uint64(timestamp.UnixNano())
	}

	// Generate a hash for the data
	dataHash := data.Hash()
	copy(header.Header.DataHash[:], dataHash)

	// Sign the header with the proposer's key
	signature, err := m.getSignature(header.Header)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to sign header: %w", err)
	}
	header.Signature = signature

	return header, data, nil
}

func (m *Manager) applyBlock(ctx context.Context, header *types.SignedHeader, data *types.Data) (types.State, *abci.ResponseFinalizeBlock, error) {
	m.state.stateMtx.RLock()
	defer m.state.stateMtx.RUnlock()
	return m.executor.execApplyBlock(ctx, m.state.currentState, header, data)
}

func (m *Manager) execCommit(ctx context.Context, newState types.State, h *types.SignedHeader, _ *types.Data, _ *abci.ResponseFinalizeBlock) ([]byte, error) {
	err := m.executor.execSetFinal(ctx, h.Height())
	return newState.AppHash, err
}

// getInitialState tries to load lastState from Store
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
		}
		return s, nil
	} else if err != nil {
		logger.Error("error while getting state", "error", err)
		return types.State{}, err
	}

	// Perform a sanity-check to stop the user from using a higher genesis than the last stored state.
	if uint64(genesis.InitialHeight) > s.LastBlockHeight {
		return types.State{}, fmt.Errorf("genesis.InitialHeight (%d) is greater than last stored state's LastBlockHeight (%d)", genesis.InitialHeight, s.LastBlockHeight)
	}

	return s, nil
}

// Direct batch-based sequencing mode implementation

// BatchSequencingLoop runs a loop that collects transaction batches and directly
// posts them to DA without waiting for block creation
func (m *Manager) BatchSequencingLoop(ctx context.Context) {
	if !m.isProposer {
		m.logger.Info("BatchSequencingLoop not starting - not a proposer")
		return
	}

	if m.sequencer == nil {
		m.logger.Error("BatchSequencingLoop not starting - sequencer not configured")
		return
	}

	// Use a shorter interval for batch processing than regular blocks
	batchTimer := time.NewTicker(m.conf.BlockTime / 2)
	defer batchTimer.Stop()

	m.logger.Info("Starting batch sequencing loop")

	for {
		select {
		case <-ctx.Done():
			m.logger.Info("Stopping batch sequencing loop")
			return
		case <-batchTimer.C:
			err := m.processBatchDirectToDA(ctx)
			if err != nil {
				m.logger.Error("Error in batch processing", "error", err)
			}
		}
	}
}

// processBatchDirectToDA gets the next batch from the queue,
// wraps it in a minimal header, and posts directly to DA
func (m *Manager) processBatchDirectToDA(ctx context.Context) error {
	// Check if DA client is initialized
	if !m.DALCInitialized() {
		return errors.New("DA client not initialized")
	}

	// Get next batch from the batch queue
	batch := m.batchQueue.Next()
	if batch == nil {
		// No batch available, nothing to do
		return nil
	}

	// Get current state for creating the header
	m.state.stateMtx.RLock()
	currentState := m.state.currentState
	m.state.stateMtx.RUnlock()

	// Convert batch transactions to types.Txs
	txs := make(types.Txs, len(batch.Transactions))
	for i, tx := range batch.Transactions {
		txs[i] = types.Tx(tx)
	}

	// Create a data structure
	data := &types.Data{
		Txs: txs,
	}

	// Generate data hash
	dataHash := data.Hash()

	// Create a minimal header with batch information
	height := currentState.LastBlockHeight + 1
	header := &types.SignedHeader{
		Header: types.Header{
			Version: types.Version{
				Block: currentState.Version.Consensus.Block,
				App:   currentState.Version.Consensus.App,
			},
			BaseHeader: types.BaseHeader{
				ChainID: currentState.ChainID,
				Height:  height,
				Time:    uint64(batch.Time.UnixNano()),
			},
			DataHash:        dataHash,
			ConsensusHash:   make(types.Hash, 32),
			AppHash:         currentState.AppHash,
			ProposerAddress: m.creator.proposerAddress,
		},
	}

	// Validate batch transactions
	if err := m.validateBatch(ctx, txs); err != nil {
		m.logger.Error("Batch validation failed", "error", err, "height", height)
		return fmt.Errorf("batch validation failed: %w", err)
	}

	// Optionally pre-execute the batch to ensure validity
	if m.conf.PreExecuteBatches {
		// Extract raw transaction bytes for execution
		rawTxs := make([][]byte, len(txs))
		for i, tx := range txs {
			rawTxs[i] = tx
		}

		timestamp := time.Unix(0, int64(header.Header.BaseHeader.Time))

		// Pre-execute the transactions
		_, err := m.executor.execExecuteTxs(ctx, rawTxs, height, timestamp, currentState.AppHash)
		if err != nil {
			m.logger.Error("Batch pre-execution failed", "error", err, "height", height)
			return fmt.Errorf("batch pre-execution failed: %w", err)
		}
	}

	// Sign the header
	signature, err := m.getSignature(header.Header)
	if err != nil {
		return fmt.Errorf("failed to sign header for batch: %w", err)
	}
	header.Signature = signature

	// Submit directly to DA
	maxBlobSize, err := m.da.client.DA.MaxBlobSize(ctx)
	if err != nil {
		return fmt.Errorf("failed to get max blob size: %w", err)
	}

	// Create a batch submission that includes both header and data
	// We'll need to serialize both and submit them together
	headerBlob, err := header.MarshalBinary()
	if err != nil {
		return fmt.Errorf("failed to serialize header: %w", err)
	}

	dataBlob, err := data.MarshalBinary()
	if err != nil {
		return fmt.Errorf("failed to serialize data: %w", err)
	}

	// Combine header and data with a simple separator
	// A more robust production implementation would use a proper serialization format
	batchSize := uint64(len(headerBlob) + len(dataBlob) + 8) // 8 bytes for size prefix
	if batchSize > maxBlobSize {
		return fmt.Errorf("batch size %d exceeds max blob size %d", batchSize, maxBlobSize)
	}

	// Create a combined blob with size prefixes
	combinedBlob := make([]byte, 4+len(headerBlob)+4+len(dataBlob))
	binary.BigEndian.PutUint32(combinedBlob[0:4], uint32(len(headerBlob)))
	copy(combinedBlob[4:4+len(headerBlob)], headerBlob)
	binary.BigEndian.PutUint32(combinedBlob[4+len(headerBlob):8+len(headerBlob)], uint32(len(dataBlob)))
	copy(combinedBlob[8+len(headerBlob):], dataBlob)

	// Submit the combined blob to DA
	blob := da.Blob{
		Data: combinedBlob,
	}

	// Submit to DA and handle the result
	result, err := m.da.client.SubmitBlob(ctx, blob)
	if err != nil {
		return fmt.Errorf("failed to submit batch to DA: %w", err)
	}

	if result.Code != da.StatusSuccess {
		return fmt.Errorf("batch submission failed: %s", result.Message)
	}

	// Log success
	m.logger.Info("Batch successfully submitted to DA",
		"height", height,
		"txs", len(txs),
		"da_height", result.DAHeight)

	return nil
}

// validateBatch performs basic validation on a batch of transactions
// This helps avoid posting invalid batches to the DA layer
func (m *Manager) validateBatch(ctx context.Context, txs types.Txs) error {
	if len(txs) == 0 {
		// Empty batch is technically valid, but we might want to skip it
		m.logger.Debug("Empty batch detected, proceeding anyway")
	}

	// Validate each transaction in the batch
	for i, tx := range txs {
		if len(tx) == 0 {
			return fmt.Errorf("transaction %d is empty", i)
		}

		// Add more transaction validation as needed
		// For example, size limits, format checks, etc.
	}

	return nil
}

// basedSequencingLoop implements a sequencing loop where batches are directly posted to DA
// This is the implementation of Based Sequencing as described in the design meeting notes
func (m *Manager) basedSequencingLoop(ctx context.Context) {
	if !m.isProposer {
		m.logger.Info("Based sequencing loop not starting - not a proposer")
		return
	}

	if m.sequencer == nil {
		m.logger.Error("Based sequencing loop not starting - sequencer not configured")
		return
	}

	// Use a shorter interval for batch processing than regular blocks
	batchTimer := time.NewTicker(m.conf.BlockTime / 2)
	defer batchTimer.Stop()

	m.logger.Info("Starting based sequencing loop")

	for {
		select {
		case <-ctx.Done():
			m.logger.Info("Stopping based sequencing loop")
			return
		case <-batchTimer.C:
			err := m.processBatchDirectToDA(ctx)
			if err != nil {
				m.logger.Error("Error in batch processing", "error", err)
			}
		}
	}
}
