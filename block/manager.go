package block

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	goheaderstore "github.com/celestiaorg/go-header/store"
	abci "github.com/cometbft/cometbft/abci/types"
	cmcrypto "github.com/cometbft/cometbft/crypto"
	"github.com/cometbft/cometbft/crypto/merkle"
	cmstate "github.com/cometbft/cometbft/proto/tendermint/state"
	"github.com/cometbft/cometbft/proxy"
	cmtypes "github.com/cometbft/cometbft/types"
	"github.com/libp2p/go-libp2p/core/crypto"
	"go.uber.org/multierr"

	"github.com/rollkit/rollkit/config"
	"github.com/rollkit/rollkit/da"
	"github.com/rollkit/rollkit/mempool"
	"github.com/rollkit/rollkit/state"
	"github.com/rollkit/rollkit/store"
	"github.com/rollkit/rollkit/third_party/log"
	"github.com/rollkit/rollkit/types"
)

// defaultDABlockTime is used only if DABlockTime is not configured for manager
const defaultDABlockTime = 15 * time.Second

// defaultBlockTime is used only if BlockTime is not configured for manager
const defaultBlockTime = 1 * time.Second

// maxSubmitAttempts defines how many times Rollkit will re-try to publish block to DA layer.
// This is temporary solution. It will be removed in future versions.
const maxSubmitAttempts = 30

// Applies to most channels, 100 is a large enough buffer to avoid blocking
const channelLength = 100

// Applies to the blockInCh, 10000 is a large enough number for blocks per DA block.
const blockInChLength = 10000

// initialBackoff defines initial value for block submission backoff
var initialBackoff = 100 * time.Millisecond

type newBlockEvent struct {
	block    *types.Block
	daHeight uint64
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

	dalc      da.DataAvailabilityLayerClient
	retriever da.BlockRetriever
	// daHeight is the height of the latest processed DA block
	daHeight uint64

	HeaderCh chan *types.SignedHeader
	BlockCh  chan *types.Block

	blockInCh  chan newBlockEvent
	blockStore *goheaderstore.Store[*types.Block]

	blockCache *BlockCache

	// blockStoreCh is used to notify sync goroutine (SyncLoop) that it needs to retrieve blocks from blockStore
	blockStoreCh chan struct{}

	// retrieveCond is used to notify sync goroutine (SyncLoop) that it needs to retrieve data
	retrieveCh chan struct{}

	logger log.Logger

	// For usage by Lazy Aggregator mode
	buildingBlock     bool
	txsAvailable      <-chan struct{}
	doneBuildingBlock chan struct{}

	pendingBlocks *PendingBlocks
}

// getInitialState tries to load lastState from Store, and if it's not available it reads GenesisDoc.
func getInitialState(store store.Store, genesis *cmtypes.GenesisDoc) (types.State, error) {
	s, err := store.LoadState()
	if err != nil {
		s, err = types.NewFromGenesisDoc(genesis)
	}
	return s, err
}

