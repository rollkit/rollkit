package block

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"cosmossdk.io/log"
	goheader "github.com/celestiaorg/go-header"
	ds "github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p/core/crypto"

	coreda "github.com/rollkit/rollkit/core/da"
	coreexecutor "github.com/rollkit/rollkit/core/execution"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	"github.com/rollkit/rollkit/pkg/cache"
	"github.com/rollkit/rollkit/pkg/config"
	"github.com/rollkit/rollkit/pkg/genesis"
	"github.com/rollkit/rollkit/pkg/signer"
	"github.com/rollkit/rollkit/pkg/store"
	"github.com/rollkit/rollkit/types"
)

const (
	// defaultDABlockTime is used only if DABlockTime is not configured for manager
	defaultDABlockTime = 15 * time.Second

	// defaultBlockTime is used only if BlockTime is not configured for manager
	defaultBlockTime = 1 * time.Second

	// defaultLazyBlockTime is used only if LazyBlockTime is not configured for manager
	defaultLazyBlockTime = 60 * time.Second

	// defaultMempoolTTL is the number of blocks until transaction is dropped from mempool
	defaultMempoolTTL = 25

	// maxSubmitAttempts defines how many times Rollkit will re-try to publish block to DA layer.
	// This is temporary solution. It will be removed in future versions.
	maxSubmitAttempts = 30

	// Applies to most channels, 100 is a large enough buffer to avoid blocking
	channelLength = 100

	// Applies to the headerInCh, 10000 is a large enough number for headers per DA block.
	headerInChLength = 10000

	// DAIncludedHeightKey is the key used for persisting the da included height in store.
	DAIncludedHeightKey = "d"

	// LastBatchDataKey is the key used for persisting the last batch data in store.
	LastBatchDataKey = "l"
)

var (
	// dataHashForEmptyTxs to be used while only syncing headers from DA and no p2p to get the Data for no txs scenarios, the syncing can proceed without getting stuck forever.
	dataHashForEmptyTxs = []byte{110, 52, 11, 156, 255, 179, 122, 152, 156, 165, 68, 230, 187, 120, 10, 44, 120, 144, 29, 63, 179, 55, 56, 118, 133, 17, 163, 6, 23, 175, 160, 29}

	// initialBackoff defines initial value for block submission backoff
	initialBackoff = 100 * time.Millisecond
)

// publishBlockFunc defines the function signature for publishing a block.
// This allows for overriding the behavior in tests.
type publishBlockFunc func(ctx context.Context) error

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

// BatchData is used to pass batch, time and data (da.IDs) to BatchQueue
type BatchData struct {
	*coresequencer.Batch
	time.Time
	Data [][]byte
}

// Manager is responsible for aggregating transactions into blocks.
type Manager struct {
	lastState types.State
	// lastStateMtx is used by lastState
	lastStateMtx *sync.RWMutex
	store        store.Store

	config  config.Config
	genesis genesis.Genesis

	signer signer.Signer

	daHeight *atomic.Uint64

	HeaderCh chan *types.SignedHeader
	DataCh   chan *types.Data

	headerInCh  chan NewHeaderEvent
	headerStore goheader.Store[*types.SignedHeader]

	dataInCh  chan NewDataEvent
	dataStore goheader.Store[*types.Data]

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
	txsAvailable bool

	pendingHeaders *PendingHeaders

	// for reporting metrics
	metrics *Metrics

	exec coreexecutor.Executor

	// daIncludedHeight is rollup height at which all blocks have been included
	// in the DA
	daIncludedHeight atomic.Uint64
	da               coreda.DA
	gasPrice         float64
	gasMultiplier    float64

	sequencer     coresequencer.Sequencer
	lastBatchData [][]byte

	// publishBlock is the function used to publish blocks. It defaults to
	// the manager's publishBlock method but can be overridden for testing.
	publishBlock publishBlockFunc

	// txNotifyCh is used to signal when new transactions are available
	txNotifyCh chan struct{}
}

