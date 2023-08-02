package block

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

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
	"github.com/rollkit/rollkit/log"
	"github.com/rollkit/rollkit/mempool"
	"github.com/rollkit/rollkit/state"
	"github.com/rollkit/rollkit/store"
	"github.com/rollkit/rollkit/types"
)

// defaultDABlockTime is used only if DABlockTime is not configured for manager
const defaultDABlockTime = 30 * time.Second

// maxSubmitAttempts defines how many times Rollkit will re-try to publish block to DA layer.
// This is temporary solution. It will be removed in future versions.
const maxSubmitAttempts = 30

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

	HeaderCh  chan *types.SignedHeader
	BlockCh   chan *types.Block
	blockInCh chan newBlockEvent
	syncCache map[uint64]*types.Block

	// retrieveMtx is used by retrieveCond
	retrieveMtx *sync.Mutex
	// retrieveCond is used to notify sync goroutine (SyncLoop) that it needs to retrieve data
	retrieveCond *sync.Cond

	logger log.Logger

	// For usage by Lazy Aggregator mode
	buildingBlock     bool
	txsAvailable      <-chan struct{}
	doneBuildingBlock chan struct{}
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
	doneBuildingCh chan struct{},
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
		logger.Info("WARNING: using default DA block time", "DABlockTime", defaultDABlockTime)
		conf.DABlockTime = defaultDABlockTime
	}

	exec := state.NewBlockExecutor(proposerAddress, conf.NamespaceID, genesis.ChainID, mempool, proxyApp, eventBus, logger)
	if s.LastBlockHeight+1 == genesis.InitialHeight {
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
		HeaderCh:          make(chan *types.SignedHeader, 100),
		BlockCh:           make(chan *types.Block, 100),
		blockInCh:         make(chan newBlockEvent, 100),
		retrieveMtx:       new(sync.Mutex),
		lastStateMtx:      new(sync.RWMutex),
		syncCache:         make(map[uint64]*types.Block),
		logger:            logger,
		txsAvailable:      txsAvailableCh,
		doneBuildingBlock: doneBuildingCh,
		buildingBlock:     false,
	}
	agg.retrieveCond = sync.NewCond(agg.retrieveMtx)

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

	//var timer *time.Timer
	timer := time.NewTimer(0)

	if !lazy {
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				start := time.Now()
				err := m.publishBlock(ctx)
				if err != nil {
					m.logger.Error("error while publishing block", "error", err)
				}
				timer.Reset(m.getRemainingSleep(start))
			}
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