// NewManager creates new block Manager.
func NewManager(
	proposerKey crypto.PrivKey,
	conf config.BlockManagerConfig,
	genesis *cmtypes.GenesisDoc,
	store store.Store,
	mempool mempool.Mempool,
	proxyApp proxy.AppConnConsensus,
	dalc da.DataAvailabilityLayerClient,
	eventBus *cmtypes.EventBus,
	logger log.Logger,
	blockStore *goheaderstore.Store[*types.Block],
) (*Manager, error) {
	s, err := getInitialState(store, genesis)
	if err != nil {
		return nil, err
	}
	if s.DAHeight < conf.DAStartHeight {
		s.DAHeight = conf.DAStartHeight
	}

	proposerAddress, err := getAddress(proposerKey)
	if err != nil {
		return nil, err
	}

	if conf.DABlockTime == 0 {
		logger.Info("Using default DA block time", "DABlockTime", defaultDABlockTime)
		conf.DABlockTime = defaultDABlockTime
	}

	if conf.BlockTime == 0 {
		logger.Info("Using default block time", "BlockTime", defaultBlockTime)
		conf.BlockTime = defaultBlockTime
	}

	exec := state.NewBlockExecutor(proposerAddress, conf.NamespaceID, genesis.ChainID, mempool, proxyApp, eventBus, logger)
	if s.LastBlockHeight+1 == uint64(genesis.InitialHeight) {
		res, err := exec.InitChain(genesis)
		if err != nil {
			return nil, err
		}

		updateState(&s, res)
		if err := store.UpdateState(s); err != nil {
			return nil, err
		}
	}

	var txsAvailableCh <-chan struct{}
	if mempool != nil {
		txsAvailableCh = mempool.TxsAvailable()
	} else {
		txsAvailableCh = nil
	}

	agg := &Manager{
		proposerKey: proposerKey,
		conf:        conf,
		genesis:     genesis,
		lastState:   s,
		store:       store,
		executor:    exec,
		dalc:        dalc,
		retriever:   dalc.(da.BlockRetriever), // TODO(tzdybal): do it in more gentle way (after MVP)
		daHeight:    s.DAHeight,
		// channels are buffered to avoid blocking on input/output operations, buffer sizes are arbitrary
		HeaderCh:          make(chan *types.SignedHeader, channelLength),
		BlockCh:           make(chan *types.Block, channelLength),
		blockInCh:         make(chan newBlockEvent, blockInChLength),
		blockStoreCh:      make(chan struct{}, 1),
		blockStore:        blockStore,
		lastStateMtx:      new(sync.RWMutex),
		blockCache:        NewBlockCache(),
		retrieveCh:        make(chan struct{}, 1),
		logger:            logger,
		txsAvailable:      txsAvailableCh,
		doneBuildingBlock: make(chan struct{}),
		buildingBlock:     false,
		pendingBlocks:     NewPendingBlocks(),
	}
	return agg, nil
}

func getAddress(key crypto.PrivKey) ([]byte, error) {
	rawKey, err := key.GetPublic().Raw()
	if err != nil {
		return nil, err
	}
	return cmcrypto.AddressHash(rawKey), nil
}

// SetDALC is used to set DataAvailabilityLayerClient used by Manager.
func (m *Manager) SetDALC(dalc da.DataAvailabilityLayerClient) {
	m.dalc = dalc
	m.retriever = dalc.(da.BlockRetriever)
}

// GetStoreHeight returns the manager's store height
func (m *Manager) GetStoreHeight() uint64 {
	return m.store.Height()
}

// GetHardConfirmation returns true if the block is hard confirmed
func (m *Manager) GetHardConfirmation(hash types.Hash) bool {
	return m.blockCache.isHardConfirmed(hash.String())
}

// AggregationLoop is responsible for aggregating transactions into rollup-blocks.
func (m *Manager) AggregationLoop(ctx context.Context, lazy bool) {
	initialHeight := uint64(m.genesis.InitialHeight)
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

	timer := time.NewTimer(0)

	if !lazy {
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
			start := time.Now()
			err := m.publishBlock(ctx)
			if err != nil {
				m.logger.Error("error while publishing block", "error", err)
			}
			timer.Reset(m.getRemainingSleep(start))
		}
	} else {
		for {
			select {
			case <-ctx.Done():
				return
			// the buildBlock channel is signalled when Txns become available
			// in the mempool, or after transactions remain in the mempool after
			// building a block.
			case <-m.txsAvailable:
				if !m.buildingBlock {
					m.buildingBlock = true
					timer.Reset(1 * time.Second)
				}
			case <-timer.C:
				// build a block with all the transactions received in the last 1 second
				err := m.publishBlock(ctx)
				if err != nil {
					m.logger.Error("error while publishing block", "error", err)
				}
				// this can be used to notify multiple subscribers when a block has been built
				// intended to help improve the UX of lightclient frontends and wallets.
				close(m.doneBuildingBlock)
				m.doneBuildingBlock = make(chan struct{})
				m.buildingBlock = false
			}
		}
	}
}

// BlockSubmissionLoop is responsible for submitting blocks to the DA layer.
func (m *Manager) BlockSubmissionLoop(ctx context.Context) {
	timer := time.NewTicker(m.conf.DABlockTime)
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		if m.pendingBlocks.isEmpty() {
			continue
		}
		err := m.submitBlocksToDA(ctx)
		if err != nil {
			m.logger.Error("error while submitting block to DA", "error", err)
		}
	}
}