// getInitialState tries to load lastState from Store, and if it's not available it reads genesis.
func getInitialState(ctx context.Context, genesis genesis.Genesis, signer signer.Signer, store store.Store, exec coreexecutor.Executor, logger log.Logger) (types.State, error) {
	// Load the state from store.
	s, err := store.GetState(ctx)

	if errors.Is(err, ds.ErrNotFound) {
		logger.Info("No state found in store, initializing new state")

		// Initialize genesis block explicitly
		header := types.Header{
			DataHash:        new(types.Data).Hash(),
			ProposerAddress: genesis.ProposerAddress,
			BaseHeader: types.BaseHeader{
				ChainID: genesis.ChainID,
				Height:  genesis.InitialHeight,
				Time:    uint64(genesis.GenesisDAStartTime.UnixNano()),
			}}

		var signature types.Signature

		var pubKey crypto.PubKey
		// The signer is only provided in aggregator nodes. This enables the creation of a signed genesis header,
		// which includes a public key and a cryptographic signature for the header.
		// In a full node (non-aggregator), the signer will be nil, and only an unsigned genesis header will be initialized locally.
		if signer != nil {
			pubKey, err = signer.GetPublic()
			if err != nil {
				return types.State{}, fmt.Errorf("failed to get public key: %w", err)
			}

			b, err := header.MarshalBinary()
			if err != nil {
				return types.State{}, err
			}
			signature, err = signer.Sign(b)
			if err != nil {
				return types.State{}, fmt.Errorf("failed to get header signature: %w", err)
			}
		}

		err = store.SaveBlockData(ctx,
			&types.SignedHeader{
				Header: header,
				Signer: types.Signer{
					PubKey:  pubKey,
					Address: genesis.ProposerAddress,
				},
				Signature: signature,
			},
			&types.Data{},
			&signature,
		)
		if err != nil {
			return types.State{}, fmt.Errorf("failed to save genesis block: %w", err)
		}

		// If the user is starting a fresh chain (or hard-forking), we assume the stored state is empty.
		// TODO(tzdybal): handle max bytes
		stateRoot, _, err := exec.InitChain(ctx, genesis.GenesisDAStartTime, genesis.InitialHeight, genesis.ChainID)
		if err != nil {
			logger.Error("error while initializing chain", "error", err)
			return types.State{}, err
		}

		s := types.State{
			Version:         types.Version{},
			ChainID:         genesis.ChainID,
			InitialHeight:   genesis.InitialHeight,
			LastBlockHeight: genesis.InitialHeight - 1,
			LastBlockTime:   genesis.GenesisDAStartTime,
			AppHash:         stateRoot,
			DAHeight:        0,
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
	signer signer.Signer,
	config config.Config,
	genesis genesis.Genesis,
	store store.Store,
	exec coreexecutor.Executor,
	sequencer coresequencer.Sequencer,
	da coreda.DA,
	logger log.Logger,
	headerStore goheader.Store[*types.SignedHeader],
	dataStore goheader.Store[*types.Data],
	seqMetrics *Metrics,
	gasPrice float64,
	gasMultiplier float64,
) (*Manager, error) {
	s, err := getInitialState(ctx, genesis, signer, store, exec, logger)
	if err != nil {
		logger.Error("error while getting initial state", "error", err)
		return nil, err
	}
	//set block height in store
	err = store.SetHeight(ctx, s.LastBlockHeight)
	if err != nil {
		return nil, err
	}

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

	if config.Node.LazyBlockInterval.Duration == 0 {
		logger.Info("Using default lazy block time", "LazyBlockTime", defaultLazyBlockTime)
		config.Node.LazyBlockInterval.Duration = defaultLazyBlockTime
	}

	if config.DA.MempoolTTL == 0 {
		logger.Info("Using default mempool ttl", "MempoolTTL", defaultMempoolTTL)
		config.DA.MempoolTTL = defaultMempoolTTL
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

	daH := atomic.Uint64{}
	daH.Store(s.DAHeight)

	agg := &Manager{
		signer:    signer,
		config:    config,
		genesis:   genesis,
		lastState: s,
		store:     store,
		daHeight:  &daH,
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
		txsAvailable:   false,
		pendingHeaders: pendingHeaders,
		metrics:        seqMetrics,
		sequencer:      sequencer,
		exec:           exec,
		da:             da,
		gasPrice:       gasPrice,
		gasMultiplier:  gasMultiplier,
		txNotifyCh:     make(chan struct{}, 1), // Non-blocking channel
	}
	agg.init(ctx)
	// Set the default publishBlock implementation
	agg.publishBlock = agg.publishBlockInternal
	return agg, nil
}

// PendingHeaders returns the pending headers.
func (m *Manager) PendingHeaders() *PendingHeaders {
	return m.pendingHeaders
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
		m.daIncludedHeight.Store(binary.LittleEndian.Uint64(height))
	}
}

// GetDAIncludedHeight returns the rollup height at which all blocks have been
// included in the DA
func (m *Manager) GetDAIncludedHeight() uint64 {
	return m.daIncludedHeight.Load()
}

// isProposer returns whether or not the manager is a proposer
func isProposer(signer signer.Signer, pubkey crypto.PubKey) (bool, error) {
	if signer == nil {
		return false, nil
	}
	pubKey, err := signer.GetPublic()
	if err != nil {
		return false, err
	}
	if pubKey == nil {
		return false, errors.New("public key is nil")
	}
	if !pubKey.Equals(pubkey) {
		return false, nil
	}
	return true, nil
}

// SetLastState is used to set lastState used by Manager.
func (m *Manager) SetLastState(state types.State) {
	m.lastStateMtx.Lock()
	defer m.lastStateMtx.Unlock()
	m.lastState = state
}

// GetStoreHeight returns the manager's store height
func (m *Manager) GetStoreHeight(ctx context.Context) (uint64, error) {
	return m.store.Height(ctx)
}

// IsBlockHashSeen returns true if the block with the given hash has been seen.
func (m *Manager) IsBlockHashSeen(blockHash string) bool {
	return m.headerCache.IsSeen(blockHash)
}

// IsDAIncluded returns true if the block with the given hash has been seen on DA.
// TODO(tac0turtle): should we use this for pending header system to verify how far ahead a rollup is?
func (m *Manager) IsDAIncluded(hash types.Hash) bool {
	return m.headerCache.IsDAIncluded(hash.String())
}

// GetExecutor returns the executor used by the manager.
//
// Note: this is a temporary method to allow testing the manager.
// It will be removed once the manager is fully integrated with the execution client.
// TODO(tac0turtle): remove
func (m *Manager) GetExecutor() coreexecutor.Executor {
	return m.exec
}

func (m *Manager) retrieveBatch(ctx context.Context) (*BatchData, error) {
	m.logger.Debug("Attempting to retrieve next batch",
		"chainID", m.genesis.ChainID,
		"lastBatchData", m.lastBatchData)

	req := coresequencer.GetNextBatchRequest{
		RollupId:      []byte(m.genesis.ChainID),
		LastBatchData: m.lastBatchData,
	}

	res, err := m.sequencer.GetNextBatch(ctx, req)
	if err != nil {
		return nil, err
	}

	if res != nil && res.Batch != nil {
		m.logger.Debug("Retrieved batch",
			"txCount", len(res.Batch.Transactions),
			"timestamp", res.Timestamp)

		var errRetrieveBatch error
		// Even if there are no transactions, return the batch with timestamp
		// This allows empty blocks to maintain proper timing
		if len(res.Batch.Transactions) == 0 {
			errRetrieveBatch = ErrNoBatch
		}
		// Even if there are no transactions, update lastBatchData so we don’t
		// repeatedly emit the same empty batch, and persist it to metadata.
		if err := m.store.SetMetadata(ctx, LastBatchDataKey, convertBatchDataToBytes(res.BatchData)); err != nil {
			m.logger.Error("error while setting last batch hash", "error", err)
		}
		m.lastBatchData = res.BatchData
		return &BatchData{Batch: res.Batch, Time: res.Timestamp, Data: res.BatchData}, errRetrieveBatch
	}
	h := convertBatchDataToBytes(res.BatchData)
	if err := m.store.SetMetadata(ctx, LastBatchDataKey, h); err != nil {
		m.logger.Error("error while setting last batch hash", "error", err)
	}
	m.lastBatchData = res.BatchData
	return &BatchData{Batch: res.Batch, Time: res.Timestamp, Data: res.BatchData}, nil
}

func (m *Manager) isUsingExpectedSingleSequencer(header *types.SignedHeader) bool {
	return bytes.Equal(header.ProposerAddress, m.genesis.ProposerAddress) && header.ValidateBasic() == nil
}

// publishBlockInternal is the internal implementation for publishing a block.
// It's assigned to the publishBlock field by default.
func (m *Manager) publishBlockInternal(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if m.config.Node.MaxPendingBlocks != 0 && m.pendingHeaders.numPendingHeaders() >= m.config.Node.MaxPendingBlocks {
		return fmt.Errorf("refusing to create block: pending blocks [%d] reached limit [%d]",
			m.pendingHeaders.numPendingHeaders(), m.config.Node.MaxPendingBlocks)
	}

	var (
		lastSignature  *types.Signature
		lastHeaderHash types.Hash
		lastDataHash   types.Hash
		lastHeaderTime time.Time
		err            error
	)

	height, err := m.store.Height(ctx)
	if err != nil {
		return fmt.Errorf("error while getting height: %w", err)
	}

	newHeight := height + 1
	// this is a special case, when first block is produced - there is no previous commit
	if newHeight <= m.genesis.InitialHeight {
		// Special handling for genesis block
		lastSignature = &types.Signature{}
	} else {
		lastSignature, err = m.store.GetSignature(ctx, height)
		if err != nil {
			return fmt.Errorf("error while loading last commit: %w, height: %d", err, height)
		}
		lastHeader, lastData, err := m.store.GetBlockData(ctx, height)
		if err != nil {
			return fmt.Errorf("error while loading last block: %w, height: %d", err, height)
		}
		lastHeaderHash = lastHeader.Hash()
		lastDataHash = lastData.Hash()
		lastHeaderTime = lastHeader.Time()
	}

	var (
		header    *types.SignedHeader
		data      *types.Data
		signature types.Signature
	)

	// Check if there's an already stored block at a newer height
	// If there is use that instead of creating a new block
	pendingHeader, pendingData, err := m.store.GetBlockData(ctx, newHeight)
	if err == nil {
		m.logger.Info("Using pending block", "height", newHeight)
		header = pendingHeader
		data = pendingData
	} else {
		batchData, err := m.retrieveBatch(ctx)
		if err != nil {
			if errors.Is(err, ErrNoBatch) {
				if batchData == nil {
					m.logger.Info("No batch retrieved from sequencer, skipping block production")
					return nil
				}
				m.logger.Info("Creating empty block", "height", newHeight)
			} else {
				return fmt.Errorf("failed to get transactions from batch: %w", err)
			}
		} else {
			if batchData.Before(lastHeaderTime) {
				return fmt.Errorf("timestamp is not monotonically increasing: %s < %s", batchData.Time, m.getLastBlockTime())
			}
			m.logger.Info("Creating and publishing block", "height", newHeight)
			m.logger.Debug("block info", "num_tx", len(batchData.Transactions))
		}

		header, data, err = m.createBlock(ctx, newHeight, lastSignature, lastHeaderHash, batchData)
		if err != nil {
			return err
		}

		if err = m.store.SaveBlockData(ctx, header, data, &signature); err != nil {
			return SaveBlockError{err}
		}
	}

	newState, err := m.applyBlock(ctx, header, data)
	if err != nil {
		if ctx.Err() != nil {
			return err
		}
		// if call to applyBlock fails, we halt the node, see https://github.com/cometbft/cometbft/pull/496
		panic(err)
	}

	signature, err = m.getSignature(header.Header)
	if err != nil {
		return err
	}

	// set the signature to current block's signed header
	header.Signature = signature

	if err := header.ValidateBasic(); err != nil {
		// TODO(tzdybal): I think this is could be even a panic, because if this happens, header is FUBAR
		m.logger.Error("header validation error", "error", err)
	}

	// append metadata to Data before validating and saving
	data.Metadata = &types.Metadata{
		ChainID:      header.ChainID(),
		Height:       header.Height(),
		Time:         header.BaseHeader.Time,
		LastDataHash: lastDataHash,
	}
	// Validate the created block before storing
	if err := m.execValidate(m.lastState, header, data); err != nil {
		return fmt.Errorf("failed to validate block: %w", err)
	}

	headerHeight := header.Height()

	headerHash := header.Hash().String()
	m.headerCache.SetSeen(headerHash)

	// SaveBlock commits the DB tx
	err = m.store.SaveBlockData(ctx, header, data, &signature)
	if err != nil {
		return SaveBlockError{err}
	}

	// Commit the new state and block which writes to disk on the proxy app
	appHash, err := m.execCommit(ctx, newState, header, data)
	if err != nil {
		return err
	}
	// Update app hash in state
	newState.AppHash = appHash

	// Update the store height before submitting to the DA layer but after committing to the DB
	err = m.store.SetHeight(ctx, headerHeight)
	if err != nil {
		return err
	}

	newState.DAHeight = m.daHeight.Load()
	// After this call m.lastState is the NEW state returned from ApplyBlock
	// updateState also commits the DB tx
	m.logger.Debug("updating state", "newState", newState)
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

func (m *Manager) recordMetrics(data *types.Data) {
	m.metrics.NumTxs.Set(float64(len(data.Txs)))
	m.metrics.TotalTxs.Add(float64(len(data.Txs)))
	m.metrics.BlockSizeBytes.Set(float64(data.Size()))
	m.metrics.CommittedHeight.Set(float64(data.Metadata.Height))
}

func (m *Manager) exponentialBackoff(backoff time.Duration) time.Duration {
	backoff *= 2
	if backoff == 0 {
		backoff = initialBackoff
	}
	if backoff > m.config.DA.BlockTime.Duration {
		backoff = m.config.DA.BlockTime.Duration
	}
	return backoff
}

func (m *Manager) getLastBlockTime() time.Time {
	m.lastStateMtx.RLock()
	defer m.lastStateMtx.RUnlock()
	return m.lastState.LastBlockTime
}

func (m *Manager) createBlock(ctx context.Context, height uint64, lastSignature *types.Signature, lastHeaderHash types.Hash, batchData *BatchData) (*types.SignedHeader, *types.Data, error) {
	m.lastStateMtx.RLock()
	defer m.lastStateMtx.RUnlock()

	if m.signer == nil {
		return nil, nil, fmt.Errorf("signer is nil; cannot create block")
	}

	key, err := m.signer.GetPublic()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get proposer public key: %w", err)
	}

	// check that the proposer address is the same as the genesis proposer address
	address, err := m.signer.GetAddress()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get proposer address: %w", err)
	}
	if !bytes.Equal(m.genesis.ProposerAddress, address) {
		return nil, nil, fmt.Errorf("proposer address is not the same as the genesis proposer address %x != %x", address, m.genesis.ProposerAddress)
	}

	// Determine if this is an empty block
	isEmpty := batchData.Batch == nil || len(batchData.Transactions) == 0

	// Set the appropriate data hash based on whether this is an empty block
	var dataHash types.Hash
	if isEmpty {
		dataHash = dataHashForEmptyTxs // Use dataHashForEmptyTxs for empty blocks
	} else {
		dataHash = convertBatchDataToBytes(batchData.Data)
	}

	header := &types.SignedHeader{
		Header: types.Header{
			Version: types.Version{
				Block: m.lastState.Version.Block,
				App:   m.lastState.Version.App,
			},
			BaseHeader: types.BaseHeader{
				ChainID: m.lastState.ChainID,
				Height:  height,
				Time:    uint64(batchData.UnixNano()), //nolint:gosec // why is time unix? (tac0turtle)
			},
			LastHeaderHash:  lastHeaderHash,
			DataHash:        dataHash,
			ConsensusHash:   make(types.Hash, 32),
			AppHash:         m.lastState.AppHash,
			ProposerAddress: m.genesis.ProposerAddress,
		},
		Signature: *lastSignature,
		Signer: types.Signer{
			PubKey:  key,
			Address: m.genesis.ProposerAddress,
		},
	}

	// Create block data with appropriate transactions
	blockData := &types.Data{
		Txs: make(types.Txs, 0), // Start with empty transaction list
	}

	// Only add transactions if this is not an empty block
	if !isEmpty {
		blockData.Txs = make(types.Txs, len(batchData.Transactions))
		for i := range batchData.Transactions {
			blockData.Txs[i] = types.Tx(batchData.Transactions[i])
		}
	}

	return header, blockData, nil
}

func (m *Manager) execCreateBlock(ctx context.Context, height uint64, lastSignature *types.Signature, lastHeaderHash types.Hash, lastState types.State, batchData *BatchData) (*types.SignedHeader, *types.Data, error) {
	if m.signer == nil {
		return nil, nil, fmt.Errorf("signer is nil; cannot create block")
	}

	key, err := m.signer.GetPublic()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get proposer public key: %w", err)
	}

	// check that the proposer address is the same as the genesis proposer address
	address, err := m.signer.GetAddress()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get proposer address: %w", err)
	}
	if !bytes.Equal(m.genesis.ProposerAddress, address) {
		return nil, nil, fmt.Errorf("proposer address is not the same as the genesis proposer address %x != %x", address, m.genesis.ProposerAddress)
	}

	// Determine if this is an empty block
	isEmpty := batchData.Batch == nil || len(batchData.Transactions) == 0

	// Set the appropriate data hash based on whether this is an empty block
	var dataHash types.Hash
	if isEmpty {
		dataHash = dataHashForEmptyTxs // Use dataHashForEmptyTxs for empty blocks
	} else {
		dataHash = convertBatchDataToBytes(batchData.Data)
	}

	header := &types.SignedHeader{
		Header: types.Header{
			Version: types.Version{
				Block: m.lastState.Version.Block,
				App:   m.lastState.Version.App,
			},
			BaseHeader: types.BaseHeader{
				ChainID: m.lastState.ChainID,
				Height:  height,
				Time:    uint64(batchData.UnixNano()), //nolint:gosec // why is time unix? (tac0turtle)
			},
			LastHeaderHash:  lastHeaderHash,
			DataHash:        dataHash,
			ConsensusHash:   make(types.Hash, 32),
			AppHash:         m.lastState.AppHash,
			ProposerAddress: m.genesis.ProposerAddress,
		},
		Signature: *lastSignature,
		Signer: types.Signer{
			PubKey:  key,
			Address: m.genesis.ProposerAddress,
		},
	}

	// Create block data with appropriate transactions
	blockData := &types.Data{
		Txs: make(types.Txs, 0), // Start with empty transaction list
	}

	// Only add transactions if this is not an empty block
	if !isEmpty {
		blockData.Txs = make(types.Txs, len(batchData.Transactions))
		for i := range batchData.Transactions {
			blockData.Txs[i] = types.Tx(batchData.Transactions[i])
		}
	}

	return header, blockData, nil
}

