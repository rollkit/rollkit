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

	secp256k1 "github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	abci "github.com/cometbft/cometbft/abci/types"
	cmcrypto "github.com/cometbft/cometbft/crypto"
	"github.com/cometbft/cometbft/crypto/merkle"
	cmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	"github.com/cometbft/cometbft/proxy"
	cmtypes "github.com/cometbft/cometbft/types"
	ds "github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/crypto/pb"

	goheaderstore "github.com/celestiaorg/go-header/store"

	"github.com/rollkit/go-sequencing"
	"github.com/rollkit/go-sequencing/proxy/grpc"
	"github.com/rollkit/rollkit/config"
	"github.com/rollkit/rollkit/da"
	"github.com/rollkit/rollkit/mempool"
	"github.com/rollkit/rollkit/state"
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
	*sequencing.Batch
	time.Time
}

// Manager is responsible for aggregating transactions into blocks.
type Manager struct {
	lastState types.State
	// lastStateMtx is used by lastState
	lastStateMtx *sync.RWMutex
	store        store.Store

	conf    config.BlockManagerConfig
	genesis *cmtypes.GenesisDoc

	proposerKey crypto.PrivKey

	executor *state.BlockExecutor

	dalc *da.DAClient
	// daHeight is the height of the latest processed DA block
	daHeight uint64

	HeaderCh chan *types.SignedHeader
	DataCh   chan *types.Data

	headerInCh  chan NewHeaderEvent
	headerStore *goheaderstore.Store[*types.SignedHeader]

	dataInCh  chan NewDataEvent
	dataStore *goheaderstore.Store[*types.Data]

	headerCache *HeaderCache
	dataCache   *DataCache

	// headerStoreCh is used to notify sync goroutine (SyncLoop) that it needs to retrieve headers from headerStore
	headerStoreCh chan struct{}

	// dataStoreCh is used to notify sync goroutine (SyncLoop) that it needs to retrieve data from dataStore
	dataStoreCh chan struct{}

	// retrieveCh is used to notify sync goroutine (SyncLoop) that it needs to retrieve data
	retrieveCh chan struct{}

	logger log.Logger

	// For usage by Lazy Aggregator mode
	buildingBlock bool

	pendingHeaders *PendingHeaders

	// for reporting metrics
	metrics *Metrics

	// true if the manager is a proposer
	isProposer bool

	// daIncludedHeight is rollup height at which all blocks have been included
	// in the DA
	daIncludedHeight atomic.Uint64
	// grpc client for sequencing middleware
	seqClient     *grpc.Client
	lastBatchHash []byte
	bq            *BatchQueue
}

// getInitialState tries to load lastState from Store, and if it's not available it reads GenesisDoc.
func getInitialState(store store.Store, genesis *cmtypes.GenesisDoc) (types.State, error) {
	// Load the state from store.
	s, err := store.GetState(context.Background())

	if errors.Is(err, ds.ErrNotFound) {
		// If the user is starting a fresh chain (or hard-forking), we assume the stored state is empty.
		s, err = types.NewFromGenesisDoc(genesis)
		if err != nil {
			return types.State{}, err
		}
	} else if err != nil {
		return types.State{}, err
	} else {
		// Perform a sanity-check to stop the user from
		// using a higher genesis than the last stored state.
		// if they meant to hard-fork, they should have cleared the stored State
		if uint64(genesis.InitialHeight) > s.LastBlockHeight { //nolint:gosec
			return types.State{}, fmt.Errorf("genesis.InitialHeight (%d) is greater than last stored state's LastBlockHeight (%d)", genesis.InitialHeight, s.LastBlockHeight)
		}
	}

	return s, nil
}