// SyncLoop is responsible for syncing blocks.
//
// SyncLoop processes headers gossiped in P2P network to know what's the latest block height,
// block data is retrieved from DA layer.
func (m *Manager) SyncLoop(ctx context.Context, cancel context.CancelFunc) {
	daTicker := time.NewTicker(m.conf.DABlockTime)
	blockTicker := time.NewTicker(m.conf.BlockTime)
	for {
		select {
		case <-daTicker.C:
			m.sendNonBlockingSignalToRetrieveCh()
		case <-blockTicker.C:
			m.sendNonBlockingSignalToBlockStoreCh()
		case blockEvent := <-m.blockInCh:
			block := blockEvent.block
			daHeight := blockEvent.daHeight
			blockHash := block.Hash().String()
			blockHeight := uint64(block.Height())
			m.logger.Debug("block body retrieved",
				"height", blockHeight,
				"daHeight", daHeight,
				"hash", blockHash,
			)
			if m.blockCache.isSeen(blockHash) {
				m.logger.Debug("block already seen", "height", blockHeight, "block hash", blockHash)
				continue
			}
			m.blockCache.setBlock(blockHeight, block)

			m.sendNonBlockingSignalToBlockStoreCh()
			m.sendNonBlockingSignalToRetrieveCh()

			err := m.trySyncNextBlock(ctx, daHeight)
			if err != nil {
				m.logger.Info("failed to sync next block", "error", err)
				continue
			}
			m.blockCache.setSeen(blockHash)
		case <-ctx.Done():
			return
		}
	}
}

func (m *Manager) sendNonBlockingSignalToBlockStoreCh() {
	select {
	case m.blockStoreCh <- struct{}{}:
	default:
	}
}

func (m *Manager) sendNonBlockingSignalToRetrieveCh() {
	select {
	case m.retrieveCh <- struct{}{}:
	default:
	}
}

// trySyncNextBlock tries to progress one step (one block) in sync process.
//
// To be able to apply block and height h, we need to have its Commit. It is contained in block at height h+1.
// If block at height h+1 is not available, value of last gossiped commit is checked.
// If commit for block h is available, we proceed with sync process, and remove synced block from sync cache.
func (m *Manager) trySyncNextBlock(ctx context.Context, daHeight uint64) error {
	var commit *types.Commit
	currentHeight := m.store.Height() // TODO(tzdybal): maybe store a copy in memory

	b, ok := m.blockCache.getBlock(currentHeight + 1)
	if !ok {
		return nil
	}

	signedHeader := &b.SignedHeader
	if signedHeader != nil {
		commit = &b.SignedHeader.Commit
	}

	if b != nil && commit != nil {
		bHeight := uint64(b.Height())
		m.logger.Info("Syncing block", "height", bHeight)
		// Validate the received block before applying
		if err := m.executor.Validate(m.lastState, b); err != nil {
			return fmt.Errorf("failed to validate block: %w", err)
		}
		newState, responses, err := m.executor.ApplyBlock(ctx, m.lastState, b)
		if err != nil {
			return fmt.Errorf("failed to ApplyBlock: %w", err)
		}
		err = m.store.SaveBlock(b, commit)
		if err != nil {
			return fmt.Errorf("failed to save block: %w", err)
		}
		_, _, err = m.executor.Commit(ctx, newState, b, responses)
		if err != nil {
			return fmt.Errorf("failed to Commit: %w", err)
		}

		err = m.store.SaveBlockResponses(uint64(bHeight), responses)
		if err != nil {
			return fmt.Errorf("failed to save block responses: %w", err)
		}

		// SaveValidators commits the DB tx
		err = m.saveValidatorsToStore(bHeight)
		if err != nil {
			return err
		}

		m.store.SetHeight(bHeight)

		if daHeight > newState.DAHeight {
			newState.DAHeight = daHeight
		}
		err = m.updateState(newState)
		if err != nil {
			m.logger.Error("failed to save updated state", "error", err)
		}
		m.blockCache.deleteBlock(currentHeight + 1)
	}

	return nil
}