func (m *Manager) applyBlock(ctx context.Context, header *types.SignedHeader, data *types.Data) (types.State, error) {
	m.lastStateMtx.RLock()
	defer m.lastStateMtx.RUnlock()
	return m.execApplyBlock(ctx, m.lastState, header, data)
}

func (m *Manager) execValidate(_ types.State, _ *types.SignedHeader, _ *types.Data) error {
	// TODO(tzdybal): implement
	return nil
}

func (m *Manager) execCommit(ctx context.Context, newState types.State, h *types.SignedHeader, _ *types.Data) ([]byte, error) {
	err := m.exec.SetFinal(ctx, h.Height())
	return newState.AppHash, err
}

func (m *Manager) execApplyBlock(ctx context.Context, lastState types.State, header *types.SignedHeader, data *types.Data) (types.State, error) {
	rawTxs := make([][]byte, len(data.Txs))
	for i := range data.Txs {
		rawTxs[i] = data.Txs[i]
	}
	newStateRoot, _, err := m.exec.ExecuteTxs(ctx, rawTxs, header.Height(), header.Time(), lastState.AppHash)
	if err != nil {
		return types.State{}, err
	}

	s, err := m.nextState(lastState, header, newStateRoot)
	if err != nil {
		return types.State{}, err
	}

	return s, nil
}

