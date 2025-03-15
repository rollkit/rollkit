package block

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	goheaderstore "github.com/celestiaorg/go-header/store"
	abci "github.com/cometbft/cometbft/abci/types"
	cmbytes "github.com/cometbft/cometbft/libs/bytes"
	cmstate "github.com/cometbft/cometbft/proto/tendermint/state"
	cmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	cmtypes "github.com/cometbft/cometbft/types"
	ds "github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p/core/crypto"

	"github.com/rollkit/rollkit/config"
	coreda "github.com/rollkit/rollkit/core/da"
	coreexecutor "github.com/rollkit/rollkit/core/execution"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
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

	// DAIncludedHeightKey is the key used for persisting the da included height in store.
	DAIncludedHeightKey = "d"

	// LastBatchHashKey is the key used for persisting the last batch hash in store.
	LastBatchHashKey = "l"
)

var (
	// dataHashForEmptyTxs to be used while only syncing headers from DA and no p2p to get the Data for no txs scenarios, the syncing can proceed without getting stuck forever.
	dataHashForEmptyTxs = []byte{110, 52, 11, 156, 255, 179, 122, 152, 156, 165, 68, 230, 187, 120, 10, 44, 120, 144, 29, 63, 179, 55, 56, 118, 133, 17, 163, 6, 23, 175, 160, 29}

	// ErrNoBatch indicate no batch is available for creating block
	ErrNoBatch = errors.New("no batch to process")

	// ErrHeightFromFutureStr is the error message for height from future returned by da
	ErrHeightFromFutureStr = "given height is from the future"
	// initialBackoff defines initial value for block submission backoff
	initialBackoff = 100 * time.Millisecond
)

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

// Manager is responsible for aggregating transactions into blocks.
type Manager struct {
	lastState types.State
	// lastStateMtx is used by lastState
	lastStateMtx *sync.RWMutex
	store        store.Store

	config  config.Config
	genesis *RollkitGenesis

	proposerKey crypto.PrivKey

	// daHeight is the height of the latest processed DA block
	daHeight uint64

	// Channels for receiving headers and data
	HeaderCh chan *types.SignedHeader
	DataCh   chan *types.Data

	// Channels for internal communication
	headerInCh    chan NewHeaderEvent
	dataInCh      chan NewDataEvent
	headerStoreCh chan struct{}
	dataStoreCh   chan struct{}
	retrieveCh    chan struct{}

	// Caches for headers and data
	headerCache *HeaderCache
	dataCache   *DataCache

	// Stores for headers and data
	headerStore *goheaderstore.Store[*types.SignedHeader]
	dataStore   *goheaderstore.Store[*types.Data]

	logger log.Logger

	// For usage by Lazy Aggregator mode
	buildingBlock bool

	pendingHeaders *PendingHeaders

	// for reporting metrics
	metrics *Metrics

	// true if the manager is a proposer
	isProposer bool

	exec coreexecutor.Executor

	// daIncludedHeight is rollup height at which all blocks have been included
	// in the DA
	daIncludedHeight atomic.Uint64
	dalc             coreda.Client
	gasPrice         float64
	gasMultiplier    float64

	sequencer     coresequencer.Sequencer
	lastBatchHash []byte
	bq            *BatchQueue
}