// BlockStoreRetrieveLoop is responsible for retrieving blocks from the Block Store.
func (m *Manager) BlockStoreRetrieveLoop(ctx context.Context) {
	lastBlockStoreHeight := uint64(0)
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.blockStoreCh:
		}
		blockStoreHeight := m.blockStore.Height()
		if blockStoreHeight > lastBlockStoreHeight {
			blocks, err := m.getBlocksFromBlockStore(ctx, lastBlockStoreHeight+1, blockStoreHeight)
			if err != nil {
				m.logger.Error("failed to get blocks from Block Store", "lastBlockHeight", lastBlockStoreHeight, "blockStoreHeight", blockStoreHeight, "errors", err.Error())
				continue
			}
			daHeight := atomic.LoadUint64(&m.daHeight)
			for _, block := range blocks {
				m.logger.Debug("block retrieved from p2p block sync", "blockHeight", block.Height(), "daHeight", daHeight)
				m.blockInCh <- newBlockEvent{block, daHeight}
			}
		}
		lastBlockStoreHeight = blockStoreHeight
	}
}

func (m *Manager) getBlocksFromBlockStore(ctx context.Context, startHeight, endHeight uint64) ([]*types.Block, error) {
	if startHeight > endHeight {
		return nil, fmt.Errorf("startHeight (%d) is greater than endHeight (%d)", startHeight, endHeight)
	}
	if startHeight == 0 {
		startHeight++
	}
	blocks := make([]*types.Block, endHeight-startHeight+1)
	for i := startHeight; i <= endHeight; i++ {
		block, err := m.blockStore.GetByHeight(ctx, i)
		if err != nil {
			return nil, err
		}
		blocks[i-startHeight] = block
	}
	return blocks, nil
}

// RetrieveLoop is responsible for interacting with DA layer.
func (m *Manager) RetrieveLoop(ctx context.Context) {
	// blockFoundCh is used to track when we successfully found a block so
	// that we can continue to try and find blocks that are in the next DA height.
	// This enables syncing faster than the DA block time.
	blockFoundCh := make(chan struct{}, 1)
	defer close(blockFoundCh)
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.retrieveCh:
		case <-blockFoundCh:
		}
		daHeight := atomic.LoadUint64(&m.daHeight)
		err := m.processNextDABlock(ctx)
		if err != nil {
			m.logger.Error("failed to retrieve block from DALC", "daHeight", daHeight, "errors", err.Error())
			continue
		}
		// Signal the blockFoundCh to try and retrieve the next block
		select {
		case blockFoundCh <- struct{}{}:
		default:
		}
		atomic.AddUint64(&m.daHeight, 1)
	}
}

func (m *Manager) processNextDABlock(ctx context.Context) error {
	// TODO(tzdybal): extract configuration option
	maxRetries := 10
	daHeight := atomic.LoadUint64(&m.daHeight)

	var err error
	m.logger.Debug("trying to retrieve block from DA", "daHeight", daHeight)
	for r := 0; r < maxRetries; r++ {
		blockResp, fetchErr := m.fetchBlock(ctx, daHeight)
		if fetchErr == nil {
			if blockResp.Code == da.StatusNotFound {
				m.logger.Debug("no block found", "daHeight", daHeight, "reason", blockResp.Message)
				return nil
			}
			m.logger.Debug("retrieved potential blocks", "n", len(blockResp.Blocks), "daHeight", daHeight)
			for _, block := range blockResp.Blocks {
				blockHash := block.Hash().String()
				m.blockCache.setHardConfirmed(blockHash)
				m.logger.Info("block marked as hard confirmed", "blockHeight", block.Height(), "blockHash", blockHash)
				if !m.blockCache.isSeen(blockHash) {
					m.blockInCh <- newBlockEvent{block, daHeight}
				}
			}
			return nil
		}

		// Track the error
		err = multierr.Append(err, fetchErr)
		// Delay before retrying
		select {
		case <-ctx.Done():
			return err
		case <-time.After(100 * time.Millisecond):
		}
	}
	return err
}

func (m *Manager) fetchBlock(ctx context.Context, daHeight uint64) (da.ResultRetrieveBlocks, error) {
	var err error
	blockRes := m.retriever.RetrieveBlocks(ctx, daHeight)
	switch blockRes.Code {
	case da.StatusError:
		err = fmt.Errorf("failed to retrieve block: %s", blockRes.Message)
	}
	return blockRes, err
}

func (m *Manager) getRemainingSleep(start time.Time) time.Duration {
	publishingDuration := time.Since(start)
	sleepDuration := m.conf.BlockTime - publishingDuration
	if sleepDuration < 0 {
		sleepDuration = 0
	}
	return sleepDuration
}