func (m *Manager) nextState(state types.State, header *types.SignedHeader, stateRoot []byte) (types.State, error) {
	height := header.Height()

	s := types.State{
		Version:         state.Version,
		ChainID:         state.ChainID,
		InitialHeight:   state.InitialHeight,
		LastBlockHeight: height,
		LastBlockTime:   header.Time(),
		AppHash:         stateRoot,
	}
	return s, nil
}

func convertBatchDataToBytes(batchData [][]byte) []byte {
	// If batchData is nil or empty, return an empty byte slice
	if len(batchData) == 0 {
		return []byte{}
	}

	// For a single item, we still need to length-prefix it for consistency
	// First, calculate the total size needed
	// Format: 4 bytes (length) + data for each entry
	totalSize := 0
	for _, data := range batchData {
		totalSize += 4 + len(data) // 4 bytes for length prefix + data length
	}

	// Allocate buffer with calculated capacity
	result := make([]byte, 0, totalSize)

	// Add length-prefixed data
	for _, data := range batchData {
		// Encode length as 4-byte big-endian integer
		lengthBytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(lengthBytes, uint32(len(data)))

		// Append length prefix
		result = append(result, lengthBytes...)

		// Append actual data
		result = append(result, data...)
	}

	return result
}

// bytesToBatchData converts a length-prefixed byte array back to a slice of byte slices
func bytesToBatchData(data []byte) ([][]byte, error) {
	if len(data) == 0 {
		return [][]byte{}, nil
	}

	var result [][]byte
	offset := 0

	for offset < len(data) {
		// Check if we have at least 4 bytes for the length prefix
		if offset+4 > len(data) {
			return nil, fmt.Errorf("corrupted data: insufficient bytes for length prefix at offset %d", offset)
		}

		// Read the length prefix
		length := binary.LittleEndian.Uint32(data[offset : offset+4])
		offset += 4

		// Check if we have enough bytes for the data
		if offset+int(length) > len(data) {
			return nil, fmt.Errorf("corrupted data: insufficient bytes for entry of length %d at offset %d", length, offset)
		}

		// Extract the data entry
		entry := make([]byte, length)
		copy(entry, data[offset:offset+int(length)])
		result = append(result, entry)

		// Move to the next entry
		offset += int(length)
	}

	return result, nil
}

func (m *Manager) getSignature(header types.Header) (types.Signature, error) {
	b, err := header.MarshalBinary()
	if err != nil {
		return nil, err
	}
	if m.signer == nil {
		return nil, fmt.Errorf("signer is nil; cannot sign header")
	}
	return m.signer.Sign(b)
}

// NotifyNewTransactions signals that new transactions are available for processing
// This method will be called by the Reaper when it receives new transactions
func (m *Manager) NotifyNewTransactions() {
	// Non-blocking send to avoid slowing down the transaction submission path
	select {
	case m.txNotifyCh <- struct{}{}:
		// Successfully sent notification
	default:
		// Channel buffer is full, which means a notification is already pending
		// This is fine, as we just need to trigger one block production
	}
}