// RollkitGenesis is the genesis state of the rollup
type RollkitGenesis struct {
	GenesisTime     time.Time
	InitialHeight   uint64
	ChainID         string
	ProposerAddress []byte
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

// NewManager creates new block Manager.
func NewManager(
	ctx context.Context,
	proposerKey crypto.PrivKey,
	config config.Config,
	genesis *RollkitGenesis,
	store store.Store,
	exec coreexecutor.Executor,
	sequencer coresequencer.Sequencer,
	dalc coreda.Client,
	logger log.Logger,
	headerStore *goheaderstore.Store[*types.SignedHeader],
	dataStore *goheaderstore.Store[*types.Data],
	seqMetrics *Metrics,
	gasPrice float64,
	gasMultiplier float64,
) (*Manager, error) {
	s, err := getInitialState(ctx, genesis, store, exec, logger)
	if err != nil {
		logger.Error("error while getting initial state", "error", err)
		return nil, err
	}
	//set block height in store
	store.SetHeight(ctx, s.LastBlockHeight)

	if s.DAHeight < config.DA.StartHeight {
		s.DAHeight = config.DA.StartHeight
	}

	if config.DA.BlockTime == 0 {
		logger.Info("Using default DA block time", "DABlockTime", defaultDABlockTime)
		config.DA.BlockTime = defaultDABlockTime
	}

	if config.Node.BlockTime == 0 {
		logger.Info("Using default block time", "BlockTime", defaultBlockTime)
		config.Node.BlockTime = defaultBlockTime
	}

	if config.Node.LazyBlockTime == 0 {
		logger.Info("Using default lazy block time", "LazyBlockTime", defaultLazyBlockTime)
		config.Node.LazyBlockTime = defaultLazyBlockTime
	}

	if config.DA.MempoolTTL == 0 {
		logger.Info("Using default mempool ttl", "MempoolTTL", defaultMempoolTTL)
		config.DA.MempoolTTL = defaultMempoolTTL
	}

	//proposerAddress := s.Validators.Proposer.Address.Bytes()

	maxBlobSize, err := dalc.MaxBlobSize(ctx)
	if err != nil {
		return nil, err
	}
	// allow buffer for the block header and protocol encoding
	//nolint:ineffassign // This assignment is needed
	maxBlobSize -= blockProtocolOverhead

	isProposer, err := isProposer(proposerKey, s)
	if err != nil {
		logger.Error("error while checking if proposer", "error", err)
		return nil, err
	}

	pendingHeaders, err := NewPendingHeaders(store, logger)
	if err != nil {
		return nil, err
	}

	// If lastBatchHash is not set, retrieve the last batch hash from store
	lastBatchHash, err := store.GetMetadata(ctx, LastBatchHashKey)
	if err != nil {
		logger.Error("error while retrieving last batch hash", "error", err)
	}

	agg := &Manager{
		proposerKey: proposerKey,
		config:      config,
		genesis:     genesis,
		lastState:   s,
		store:       store,
		dalc:        dalc,
		daHeight:    s.DAHeight,
		// channels are buffered to avoid blocking on input/output operations, buffer sizes are arbitrary
		HeaderCh:       make(chan *types.SignedHeader, channelLength),
		DataCh:         make(chan *types.Data, channelLength),
		headerInCh:     make(chan NewHeaderEvent, headerInChLength),
		dataInCh:       make(chan NewDataEvent, headerInChLength),
		headerStoreCh:  make(chan struct{}, 1),
		dataStoreCh:    make(chan struct{}, 1),
		headerStore:    headerStore,
		dataStore:      dataStore,
		lastStateMtx:   new(sync.RWMutex),
		lastBatchHash:  lastBatchHash,
		headerCache:    NewHeaderCache(),
		dataCache:      NewDataCache(),
		retrieveCh:     make(chan struct{}, 1),
		logger:         logger,
		buildingBlock:  false,
		pendingHeaders: pendingHeaders,
		metrics:        seqMetrics,
		isProposer:     isProposer,
		sequencer:      sequencer,
		bq:             NewBatchQueue(),
		exec:           exec,
		gasPrice:       gasPrice,
		gasMultiplier:  gasMultiplier,
	}
	agg.init(ctx)
	return agg, nil
}

// DALCInitialized returns true if DALC is initialized.
func (m *Manager) DALCInitialized() bool {
	return m.dalc != nil
}

// PendingHeaders returns the pending headers.
func (m *Manager) PendingHeaders() *PendingHeaders {
	return m.pendingHeaders
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
	return m.lastState
}

// init initializes the Manager instance.
func (m *Manager) init(ctx context.Context) {
	go m.SyncLoop(ctx)
	go m.HeaderStoreRetrieveLoop(ctx)
	go m.DataStoreRetrieveLoop(ctx)

	// If dalc is initialized start the DA retrieve loop
	if m.dalc != nil {
		go m.RetrieveLoop(ctx)
		if m.isProposer {
			go m.HeaderSubmissionLoop(ctx)
		}
	}

	if m.isProposer {
		// Starting Aggregation Loop based on configuration
		blockTimer := time.NewTimer(0)
		defer blockTimer.Stop()

		// If we're using based sequencing, start the batch retrieve loop
		if m.sequencer != nil && m.IsBasedSequencing() {
			m.logger.Info("Starting batch retrieve loop for based sequencing")
			go m.BatchRetrieveLoop(ctx)
		}

		if m.config.Node.LazyAggregator {
			go m.lazyAggregationLoop(ctx, blockTimer)
		} else {
			go m.normalAggregationLoop(ctx, blockTimer)
		}
	}
}

func (m *Manager) setDAIncludedHeight(ctx context.Context, newHeight uint64) error {
	for {
		currentHeight := m.daIncludedHeight.Load()
		if newHeight <= currentHeight {
			break
		}
		if m.daIncludedHeight.CompareAndSwap(currentHeight, newHeight) {
			heightBytes := make([]byte, 8)
			binary.BigEndian.PutUint64(heightBytes, newHeight)
			return m.store.SetMetadata(ctx, DAIncludedHeightKey, heightBytes)
		}
	}
	return nil
}

// GetDAIncludedHeight returns the rollup height at which all blocks have been
// included in the DA
func (m *Manager) GetDAIncludedHeight() uint64 {
	return m.daIncludedHeight.Load()
}

// SetDALC is used to set DataAvailabilityLayerClient used by Manager.
func (m *Manager) SetDALC(dalc coreda.Client) {
	m.dalc = dalc
}

// isProposer returns whether or not the manager is a proposer
func isProposer(_ crypto.PrivKey, _ types.State) (bool, error) {
	return true, nil
}

// SetLastState is used to set lastState used by Manager.
func (m *Manager) SetLastState(state types.State) {
	m.lastStateMtx.Lock()
	defer m.lastStateMtx.Unlock()
	m.lastState = state
}

// GetStoreHeight returns the manager's store height
func (m *Manager) GetStoreHeight() uint64 {
	return m.store.Height()
}

// GetHeaderInCh returns the manager's blockInCh
func (m *Manager) GetHeaderInCh() chan NewHeaderEvent {
	return m.headerInCh
}

// GetDataInCh returns the manager's dataInCh
func (m *Manager) GetDataInCh() chan NewDataEvent {
	return m.dataInCh
}

// IsBlockHashSeen returns true if the block with the given hash has been seen.
func (m *Manager) IsBlockHashSeen(blockHash string) bool {
	return m.headerCache.isSeen(blockHash)
}

// IsDAIncluded returns true if the block with the given hash has been seen on DA.
func (m *Manager) IsDAIncluded(hash types.Hash) bool {
	return m.headerCache.isDAIncluded(hash.String())
}

// getRemainingSleep calculates the remaining sleep time based on config and a start time.
func (m *Manager) getRemainingSleep(start time.Time) time.Duration {
	elapsed := time.Since(start)
	interval := m.config.Node.BlockTime

	if m.config.Node.LazyAggregator {
		if m.buildingBlock && elapsed >= interval {
			// Special case to give time for transactions to accumulate if we
			// are coming out of a period of inactivity.
			return (interval * time.Duration(defaultLazySleepPercent) / 100)
		} else if !m.buildingBlock {
			interval = m.config.Node.LazyBlockTime
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
	return m.exec
}

func (m *Manager) handleEmptyDataHash(ctx context.Context, header *types.Header) {
	headerHeight := header.Height()
	if bytes.Equal(header.DataHash, dataHashForEmptyTxs) {
		var lastDataHash types.Hash
		var err error
		var lastData *types.Data
		if headerHeight > 1 {
			_, lastData, err = m.store.GetBlockData(ctx, headerHeight-1)
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
			m.dataCache.setData(headerHeight, d)
		}
	}
}

func (m *Manager) getSignature(header types.Header) (types.Signature, error) {
	b, err := header.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return m.proposerKey.Sign(b)
}

func (m *Manager) sign(payload []byte) ([]byte, error) {
	return m.proposerKey.Sign(payload)
}

func (m *Manager) recordMetrics(height uint64) {
	if m.metrics == nil {
		return
	}

	// If height is provided directly, use it
	m.metrics.CommittedHeight.Set(float64(height))
}

func (m *Manager) exponentialBackoff(backoff time.Duration) time.Duration {
	backoff *= 2
	if backoff == 0 {
		backoff = initialBackoff
	}
	if backoff > m.config.DA.BlockTime {
		backoff = m.config.DA.BlockTime
	}
	return backoff
}

// Updates the state stored in manager's store along the manager's lastState
func (m *Manager) updateState(ctx context.Context, s types.State) error {
	m.logger.Debug("updating state", "newState", s)
	m.lastStateMtx.Lock()
	defer m.lastStateMtx.Unlock()
	err := m.store.UpdateState(ctx, s)
	if err != nil {
		return err
	}
	m.lastState = s
	m.metrics.Height.Set(float64(s.LastBlockHeight))
	return nil
}

func (m *Manager) getLastBlockTime() time.Time {
	m.lastStateMtx.RLock()
	defer m.lastStateMtx.RUnlock()
	return m.lastState.LastBlockTime
}

func (m *Manager) createBlock(ctx context.Context, height uint64, lastSignature *types.Signature, lastHeaderHash types.Hash, txs [][]byte, timestamp time.Time) (*types.SignedHeader, *types.Data, error) {
	m.lastStateMtx.RLock()
	defer m.lastStateMtx.RUnlock()
	return m.execCreateBlock(ctx, height, lastSignature, lastHeaderHash, m.lastState, txs, timestamp)
}

func (m *Manager) applyBlock(ctx context.Context, header *types.SignedHeader, data *types.Data) (types.State, *abci.ResponseFinalizeBlock, error) {
	m.lastStateMtx.RLock()
	defer m.lastStateMtx.RUnlock()
	return m.execApplyBlock(ctx, m.lastState, header, data)
}

func (m *Manager) execValidate(_ types.State, _ *types.SignedHeader, _ *types.Data) error {
	// TODO(tzdybal): implement
	return nil
}

func (m *Manager) execCommit(ctx context.Context, newState types.State, h *types.SignedHeader, _ *types.Data, _ *abci.ResponseFinalizeBlock) ([]byte, error) {
	err := m.exec.SetFinal(ctx, h.Height())
	return newState.AppHash, err
}

func (m *Manager) execCreateBlock(_ context.Context, height uint64, lastSignature *types.Signature, _ types.Hash, lastState types.State, txs [][]byte, timestamp time.Time) (*types.SignedHeader, *types.Data, error) {
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
			DataHash:        make(types.Hash, 32),
			ConsensusHash:   make(types.Hash, 32),
			AppHash:         lastState.AppHash,
			ProposerAddress: m.genesis.ProposerAddress,
		},
		Signature: *lastSignature,
	}

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

func (m *Manager) execApplyBlock(ctx context.Context, lastState types.State, header *types.SignedHeader, data *types.Data) (types.State, *abci.ResponseFinalizeBlock, error) {
	rawTxs := make([][]byte, len(data.Txs))
	for i := range data.Txs {
		rawTxs[i] = data.Txs[i]
	}
	newStateRoot, _, err := m.exec.ExecuteTxs(ctx, rawTxs, header.Height(), header.Time(), lastState.AppHash)
	if err != nil {
		return types.State{}, nil, err
	}

	s, err := m.nextState(lastState, header, newStateRoot)
	if err != nil {
		return types.State{}, nil, err
	}

	return s, nil, nil
}

func (m *Manager) nextState(state types.State, header *types.SignedHeader, stateRoot []byte) (types.State, error) {
	height := header.Height()

	s := types.State{
		Version:         state.Version,
		ChainID:         state.ChainID,
		InitialHeight:   state.InitialHeight,
		LastBlockHeight: height,
		LastBlockTime:   header.Time(),
		LastBlockID: cmtypes.BlockID{
			Hash: cmbytes.HexBytes(header.Hash()),
			// for now, we don't care about part set headers
		},
		//ConsensusParams:                  state.ConsensusParams,
		//LastHeightConsensusParamsChanged: state.LastHeightConsensusParamsChanged,
		AppHash: stateRoot,
		//Validators:                       state.NextValidators.Copy(),
		//LastValidators:                   state.Validators.Copy(),
	}
	return s, nil
}

// handleHeaderGossip determines how to handle header gossip based on sequencer mode
func (m *Manager) handleHeaderGossip(ctx context.Context, header *types.SignedHeader) error {
	// In based sequencing mode, headers are expected to be finalized immediately
	// after verification with DA layer, so we mark them as DA included right away
	if m.IsBasedSequencing() {
		headerHash := header.Hash().String()
		m.headerCache.setDAIncluded(headerHash)
		err := m.setDAIncludedHeight(ctx, header.Height())
		if err != nil {
			return err
		}
		m.logger.Info("based mode: header marked as DA included immediately",
			"headerHeight", header.Height(),
			"headerHash", headerHash)
	}

	// Continue with normal header processing
	return nil
}
