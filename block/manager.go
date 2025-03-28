package block

import (
	"context"
	"encoding/binary"
	"sync"
	"sync/atomic"
	"time"

	"cosmossdk.io/log"
	goheaderstore "github.com/celestiaorg/go-header/store"
	"github.com/libp2p/go-libp2p/core/crypto"

	coreda "github.com/rollkit/rollkit/core/da"
	coreexecutor "github.com/rollkit/rollkit/core/execution"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	"github.com/rollkit/rollkit/pkg/cache"
	"github.com/rollkit/rollkit/pkg/config"
	"github.com/rollkit/rollkit/pkg/genesis"
	"github.com/rollkit/rollkit/pkg/queue"
	"github.com/rollkit/rollkit/pkg/store"
	"github.com/rollkit/rollkit/types"
)

var (
	// dataHashForEmptyTxs to be used while only syncing headers from DA and no p2p to get the Data for no txs scenarios, the syncing can proceed without getting stuck forever.
	dataHashForEmptyTxs = []byte{110, 52, 11, 156, 255, 179, 122, 152, 156, 165, 68, 230, 187, 120, 10, 44, 120, 144, 29, 63, 179, 55, 56, 118, 133, 17, 163, 6, 23, 175, 160, 29}

	// initialBackoff defines initial value for block submission backoff
	initialBackoff = 100 * time.Millisecond
)

// Manager is responsible for aggregating transactions into blocks.
type Manager struct {
	lastState types.State
	// lastStateMtx is used by lastState
	lastStateMtx *sync.RWMutex
	store        store.Store

	config  config.Config
	genesis genesis.Genesis

	proposerKey crypto.PrivKey

	// daHeight is the height of the latest processed DA block
	daHeight uint64

	HeaderCh chan *types.SignedHeader
	DataCh   chan *types.Data

	headerInCh  chan NewHeaderEvent
	headerStore *goheaderstore.Store[*types.SignedHeader]

	dataInCh  chan NewDataEvent
	dataStore *goheaderstore.Store[*types.Data]

	headerCache *cache.Cache[types.SignedHeader]
	dataCache   *cache.Cache[types.Data]

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

	exec coreexecutor.Executor

	// daIncludedHeight is rollup height at which all blocks have been included
	// in the DA
	daIncludedHeight atomic.Uint64
	dalc             coreda.Client
	gasPrice         float64
	gasMultiplier    float64

	sequencer     coresequencer.Sequencer
	lastBatchData [][]byte
	bq            *queue.Queue[BatchData]
}

// NewManager creates new block Manager.
func NewManager(
	ctx context.Context,
	proposerKey crypto.PrivKey,
	config config.Config,
	genesis genesis.Genesis,
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
	lastBatchDataBytes, err := store.GetMetadata(ctx, LastBatchDataKey)
	if err != nil {
		logger.Error("error while retrieving last batch hash", "error", err)
	}

	lastBatchData, err := bytesToBatchData(lastBatchDataBytes)
	if err != nil {
		logger.Error("error while converting last batch hash", "error", err)
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
		lastBatchData:  lastBatchData,
		headerCache:    cache.NewCache[types.SignedHeader](),
		dataCache:      cache.NewCache[types.Data](),
		retrieveCh:     make(chan struct{}, 1),
		logger:         logger,
		buildingBlock:  false,
		pendingHeaders: pendingHeaders,
		metrics:        seqMetrics,
		isProposer:     isProposer,
		sequencer:      sequencer,
		bq:             queue.New[BatchData](),
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

func (m *Manager) init(ctx context.Context) {
	// initialize da included height
	if height, err := m.store.GetMetadata(ctx, DAIncludedHeightKey); err == nil && len(height) == 8 {
		m.daIncludedHeight.Store(binary.BigEndian.Uint64(height))
	}
}

// SetDALC is used to set DataAvailabilityLayerClient used by Manager.
func (m *Manager) SetDALC(dalc coreda.Client) {
	m.dalc = dalc
}

// isProposer returns whether or not the manager is a proposer
func isProposer(_ crypto.PrivKey, _ types.State) (bool, error) {
	return true, nil
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
	return m.headerCache.IsSeen(blockHash)
}

// IsDAIncluded returns true if the block with the given hash has been seen on DA.
func (m *Manager) IsDAIncluded(hash types.Hash) bool {
	return m.headerCache.IsDAIncluded(hash.String())
}

// GetExecutor returns the executor used by the manager.
//
// Note: this is a temporary method to allow testing the manager.
// It will be removed once the manager is fully integrated with the execution client.
func (m *Manager) GetExecutor() coreexecutor.Executor {
	return m.exec
}

// HeaderSubmissionLoop is responsible for submitting blocks to the DA layer.
func (m *Manager) HeaderSubmissionLoop(ctx context.Context) {
	timer := time.NewTicker(m.config.DA.BlockTime.Duration)
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

func (m *Manager) getSignature(header types.Header) (types.Signature, error) {
	b, err := header.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return m.proposerKey.Sign(b)
}