func (m *Manager) getCommit(header types.Header) (*types.Commit, error) {
	headerBytes, err := header.MarshalBinary()
	if err != nil {
		return nil, err
	}
	sign, err := m.proposerKey.Sign(headerBytes)
	if err != nil {
		return nil, err
	}
	return &types.Commit{
		Signatures: []types.Signature{sign},
	}, nil
}

func (m *Manager) IsProposer() (bool, error) {
	m.lastStateMtx.RLock()
	defer m.lastStateMtx.RUnlock()
	// if proposer is not set, assume self proposer
	if m.lastState.Validators.Proposer == nil {
		return true, nil
	}

	signerPubBytes, err := m.proposerKey.GetPublic().Raw()
	if err != nil {
		return false, err
	}

	return bytes.Equal(m.lastState.Validators.Proposer.PubKey.Bytes(), signerPubBytes), nil
}

func (m *Manager) publishBlock(ctx context.Context) error {
	var lastCommit *types.Commit
	var lastHeaderHash types.Hash
	var err error
	height := m.store.Height()
	newHeight := height + 1

	isProposer, err := m.IsProposer()
	if err != nil {
		return fmt.Errorf("error while checking for proposer: %w", err)
	}
	if !isProposer {
		return nil
	}

	// this is a special case, when first block is produced - there is no previous commit
	if newHeight == uint64(m.genesis.InitialHeight) {
		lastCommit = &types.Commit{}
	} else {
		lastCommit, err = m.store.LoadCommit(height)
		if err != nil {
			return fmt.Errorf("error while loading last commit: %w", err)
		}
		lastBlock, err := m.store.LoadBlock(height)
		if err != nil {
			return fmt.Errorf("error while loading last block: %w", err)
		}
		lastHeaderHash = lastBlock.Hash()
	}

	var block *types.Block
	var commit *types.Commit

	// Check if there's an already stored block at a newer height
	// If there is use that instead of creating a new block
	pendingBlock, err := m.store.LoadBlock(newHeight)
	if err == nil {
		m.logger.Info("Using pending block", "height", newHeight)
		block = pendingBlock
	} else {
		m.logger.Info("Creating and publishing block", "height", newHeight)
		block = m.createBlock(newHeight, lastCommit, lastHeaderHash)
		m.logger.Debug("block info", "num_tx", len(block.Data.Txs))

		block.SignedHeader.DataHash, err = block.Data.Hash()
		if err != nil {
			return nil
		}
		block.SignedHeader.Header.NextAggregatorsHash = m.getNextAggregatorsHash()
		commit, err = m.getCommit(block.SignedHeader.Header)
		if err != nil {
			return err
		}

		// set the commit to current block's signed header
		block.SignedHeader.Commit = *commit

		block.SignedHeader.Validators = m.getLastStateValidators()

		// SaveBlock commits the DB tx
		err = m.store.SaveBlock(block, commit)
		if err != nil {
			return err
		}
	}

	// Apply the block but DONT commit
	newState, responses, err := m.applyBlock(ctx, block)
	if err != nil {
		return err
	}

	// Before taking the hash, we need updated ISRs, hence after ApplyBlock
	block.SignedHeader.Header.DataHash, err = block.Data.Hash()
	if err != nil {
		return err
	}

	block.SignedHeader.Header.NextAggregatorsHash = newState.NextValidators.Hash()

	commit, err = m.getCommit(block.SignedHeader.Header)
	if err != nil {
		return err
	}

	// set the commit to current block's signed header
	block.SignedHeader.Commit = *commit

	block.SignedHeader.Validators = m.getLastStateValidators()

	// Validate the created block before storing
	if err := m.executor.Validate(m.lastState, block); err != nil {
		return fmt.Errorf("failed to validate block: %w", err)
	}

	blockHeight := block.Height()
	// Update the stored height before submitting to the DA layer and committing to the DB
	m.store.SetHeight(blockHeight)

	blockHash := block.Hash().String()
	m.blockCache.setSeen(blockHash)

	// SaveBlock commits the DB tx
	err = m.store.SaveBlock(block, commit)
	if err != nil {
		return err
	}

	// Submit block to be published to the DA layer
	m.pendingBlocks.addPendingBlock(block)

	// Commit the new state and block which writes to disk on the proxy app
	_, _, err = m.executor.Commit(ctx, newState, block, responses)
	if err != nil {
		return err
	}

	// SaveBlockResponses commits the DB tx
	err = m.store.SaveBlockResponses(blockHeight, responses)
	if err != nil {
		return err
	}

	// SaveValidators commits the DB tx
	err = m.saveValidatorsToStore(blockHeight)
	if err != nil {
		return err
	}

	newState.DAHeight = atomic.LoadUint64(&m.daHeight)
	// After this call m.lastState is the NEW state returned from ApplyBlock
	// updateState also commits the DB tx
	err = m.updateState(newState)
	if err != nil {
		return err
	}

	// Check if the node has shutdown prior to publishing to channels
	select {
	case <-ctx.Done():
		return nil
	default:
	}

	// Publish header to channel so that header exchange service can broadcast
	m.HeaderCh <- &block.SignedHeader

	// Publish block to channel so that block exchange service can broadcast
	m.BlockCh <- block

	m.logger.Debug("successfully proposed block", "proposer", hex.EncodeToString(block.SignedHeader.ProposerAddress), "height", blockHeight)

	return nil
}