// SyncLoop is responsible for syncing blocks.
//
// SyncLoop processes headers gossiped in P2p network to know what's the latest block height,
// block data is retrieved from DA layer.
func (m *Manager) SyncLoop(ctx context.Context, cancel context.CancelFunc) {
	daTicker := time.NewTicker(m.conf.DABlockTime)
	for {
		select {
		case <-daTicker.C:
			m.retrieveCond.Signal()
		case blockEvent := <-m.blockInCh:
			block := blockEvent.block
			daHeight := blockEvent.daHeight
			m.logger.Debug("block body retrieved from DALC",
				"height", block.SignedHeader.Header.Height(),
				"daHeight", daHeight,
				"hash", block.Hash(),
			)
			m.syncCache[block.SignedHeader.Header.BaseHeader.Height] = block
			m.retrieveCond.Signal()

			err := m.trySyncNextBlock(ctx, daHeight)
			if err != nil {
				m.logger.Info("failed to sync next block", "error", err)
			}
		case <-ctx.Done():
			return
		}
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

	b, ok := m.syncCache[currentHeight+1]
	if !ok {
		return nil
	}

	signedHeader := &b.SignedHeader
	if signedHeader != nil {
		commit = &b.SignedHeader.Commit
	}

	if b != nil && commit != nil {
		m.logger.Info("Syncing block", "height", b.SignedHeader.Header.Height())
		newState, responses, err := m.applyBlock(ctx, b)
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

		err = m.store.SaveBlockResponses(uint64(b.SignedHeader.Header.Height()), responses)
		if err != nil {
			return fmt.Errorf("failed to save block responses: %w", err)
		}

		// SaveValidators commits the DB tx
		err = m.saveValidatorsToStore(uint64(b.SignedHeader.Header.Height()))
		if err != nil {
			return err
		}

		m.store.SetHeight(uint64(b.SignedHeader.Header.Height()))

		if daHeight > newState.DAHeight {
			newState.DAHeight = daHeight
		}
		err = m.updateState(newState)
		if err != nil {
			m.logger.Error("failed to save updated state", "error", err)
		}
		delete(m.syncCache, currentHeight+1)
	}

	return nil
}

// RetrieveLoop is responsible for interacting with DA layer.
func (m *Manager) RetrieveLoop(ctx context.Context) {
	// waitCh is used to signal the retrieve loop, that it should process next blocks
	// retrieveCond can be signalled in completely async manner, and goroutine below
	// works as some kind of "buffer" for those signals
	waitCh := make(chan interface{})
	go func() {
		for {
			m.retrieveMtx.Lock()
			m.retrieveCond.Wait()
			waitCh <- nil
			m.retrieveMtx.Unlock()
			if ctx.Err() != nil {
				return
			}
		}
	}()

	for {
		select {
		case <-waitCh:
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}
				daHeight := atomic.LoadUint64(&m.daHeight)
				m.logger.Debug("retrieve", "daHeight", daHeight)
				err := m.processNextDABlock(ctx)
				if err != nil {
					m.logger.Error("failed to retrieve block from DALC", "daHeight", daHeight, "errors", err.Error())
					break
				}
				atomic.AddUint64(&m.daHeight, 1)
			}
		case <-ctx.Done():
			return
		}
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
				m.blockInCh <- newBlockEvent{block, daHeight}
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
		lastHeaderHash = lastBlock.SignedHeader.Header.Hash()
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

	if commit == nil {
		commit, err = m.getCommit(block.SignedHeader.Header)
		if err != nil {
			return err
		}
	}

	// SaveBlock commits the DB tx
	err = m.store.SaveBlock(block, commit)
	if err != nil {
		return err
	}

	err = m.submitBlockToDA(ctx, block)
	if err != nil {
		m.logger.Error("Failed to submit block to DA Layer")
		return err
	}

	blockHeight := uint64(block.SignedHeader.Header.Height())

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

	// Only update the stored height after successfully submitting to DA layer and committing to the DB
	m.store.SetHeight(blockHeight)

	newState.DAHeight = atomic.LoadUint64(&m.daHeight)
	// After this call m.lastState is the NEW state returned from ApplyBlock
	// updateState also commits the DB tx
	err = m.updateState(newState)
	if err != nil {
		return err
	}

	// Publish header to channel so that header exchange service can broadcast
	m.HeaderCh <- &block.SignedHeader

	// Publish block to channel so that block exchange service can broadcast
	m.BlockCh <- block

	m.logger.Debug("successfully proposed block", "proposer", hex.EncodeToString(block.SignedHeader.ProposerAddress), "height", block.SignedHeader.Height())

	return nil
}

func (m *Manager) submitBlockToDA(ctx context.Context, block *types.Block) error {
	m.logger.Info("submitting block to DA layer", "height", block.SignedHeader.Header.Height())

	submitted := false
	backoff := initialBackoff
	for attempt := 1; ctx.Err() == nil && !submitted && attempt <= maxSubmitAttempts; attempt++ {
		res := m.dalc.SubmitBlocks(ctx, []*types.Block{block})
		if res.Code == da.StatusSuccess {
			m.logger.Info("successfully submitted Rollkit block to DA layer", "rollkitHeight", block.SignedHeader.Header.Height(), "daHeight", res.DAHeight)
			submitted = true
		} else {
			m.logger.Error("DA layer submission failed", "error", res.Message, "attempt", attempt)
			time.Sleep(backoff)
			backoff = m.exponentialBackoff(backoff)
		}
	}

	if !submitted {
		return fmt.Errorf("Failed to submit block to DA layer after %d attempts", maxSubmitAttempts)
	}

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