// NewManager creates new block Manager.
func NewManager(
	proposerKey crypto.PrivKey,
	conf config.BlockManagerConfig,
	genesis *cmtypes.GenesisDoc,
	store store.Store,
	mempool mempool.Mempool,
	mempoolReaper *mempool.CListMempoolReaper,
	seqClient *grpc.Client,
	proxyApp proxy.AppConnConsensus,
	dalc *da.DAClient,
	eventBus *cmtypes.EventBus,
	logger log.Logger,
	headerStore *goheaderstore.Store[*types.SignedHeader],
	dataStore *goheaderstore.Store[*types.Data],
	seqMetrics *Metrics,
	execMetrics *state.Metrics,
) (*Manager, error) {
	s, err := getInitialState(store, genesis)
	if err != nil {
		return nil, err
	}
	//set block height in store
	store.SetHeight(context.Background(), s.LastBlockHeight)

	if s.DAHeight < conf.DAStartHeight {
		s.DAHeight = conf.DAStartHeight
	}

	if conf.DABlockTime == 0 {
		logger.Info("Using default DA block time", "DABlockTime", defaultDABlockTime)
		conf.DABlockTime = defaultDABlockTime
	}

	if conf.BlockTime == 0 {
		logger.Info("Using default block time", "BlockTime", defaultBlockTime)
		conf.BlockTime = defaultBlockTime
	}

	if conf.LazyBlockTime == 0 {
		logger.Info("Using default lazy block time", "LazyBlockTime", defaultLazyBlockTime)
		conf.LazyBlockTime = defaultLazyBlockTime
	}

	if conf.DAMempoolTTL == 0 {
		logger.Info("Using default mempool ttl", "MempoolTTL", defaultMempoolTTL)
		conf.DAMempoolTTL = defaultMempoolTTL
	}

	proposerAddress := s.Validators.Proposer.Address.Bytes()

	maxBlobSize, err := dalc.DA.MaxBlobSize(context.Background())
	if err != nil {
		return nil, err
	}
	// allow buffer for the block header and protocol encoding
	maxBlobSize -= blockProtocolOverhead

	exec := state.NewBlockExecutor(proposerAddress, genesis.ChainID, mempool, mempoolReaper, proxyApp, eventBus, maxBlobSize, logger, execMetrics)
	if s.LastBlockHeight+1 == uint64(genesis.InitialHeight) { //nolint:gosec
		res, err := exec.InitChain(genesis)
		if err != nil {
			return nil, err
		}
		if err := updateState(&s, res); err != nil {
			return nil, err
		}

		if err := store.UpdateState(context.Background(), s); err != nil {
			return nil, err
		}
	}

	isProposer, err := isProposer(proposerKey, s)
	if err != nil {
		return nil, err
	}

	pendingHeaders, err := NewPendingHeaders(store, logger)
	if err != nil {
		return nil, err
	}

	// If lastBatchHash is not set, retrieve the last batch hash from store
	lastBatchHash, err := store.GetMetadata(context.Background(), LastBatchHashKey)
	if err != nil {
		logger.Error("error while retrieving last batch hash", "error", err)
	}

	agg := &Manager{
		proposerKey: proposerKey,
		conf:        conf,
		genesis:     genesis,
		lastState:   s,
		store:       store,
		executor:    exec,
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
		seqClient:      seqClient,
		bq:             NewBatchQueue(),
	}
	agg.init(context.Background())
	return agg, nil
}