func (m *Manager) submitBlocksToDA(ctx context.Context) error {
	submitted := false
	backoff := initialBackoff
	for attempt := 1; ctx.Err() == nil && !submitted && attempt <= maxSubmitAttempts; attempt++ {
		res := m.dalc.SubmitBlocks(ctx, m.pendingBlocks.getPendingBlocks())
		if res.Code == da.StatusSuccess {
			m.logger.Info("successfully submitted Rollkit block to DA layer", "daHeight", res.DAHeight)
			submitted = true
		} else {
			m.logger.Error("DA layer submission failed", "error", res.Message, "attempt", attempt)
			time.Sleep(backoff)
			backoff = m.exponentialBackoff(backoff)
		}
	}

	if !submitted {
		return fmt.Errorf("failed to submit block to DA layer after %d attempts", maxSubmitAttempts)
	}
	m.pendingBlocks.resetPendingBlocks()
	return nil
}

func (m *Manager) exponentialBackoff(backoff time.Duration) time.Duration {
	backoff *= 2
	if backoff > m.conf.DABlockTime {
		backoff = m.conf.DABlockTime
	}
	return backoff
}

// Updates the state stored in manager's store along the manager's lastState
func (m *Manager) updateState(s types.State) error {
	m.lastStateMtx.Lock()
	defer m.lastStateMtx.Unlock()
	err := m.store.UpdateState(s)
	if err != nil {
		return err
	}
	m.lastState = s
	return nil
}

func (m *Manager) saveValidatorsToStore(height uint64) error {
	m.lastStateMtx.RLock()
	defer m.lastStateMtx.RUnlock()
	return m.store.SaveValidators(height, m.lastState.Validators)
}

func (m *Manager) getLastStateValidators() *cmtypes.ValidatorSet {
	m.lastStateMtx.RLock()
	defer m.lastStateMtx.RUnlock()
	return m.lastState.Validators
}

func (m *Manager) getNextAggregatorsHash() types.Hash {
	m.lastStateMtx.RLock()
	defer m.lastStateMtx.RUnlock()
	return m.lastState.NextValidators.Hash()
}

func (m *Manager) getLastBlockTime() time.Time {
	m.lastStateMtx.RLock()
	defer m.lastStateMtx.RUnlock()
	return m.lastState.LastBlockTime
}

func (m *Manager) createBlock(height uint64, lastCommit *types.Commit, lastHeaderHash types.Hash) *types.Block {
	m.lastStateMtx.RLock()
	defer m.lastStateMtx.RUnlock()
	return m.executor.CreateBlock(height, lastCommit, lastHeaderHash, m.lastState)
}

func (m *Manager) applyBlock(ctx context.Context, block *types.Block) (types.State, *cmstate.ABCIResponses, error) {
	m.lastStateMtx.RLock()
	defer m.lastStateMtx.RUnlock()
	return m.executor.ApplyBlock(ctx, m.lastState, block)
}
func updateState(s *types.State, res *abci.ResponseInitChain) {
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

	if len(res.Validators) > 0 {
		vals, err := cmtypes.PB2TM.ValidatorUpdates(res.Validators)
		if err != nil {
			// TODO(tzdybal): handle error properly
			panic(err)
		}
		s.Validators = cmtypes.NewValidatorSet(vals)
		s.NextValidators = cmtypes.NewValidatorSet(vals).CopyIncrementProposerPriority(1)
	}
}