func (m *Manager) init(ctx context.Context) {
	// initialize da included height
	if height, err := m.store.GetMetadata(ctx, DAIncludedHeightKey); err == nil && len(height) == 8 {
		m.daIncludedHeight.Store(binary.BigEndian.Uint64(height))
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
func (m *Manager) SetDALC(dalc *da.DAClient) {
	m.dalc = dalc
}

// isProposer returns whether or not the manager is a proposer
func isProposer(signerPrivKey crypto.PrivKey, s types.State) (bool, error) {
	if len(s.Validators.Validators) == 0 {
		return false, ErrNoValidatorsInState
	}

	signerPubBytes, err := signerPrivKey.GetPublic().Raw()
	if err != nil {
		return false, err
	}

	return bytes.Equal(s.Validators.Validators[0].PubKey.Bytes(), signerPubBytes), nil
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

// BatchRetrieveLoop is responsible for retrieving batches from the sequencer.
func (m *Manager) BatchRetrieveLoop(ctx context.Context) {
	// Initialize batchTimer to fire immediately on start
	batchTimer := time.NewTimer(0)
	defer batchTimer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-batchTimer.C:
			start := time.Now()

			// Skip batch retrieval if context is already done
			if ctx.Err() != nil {
				return
			}

			res, err := m.seqClient.GetNextBatch(ctx, sequencing.GetNextBatchRequest{
				RollupId:      []byte(m.genesis.ChainID),
				LastBatchHash: m.lastBatchHash,
			})

			if err != nil {
				m.logger.Error("error while retrieving batch", "error", err)
			}

			if res != nil && res.Batch != nil {
				batch := res.Batch
				batchTime := res.Timestamp

				// Calculate and store batch hash only if hashing succeeds
				if h, err := batch.Hash(); err == nil {
					m.bq.AddBatch(BatchWithTime{Batch: batch, Time: batchTime})

					// Update lastBatchHash only if the batch contains transactions
					if batch.Transactions != nil {
						err := m.store.SetMetadata(ctx, LastBatchHashKey, h)
						if err != nil {
							m.logger.Error("error while setting last batch hash", "error", err)
						}

						m.lastBatchHash = h
					}
				} else {
					m.logger.Error("error while hashing batch", "error", err)
				}
			}

			// Determine remaining time for the next batch and reset timer
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
	initialHeight := uint64(m.genesis.InitialHeight) //nolint:gosec
	height := m.store.Height()
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

	// Lazy Aggregator mode.
	// In Lazy Aggregator mode, blocks are built only when there are
	// transactions or every LazyBlockTime.
	if m.conf.LazyAggregator {
		m.lazyAggregationLoop(ctx, blockTimer)
		return
	}

	m.normalAggregationLoop(ctx, blockTimer)
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
		// the m.bq.notifyCh channel is signalled when batch becomes available in the batch queue
		case _, ok := <-m.bq.notifyCh:
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
		if m.pendingHeaders.isEmpty() {
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

// SyncLoop is responsible for syncing blocks.
//
// SyncLoop processes headers gossiped in P2P network to know what's the latest block height,
// block data is retrieved from DA layer.
func (m *Manager) SyncLoop(ctx context.Context, cancel context.CancelFunc) {
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
		case headerEvent := <-m.headerInCh:
			// Only validated headers are sent to headerInCh, so we can safely assume that headerEvent.header is valid
			header := headerEvent.Header
			daHeight := headerEvent.DAHeight
			headerHash := header.Hash().String()
			headerHeight := header.Height()
			m.logger.Debug("header retrieved",
				"height", headerHash,
				"daHeight", daHeight,
				"hash", headerHash,
			)
			if headerHeight <= m.store.Height() || m.headerCache.isSeen(headerHash) {
				m.logger.Debug("header already seen", "height", headerHeight, "block hash", headerHash)
				continue
			}
			m.headerCache.setHeader(headerHeight, header)

			m.sendNonBlockingSignalToHeaderStoreCh()
			m.sendNonBlockingSignalToRetrieveCh()

			// check if the dataHash is dataHashForEmptyTxs
			// no need to wait for syncing Data, instead prepare now and set
			// so that trySyncNextBlock can progress
			m.handleEmptyDataHash(ctx, &header.Header)

			err := m.trySyncNextBlock(ctx, daHeight)
			if err != nil {
				m.logger.Info("failed to sync next block", "error", err)
				continue
			}
			m.headerCache.setSeen(headerHash)
		case dataEvent := <-m.dataInCh:
			data := dataEvent.Data
			daHeight := dataEvent.DAHeight
			dataHash := data.Hash().String()
			dataHeight := data.Metadata.Height
			m.logger.Debug("data retrieved",
				"height", dataHash,
				"daHeight", daHeight,
				"hash", dataHash,
			)
			if dataHeight <= m.store.Height() || m.dataCache.isSeen(dataHash) {
				m.logger.Debug("data already seen", "height", dataHeight, "data hash", dataHash)
				continue
			}
			m.dataCache.setData(dataHeight, data)

			m.sendNonBlockingSignalToDataStoreCh()

			err := m.trySyncNextBlock(ctx, daHeight)
			if err != nil {
				m.logger.Info("failed to sync next block", "error", err)
				continue
			}
			m.dataCache.setSeen(dataHash)
		case <-ctx.Done():
			return
		}
	}
}

func (m *Manager) sendNonBlockingSignalToHeaderStoreCh() {
	select {
	case m.headerStoreCh <- struct{}{}:
	default:
	}
}

func (m *Manager) sendNonBlockingSignalToDataStoreCh() {
	select {
	case m.dataStoreCh <- struct{}{}:
	default:
	}
}

func (m *Manager) sendNonBlockingSignalToRetrieveCh() {
	select {
	case m.retrieveCh <- struct{}{}:
	default:
	}
}

// trySyncNextBlock tries to execute as many blocks as possible from the blockCache.
//
//	Note: the blockCache contains only valid blocks that are not yet synced
//
// For every block, to be able to apply block at height h, we need to have its Commit. It is contained in block at height h+1.
// If commit for block h+1 is available, we proceed with sync process, and remove synced block from sync cache.
func (m *Manager) trySyncNextBlock(ctx context.Context, daHeight uint64) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		currentHeight := m.store.Height()
		h := m.headerCache.getHeader(currentHeight + 1)
		if h == nil {
			m.logger.Debug("header not found in cache", "height", currentHeight+1)
			return nil
		}
		d := m.dataCache.getData(currentHeight + 1)
		if d == nil {
			m.logger.Debug("data not found in cache", "height", currentHeight+1)
			return nil
		}

		hHeight := h.Height()
		m.logger.Info("Syncing header and data", "height", hHeight)
		// Validate the received block before applying
		if err := m.executor.Validate(m.lastState, h, d); err != nil {
			return fmt.Errorf("failed to validate block: %w", err)
		}
		newState, responses, err := m.applyBlock(ctx, h, d)
		if err != nil {
			if ctx.Err() != nil {
				return err
			}
			// if call to applyBlock fails, we halt the node, see https://github.com/cometbft/cometbft/pull/496
			panic(fmt.Errorf("failed to ApplyBlock: %w", err))
		}
		err = m.store.SaveBlockData(ctx, h, d, &h.Signature)
		if err != nil {
			return SaveBlockError{err}
		}
		_, _, err = m.executor.Commit(ctx, newState, h, d, responses)
		if err != nil {
			return fmt.Errorf("failed to Commit: %w", err)
		}

		err = m.store.SaveBlockResponses(ctx, hHeight, responses)
		if err != nil {
			return SaveBlockResponsesError{err}
		}

		// Height gets updated
		m.store.SetHeight(ctx, hHeight)

		if daHeight > newState.DAHeight {
			newState.DAHeight = daHeight
		}
		err = m.updateState(ctx, newState)
		if err != nil {
			m.logger.Error("failed to save updated state", "error", err)
		}
		m.headerCache.deleteHeader(currentHeight + 1)
		m.dataCache.deleteData(currentHeight + 1)
	}
}

// HeaderStoreRetrieveLoop is responsible for retrieving headers from the Header Store.
func (m *Manager) HeaderStoreRetrieveLoop(ctx context.Context) {
	lastHeaderStoreHeight := uint64(0)
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.headerStoreCh:
		}
		headerStoreHeight := m.headerStore.Height()
		if headerStoreHeight > lastHeaderStoreHeight {
			headers, err := m.getHeadersFromHeaderStore(ctx, lastHeaderStoreHeight+1, headerStoreHeight)
			if err != nil {
				m.logger.Error("failed to get headers from Header Store", "lastHeaderHeight", lastHeaderStoreHeight, "headerStoreHeight", headerStoreHeight, "errors", err.Error())
				continue
			}
			daHeight := atomic.LoadUint64(&m.daHeight)
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
				m.headerInCh <- NewHeaderEvent{header, daHeight}
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
		case <-m.dataStoreCh:
		}
		dataStoreHeight := m.dataStore.Height()
		if dataStoreHeight > lastDataStoreHeight {
			data, err := m.getDataFromDataStore(ctx, lastDataStoreHeight+1, dataStoreHeight)
			if err != nil {
				m.logger.Error("failed to get data from Data Store", "lastDataStoreHeight", lastDataStoreHeight, "dataStoreHeight", dataStoreHeight, "errors", err.Error())
				continue
			}
			daHeight := atomic.LoadUint64(&m.daHeight)
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
				m.dataInCh <- NewDataEvent{d, daHeight}
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
		header, err := m.headerStore.GetByHeight(ctx, i)
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
		d, err := m.dataStore.GetByHeight(ctx, i)
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
		daHeight := atomic.LoadUint64(&m.daHeight)
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
		atomic.AddUint64(&m.daHeight, 1)
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
	daHeight := atomic.LoadUint64(&m.daHeight)

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
				m.headerCache.setDAIncluded(blockHash)
				err = m.setDAIncludedHeight(ctx, header.Height())
				if err != nil {
					return err
				}
				m.logger.Info("block marked as DA included", "blockHeight", header.Height(), "blockHash", blockHash)
				if !m.headerCache.isSeen(blockHash) {
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
					m.headerInCh <- NewHeaderEvent{header, daHeight}
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
	return bytes.Equal(header.ProposerAddress, m.genesis.Validators[0].Address.Bytes()) && header.ValidateBasic() == nil
}

func (m *Manager) fetchHeaders(ctx context.Context, daHeight uint64) (da.ResultRetrieveHeaders, error) {
	var err error
	headerRes := m.dalc.RetrieveHeaders(ctx, daHeight)
	if headerRes.Code == da.StatusError {
		err = fmt.Errorf("failed to retrieve block: %s", headerRes.Message)
	}
	return headerRes, err
}

func (m *Manager) getSignature(header types.Header) (*types.Signature, error) {
	// note: for compatibility with tendermint light client
	consensusVote := header.MakeCometBFTVote()
	sign, err := m.sign(consensusVote)
	if err != nil {
		return nil, err
	}
	signature := types.Signature(sign)
	return &signature, nil
}

func (m *Manager) getTxsFromBatch() (cmtypes.Txs, *time.Time, error) {
	batch := m.bq.Next()
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

func (m *Manager) publishBlock(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if !m.isProposer {
		return ErrNotProposer
	}

	if m.conf.MaxPendingBlocks != 0 && m.pendingHeaders.numPendingHeaders() >= m.conf.MaxPendingBlocks {
		return fmt.Errorf("refusing to create block: pending blocks [%d] reached limit [%d]",
			m.pendingHeaders.numPendingHeaders(), m.conf.MaxPendingBlocks)
	}

	var (
		lastSignature  *types.Signature
		lastHeaderHash types.Hash
		lastDataHash   types.Hash
		lastHeaderTime time.Time
		err            error
	)
	height := m.store.Height()
	newHeight := height + 1
	// this is a special case, when first block is produced - there is no previous commit
	if newHeight == uint64(m.genesis.InitialHeight) { //nolint:gosec
		lastSignature = &types.Signature{}
	} else {
		lastSignature, err = m.store.GetSignature(ctx, height)
		if err != nil {
			return fmt.Errorf("error while loading last commit: %w", err)
		}
		lastHeader, lastData, err := m.store.GetBlockData(ctx, height)
		if err != nil {
			return fmt.Errorf("error while loading last block: %w", err)
		}
		lastHeaderHash = lastHeader.Hash()
		lastDataHash = lastData.Hash()
		lastHeaderTime = lastHeader.Time()
	}

	var (
		header    *types.SignedHeader
		data      *types.Data
		signature *types.Signature
	)

	// Check if there's an already stored block at a newer height
	// If there is use that instead of creating a new block
	pendingHeader, pendingData, err := m.store.GetBlockData(ctx, newHeight)
	if err == nil {
		m.logger.Info("Using pending block", "height", newHeight)
		header = pendingHeader
		data = pendingData
	} else {
		extendedCommit, err := m.getExtendedCommit(ctx, height)
		if err != nil {
			return fmt.Errorf("failed to load extended commit for height %d: %w", height, err)
		}

		txs, timestamp, err := m.getTxsFromBatch()
		if errors.Is(err, ErrNoBatch) {
			m.logger.Info(err.Error())
			return nil
		}
		if err != nil {
			return fmt.Errorf("failed to get transactions from batch: %w", err)
		}
		// sanity check timestamp for monotonically increasing
		if timestamp.Before(lastHeaderTime) {
			return fmt.Errorf("timestamp is not monotonically increasing: %s < %s", timestamp, m.getLastBlockTime())
		}
		m.logger.Info("Creating and publishing block", "height", newHeight)
		header, data, err = m.createBlock(newHeight, lastSignature, lastHeaderHash, extendedCommit, txs, *timestamp)
		if err != nil {
			return err
		}
		m.logger.Debug("block info", "num_tx", len(data.Txs))

		/*
		   here we set the SignedHeader.DataHash, and SignedHeader.Signature as a hack
		   to make the block pass ValidateBasic() when it gets called by applyBlock on line 681
		   these values get overridden on lines 687-698 after we obtain the IntermediateStateRoots.
		*/
		header.DataHash = data.Hash()
		header.Validators = m.getLastStateValidators()
		header.ValidatorHash = header.Validators.Hash()

		signature, err = m.getSignature(header.Header)
		if err != nil {
			return err
		}

		// set the signature to current block's signed header
		header.Signature = *signature
		err = m.store.SaveBlockData(ctx, header, data, signature)
		if err != nil {
			return SaveBlockError{err}
		}
	}

	newState, responses, err := m.applyBlock(ctx, header, data)
	if err != nil {
		if ctx.Err() != nil {
			return err
		}
		// if call to applyBlock fails, we halt the node, see https://github.com/cometbft/cometbft/pull/496
		panic(err)
	}
	// Before taking the hash, we need updated ISRs, hence after ApplyBlock
	header.Header.DataHash = data.Hash()

	signature, err = m.getSignature(header.Header)
	if err != nil {
		return err
	}

	if err := m.processVoteExtension(ctx, header, data, newHeight); err != nil {
		return err
	}

	// set the signature to current block's signed header
	header.Signature = *signature

	// append metadata to Data before validating and saving
	data.Metadata = &types.Metadata{
		ChainID:      header.ChainID(),
		Height:       header.Height(),
		Time:         header.BaseHeader.Time,
		LastDataHash: lastDataHash,
	}
	// Validate the created block before storing
	if err := m.executor.Validate(m.lastState, header, data); err != nil {
		return fmt.Errorf("failed to validate block: %w", err)
	}

	headerHeight := header.Height()

	headerHash := header.Hash().String()
	m.headerCache.setSeen(headerHash)

	// SaveBlock commits the DB tx
	err = m.store.SaveBlockData(ctx, header, data, signature)
	if err != nil {
		return SaveBlockError{err}
	}

	// Commit the new state and block which writes to disk on the proxy app
	appHash, _, err := m.executor.Commit(ctx, newState, header, data, responses)
	if err != nil {
		return err
	}
	// Update app hash in state
	newState.AppHash = appHash

	// SaveBlockResponses commits the DB tx
	err = m.store.SaveBlockResponses(ctx, headerHeight, responses)
	if err != nil {
		return SaveBlockResponsesError{err}
	}

	// Update the store height before submitting to the DA layer but after committing to the DB
	m.store.SetHeight(ctx, headerHeight)

	newState.DAHeight = atomic.LoadUint64(&m.daHeight)
	// After this call m.lastState is the NEW state returned from ApplyBlock
	// updateState also commits the DB tx
	err = m.updateState(ctx, newState)
	if err != nil {
		return err
	}
	m.recordMetrics(data)
	// Check for shut down event prior to sending the header and block to
	// their respective channels. The reason for checking for the shutdown
	// event separately is due to the inconsistent nature of the select
	// statement when multiple cases are satisfied.
	select {
	case <-ctx.Done():
		return fmt.Errorf("unable to send header and block, context done: %w", ctx.Err())
	default:
	}

	// Publish header to channel so that header exchange service can broadcast
	m.HeaderCh <- header

	// Publish block to channel so that block exchange service can broadcast
	m.DataCh <- data

	m.logger.Debug("successfully proposed header", "proposer", hex.EncodeToString(header.ProposerAddress), "height", headerHeight)

	return nil
}

func (m *Manager) sign(payload []byte) ([]byte, error) {
	var sig []byte
	switch m.proposerKey.Type() {
	case pb.KeyType_Ed25519:
		return m.proposerKey.Sign(payload)
	case pb.KeyType_Secp256k1:
		k := m.proposerKey.(*crypto.Secp256k1PrivateKey)
		rawBytes, err := k.Raw()
		if err != nil {
			return nil, err
		}
		priv, _ := secp256k1.PrivKeyFromBytes(rawBytes)
		sig = ecdsa.SignCompact(priv, cmcrypto.Sha256(payload), false)
		return sig[1:], nil
	default:
		return nil, fmt.Errorf("unsupported private key type: %T", m.proposerKey)
	}
}

func (m *Manager) processVoteExtension(ctx context.Context, header *types.SignedHeader, data *types.Data, newHeight uint64) error {
	if !m.voteExtensionEnabled(newHeight) {
		return nil
	}

	extension, err := m.executor.ExtendVote(ctx, header, data)
	if err != nil {
		return fmt.Errorf("error returned by ExtendVote: %w", err)
	}

	vote := &cmproto.Vote{
		Height:    int64(newHeight), //nolint:gosec
		Round:     0,
		Extension: extension,
	}
	extSignBytes := cmtypes.VoteExtensionSignBytes(m.genesis.ChainID, vote)

	sign, err := m.sign(extSignBytes)
	if err != nil {
		return fmt.Errorf("failed to sign vote extension: %w", err)
	}
	extendedCommit := buildExtendedCommit(header, extension, sign)
	err = m.store.SaveExtendedCommit(ctx, newHeight, extendedCommit)
	if err != nil {
		return fmt.Errorf("failed to save extended commit: %w", err)
	}
	return nil
}

func (m *Manager) voteExtensionEnabled(newHeight uint64) bool {
	enableHeight := m.lastState.ConsensusParams.Abci.VoteExtensionsEnableHeight
	return m.lastState.ConsensusParams.Abci != nil && enableHeight != 0 && uint64(enableHeight) <= newHeight //nolint:gosec
}

func (m *Manager) getExtendedCommit(ctx context.Context, height uint64) (abci.ExtendedCommitInfo, error) {
	emptyExtendedCommit := abci.ExtendedCommitInfo{}
	if !m.voteExtensionEnabled(height) || height <= uint64(m.genesis.InitialHeight) { //nolint:gosec
		return emptyExtendedCommit, nil
	}
	extendedCommit, err := m.store.GetExtendedCommit(ctx, height)
	if err != nil {
		return emptyExtendedCommit, err
	}
	return *extendedCommit, nil
}

func buildExtendedCommit(header *types.SignedHeader, extension []byte, sign []byte) *abci.ExtendedCommitInfo {
	extendedCommit := &abci.ExtendedCommitInfo{
		Round: 0,
		Votes: []abci.ExtendedVoteInfo{{
			Validator: abci.Validator{
				Address: header.Validators.GetProposer().Address,
				Power:   header.Validators.GetProposer().VotingPower,
			},
			VoteExtension:      extension,
			ExtensionSignature: sign,
			BlockIdFlag:        cmproto.BlockIDFlagCommit,
		}},
	}
	return extendedCommit
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
	headersToSubmit, err := m.pendingHeaders.getPendingHeaders(ctx)
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
	maxBlobSize, err := m.dalc.DA.MaxBlobSize(ctx)
	if err != nil {
		return err
	}
	initialMaxBlobSize := maxBlobSize
	initialGasPrice := m.dalc.GasPrice
	gasPrice := m.dalc.GasPrice

daSubmitRetryLoop:
	for !submittedAllHeaders && attempt < maxSubmitAttempts {
		select {
		case <-ctx.Done():
			break daSubmitRetryLoop
		case <-time.After(backoff):
		}

		res := m.dalc.SubmitHeaders(ctx, headersToSubmit, maxBlobSize, gasPrice)
		switch res.Code {
		case da.StatusSuccess:
			m.logger.Info("successfully submitted Rollkit headers to DA layer", "gasPrice", gasPrice, "daHeight", res.DAHeight, "headerCount", res.SubmittedCount)
			if res.SubmittedCount == uint64(len(headersToSubmit)) {
				submittedAllHeaders = true
			}
			submittedBlocks, notSubmittedBlocks := headersToSubmit[:res.SubmittedCount], headersToSubmit[res.SubmittedCount:]
			numSubmittedHeaders += len(submittedBlocks)
			for _, block := range submittedBlocks {
				m.headerCache.setDAIncluded(block.Hash().String())
				err = m.setDAIncludedHeight(ctx, block.Height())
				if err != nil {
					return err
				}
			}
			lastSubmittedHeight := uint64(0)
			if l := len(submittedBlocks); l > 0 {
				lastSubmittedHeight = submittedBlocks[l-1].Height()
			}
			m.pendingHeaders.setLastSubmittedHeight(ctx, lastSubmittedHeight)
			headersToSubmit = notSubmittedBlocks
			// reset submission options when successful
			// scale back gasPrice gradually
			backoff = 0
			maxBlobSize = initialMaxBlobSize
			if m.dalc.GasMultiplier > 0 && gasPrice != -1 {
				gasPrice = gasPrice / m.dalc.GasMultiplier
				if gasPrice < initialGasPrice {
					gasPrice = initialGasPrice
				}
			}
			m.logger.Debug("resetting DA layer submission options", "backoff", backoff, "gasPrice", gasPrice, "maxBlobSize", maxBlobSize)
		case da.StatusNotIncludedInBlock, da.StatusAlreadyInMempool:
			m.logger.Error("DA layer submission failed", "error", res.Message, "attempt", attempt)
			backoff = m.conf.DABlockTime * time.Duration(m.conf.DAMempoolTTL) //nolint:gosec
			if m.dalc.GasMultiplier > 0 && gasPrice != -1 {
				gasPrice = gasPrice * m.dalc.GasMultiplier
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

func (m *Manager) getLastStateValidators() *cmtypes.ValidatorSet {
	m.lastStateMtx.RLock()
	defer m.lastStateMtx.RUnlock()
	return m.lastState.Validators
}

// Updates the state stored in manager's store along the manager's lastState
func (m *Manager) updateState(ctx context.Context, s types.State) error {
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

func (m *Manager) createBlock(height uint64, lastSignature *types.Signature, lastHeaderHash types.Hash, extendedCommit abci.ExtendedCommitInfo, txs cmtypes.Txs, timestamp time.Time) (*types.SignedHeader, *types.Data, error) {
	m.lastStateMtx.RLock()
	defer m.lastStateMtx.RUnlock()
	return m.executor.CreateBlock(height, lastSignature, extendedCommit, lastHeaderHash, m.lastState, txs, timestamp)
}

func (m *Manager) applyBlock(ctx context.Context, header *types.SignedHeader, data *types.Data) (types.State, *abci.ResponseFinalizeBlock, error) {
	m.lastStateMtx.RLock()
	defer m.lastStateMtx.RUnlock()
	return m.executor.ApplyBlock(ctx, m.lastState, header, data)
}

func updateState(s *types.State, res *abci.ResponseInitChain) error {
	// If the app did not return an app hash, we keep the one set from the genesis doc in
	// the state. We don't set appHash since we don't want the genesis doc app hash
	// recorded in the genesis block. We should probably just remove GenesisDoc.AppHash.
	if len(res.AppHash) > 0 {
		s.AppHash = res.AppHash
	}

	if res.ConsensusParams != nil {
		params := res.ConsensusParams
		if params.Block != nil {
			s.ConsensusParams.Block.MaxBytes = params.Block.MaxBytes
			s.ConsensusParams.Block.MaxGas = params.Block.MaxGas
		}
		if params.Evidence != nil {
			s.ConsensusParams.Evidence.MaxAgeNumBlocks = params.Evidence.MaxAgeNumBlocks
			s.ConsensusParams.Evidence.MaxAgeDuration = params.Evidence.MaxAgeDuration
			s.ConsensusParams.Evidence.MaxBytes = params.Evidence.MaxBytes
		}
		if params.Validator != nil {
			// Copy params.Validator.PubkeyTypes, and set result's value to the copy.
			// This avoids having to initialize the slice to 0 values, and then write to it again.
			s.ConsensusParams.Validator.PubKeyTypes = append([]string{}, params.Validator.PubKeyTypes...)
		}
		if params.Version != nil {
			s.ConsensusParams.Version.App = params.Version.App
		}
		s.Version.Consensus.App = s.ConsensusParams.Version.App
	}
	// We update the last results hash with the empty hash, to conform with RFC-6962.
	s.LastResultsHash = merkle.HashFromByteSlices(nil)

	vals, err := cmtypes.PB2TM.ValidatorUpdates(res.Validators)
	if err != nil {
		return err
	}

	// apply initchain valset change
	nValSet := s.Validators.Copy()
	err = nValSet.UpdateWithChangeSet(vals)
	if err != nil {
		return err
	}
	if len(nValSet.Validators) != 1 {
		return fmt.Errorf("expected exactly one validator")
	}

	s.Validators = cmtypes.NewValidatorSet(nValSet.Validators)
	s.NextValidators = cmtypes.NewValidatorSet(nValSet.Validators).CopyIncrementProposerPriority(1)
	s.LastValidators = cmtypes.NewValidatorSet(nValSet.Validators)

	return nil
}
