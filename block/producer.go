package block

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"

	"github.com/rollkit/rollkit/config"
	coreexecutor "github.com/rollkit/rollkit/core/execution"
	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	"github.com/rollkit/rollkit/events"
	"github.com/rollkit/rollkit/store"
	"github.com/rollkit/rollkit/types"
)

// Producer is responsible for creating new blocks
type Producer struct {
	proposerKey      crypto.PrivKey
	store            store.Store
	stateManager     *StateManager
	eventBus         *events.Bus
	config           config.Config
	genesis          *RollkitGenesis
	exec             coreexecutor.Executor
	sequencer        coresequencer.Sequencer
	bq               *BatchQueue
	buildingBlock    bool
	stateMtx         *sync.RWMutex
	logger           Logger
	isProposer       bool
	daIncludedHeight uint64
	lastBatchData    [][]byte
}

// ProducerOptions contains options for creating a new Producer
type ProducerOptions struct {
	Config           config.Config
	Genesis          *RollkitGenesis
	ProposerKey      crypto.PrivKey
	Store            store.Store
	EventBus         *events.Bus
	Exec             coreexecutor.Executor
	Logger           Logger
	InitialState     types.State
	StateManager     *StateManager
	IsProposer       bool
	DAIncludedHeight uint64
	BatchQueue       *BatchQueue
	Sequencer        coresequencer.Sequencer
	StateMtx         *sync.RWMutex
}

// NewProducer creates a new block producer
func NewProducer(opts ProducerOptions) *Producer {
	// Get initial batch data from store if available
	var lastBatchData [][]byte
	batchDataBytes, err := opts.Store.GetMetadata(context.Background(), LastBatchDataKey)
	if err == nil {
		lastBatchData, _ = bytesToBatchData(batchDataBytes)
	}

	return &Producer{
		proposerKey:      opts.ProposerKey,
		store:            opts.Store,
		stateManager:     opts.StateManager,
		eventBus:         opts.EventBus,
		config:           opts.Config,
		genesis:          opts.Genesis,
		exec:             opts.Exec,
		sequencer:        opts.Sequencer,
		bq:               opts.BatchQueue,
		buildingBlock:    false,
		stateMtx:         opts.StateMtx,
		logger:           opts.Logger,
		isProposer:       opts.IsProposer,
		daIncludedHeight: opts.DAIncludedHeight,
		lastBatchData:    lastBatchData,
	}
}

// Start starts the producer
func (p *Producer) Start(ctx context.Context) error {
	// Subscribe to events
	p.eventBus.Subscribe(EventSequencerBatch, p.handleSequencerBatch)
	p.eventBus.Subscribe(EventStateUpdated, p.handleStateUpdated)

	// Start block creation loop based on configuration
	if p.isProposer {
		if p.config.Node.LazyAggregator {
			go p.lazyAggregationLoop(ctx)
		} else {
			go p.aggregationLoop(ctx)
		}
	}

	return nil
}

// GetLastBatchData returns the last batch data
func (p *Producer) GetLastBatchData() [][]byte {
	return p.lastBatchData
}

// handleSequencerBatch handles SequencerBatchEvent
func (p *Producer) handleSequencerBatch(ctx context.Context, evt events.Event) {
	batchEvent, ok := evt.(SequencerBatchEvent)
	if !ok {
		p.logger.Error("invalid event type", "expected", "SequencerBatchEvent", "got", fmt.Sprintf("%T", evt))
		return
	}

	if batchEvent.Batch != nil && len(batchEvent.Batch.Data) > 0 {
		// Save the last batch data
		h := convertBatchDataToBytes(batchEvent.Batch.Data)
		if err := p.store.SetMetadata(ctx, LastBatchDataKey, h); err != nil {
			p.logger.Error("error while setting last batch data", "error", err)
		}
		p.lastBatchData = batchEvent.Batch.Data
	}
}

// handleStateUpdated handles StateUpdatedEvent
func (p *Producer) handleStateUpdated(ctx context.Context, evt events.Event) {
	// Update state if needed
}

// lazyAggregationLoop is the main loop for lazy aggregation mode
func (p *Producer) lazyAggregationLoop(ctx context.Context) {
	// start is used to track the start time of the block production period
	start := time.Now()
	// lazyTimer is used to signal when a block should be built in
	// lazy mode to signal that the chain is still live during long
	// periods of inactivity.
	lazyTimer := time.NewTimer(0)
	defer lazyTimer.Stop()

	blockTimer := time.NewTimer(0)
	defer blockTimer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		// the m.bq.notifyCh channel is signalled when batch becomes available in the batch queue
		case _, ok := <-p.bq.notifyCh:
			if ok && !p.buildingBlock {
				// set the buildingBlock flag to prevent multiple calls to reset the time
				p.buildingBlock = true
				// Reset the block timer based on the block time.
				blockTimer.Reset(p.getRemainingSleep(start))
			}
			continue
		case <-lazyTimer.C:
		case <-blockTimer.C:
		}
		// Define the start time for the block production period
		start = time.Now()
		if err := p.publishBlock(ctx); err != nil && ctx.Err() == nil {
			p.logger.Error("error while publishing block", "error", err)
		}
		// unset the buildingBlocks flag
		p.buildingBlock = false
		// Reset the lazyTimer to produce a block even if there
		// are no transactions as a way to signal that the chain
		// is still live.
		lazyTimer.Reset(p.getRemainingSleep(start))
	}
}

// aggregationLoop is the main loop for normal aggregation mode
func (p *Producer) aggregationLoop(ctx context.Context) {
	initialHeight := p.genesis.InitialHeight
	height := p.store.Height()
	var delay time.Duration

	// TODO(tzdybal): double-check when https://github.com/celestiaorg/rollmint/issues/699 is resolved
	if height < initialHeight {
		delay = time.Until(p.genesis.GenesisTime)
	} else {
		p.stateMtx.RLock()
		lastBlockTime := p.getLastBlockTime()
		p.stateMtx.RUnlock()
		delay = time.Until(lastBlockTime.Add(p.config.Node.BlockTime.Duration))
	}

	if delay > 0 {
		p.logger.Info("Waiting to produce block", "delay", delay)
		time.Sleep(delay)
	}

	// blockTimer is used to signal when to build a block based on the
	// rollup block time. A timer is used so that the time to build a block
	// can be taken into account.
	blockTimer := time.NewTimer(0)
	defer blockTimer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-blockTimer.C:
			// Define the start time for the block production period
			start := time.Now()
			if err := p.publishBlock(ctx); err != nil && ctx.Err() == nil {
				p.logger.Error("error while publishing block", "error", err)
			}
			// Reset the blockTimer to signal the next block production
			// period based on the block time.
			blockTimer.Reset(p.getRemainingSleep(start))
		}
	}
}

// publishBlock creates a new block and publishes a BlockCreatedEvent
func (p *Producer) publishBlock(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if !p.isProposer {
		return ErrNotProposer
	}

	if p.config.Node.MaxPendingBlocks != 0 && (p.store.Height()-p.daIncludedHeight) >= p.config.Node.MaxPendingBlocks {
		return fmt.Errorf("refusing to create block: pending blocks [%d] reached limit [%d]",
			p.store.Height()-p.daIncludedHeight, p.config.Node.MaxPendingBlocks)
	}

	// Lock for reading from state
	p.stateMtx.RLock()
	lastState := p.stateManager.GetLastState()
	p.stateMtx.RUnlock()

	// Get transactions from executor
	execTxs, err := p.exec.GetTxs(ctx)
	if err != nil {
		p.logger.Error("failed to get txs from executor", "err", err)
		// Continue but log the state
		p.logger.Info("Current state",
			"height", p.store.Height(),
			"isProposer", p.isProposer)
	}

	// Submit transactions to sequencer if we have a sequencer configured
	if p.sequencer != nil && len(execTxs) > 0 {
		p.logger.Debug("Submitting transaction to sequencer", "txCount", len(execTxs))
		_, err = p.sequencer.SubmitRollupBatchTxs(ctx, coresequencer.SubmitRollupBatchTxsRequest{
			RollupId: []byte(p.genesis.ChainID),
			Batch:    &coresequencer.Batch{Transactions: execTxs},
		})
		if err != nil {
			p.logger.Error("failed to submit rollup transaction to sequencer",
				"err", err,
				"chainID", p.genesis.ChainID)
		} else {
			p.logger.Debug("Successfully submitted transaction to sequencer")
		}
	}

	txs, timestamp, batchData, err := p.getTxsFromBatch()
	if err != nil && !errors.Is(err, ErrNoBatch) {
		return fmt.Errorf("failed to get transactions from batch: %w", err)
	}

	if errors.Is(err, ErrNoBatch) {
		// Create an empty block instead of returning
		p.logger.Debug("No batch available, creating empty block")
		txs = [][]byte{}
		now := time.Now()
		timestamp = &now
		batchData = [][]byte{}
	}

	// Get the last block time for monotonicity check
	p.stateMtx.RLock()
	lastBlockTime := p.getLastBlockTime()
	p.stateMtx.RUnlock()

	if timestamp.Before(lastBlockTime) {
		return fmt.Errorf("timestamp is not monotonically increasing: %s < %s", timestamp, lastBlockTime)
	}

	// Get last block information
	var lastSignature *types.Signature
	var lastHeaderHash types.Hash

	height := p.store.Height()
	newHeight := height + 1

	// Special case for genesis block
	if newHeight <= p.genesis.InitialHeight {
		lastSignature = &types.Signature{}
	} else {
		lastSignature, err = p.store.GetSignature(ctx, height)
		if err != nil {
			return fmt.Errorf("error while loading last commit: %w", err)
		}
		lastHeader, _, err := p.store.GetBlockData(ctx, height)
		if err != nil {
			return fmt.Errorf("error while loading last block: %w", err)
		}
		lastHeaderHash = lastHeader.Hash()
	}

	// Create the block
	header, data, err := p.createBlock(ctx, newHeight, txs, *timestamp, lastSignature, lastHeaderHash, batchData)
	if err != nil {
		return err
	}

	p.logger.Debug("block info", "num_tx", len(data.Txs))

	// Extract raw transactions for execution
	rawTxs := make([][]byte, len(data.Txs))
	for i := range data.Txs {
		rawTxs[i] = data.Txs[i]
	}

	// Execute transactions to get new state root
	p.stateMtx.RLock()
	newStateRoot, _, err := p.exec.ExecuteTxs(ctx, rawTxs, header.Height(), header.Time(), lastState.AppHash)
	if err != nil {
		p.stateMtx.RUnlock()
		if ctx.Err() != nil {
			return err
		}
		// If execution fails, panic to halt the node
		panic(err)
	}
	p.stateMtx.RUnlock()

	// Update the header data hash and resign
	header.Header.DataHash = data.Hash()
	signature, err := p.getSignature(header.Header)
	if err != nil {
		return err
	}
	header.Signature = signature

	// Save the updated block data
	err = p.store.SaveBlockData(ctx, header, data, &signature)
	if err != nil {
		return fmt.Errorf("error saving updated block data: %w", err)
	}

	// Create new state with updated block info
	p.stateMtx.RLock()
	newState := types.State{
		Version:         lastState.Version,
		ChainID:         lastState.ChainID,
		InitialHeight:   lastState.InitialHeight,
		LastBlockHeight: header.Height(),
		LastBlockTime:   header.Time(),
		AppHash:         newStateRoot,
		DAHeight:        p.daIncludedHeight,
	}
	p.stateMtx.RUnlock()

	// Finalize the block
	err = p.exec.SetFinal(ctx, header.Height())
	if err != nil {
		return fmt.Errorf("failed to finalize block: %w", err)
	}

	// Update the store height
	p.store.SetHeight(ctx, header.Height())

	// Publish state updated event
	p.eventBus.Publish(StateUpdatedEvent{
		State:  newState,
		Height: header.Height(),
	})

	// Publish the block created event
	p.eventBus.Publish(BlockCreatedEvent{
		Header: header,
		Data:   data,
		Height: header.Height(),
	})

	p.logger.Debug("successfully proposed block", "proposer", header.ProposerAddress, "height", header.Height())

	return nil
}

// getRemainingSleep calculates the remaining sleep time based on config and a start time.
func (p *Producer) getRemainingSleep(start time.Time) time.Duration {
	elapsed := time.Since(start)
	interval := p.config.Node.BlockTime.Duration

	if p.config.Node.LazyAggregator {
		if p.buildingBlock && elapsed >= interval {
			// Special case to give time for transactions to accumulate if we
			// are coming out of a period of inactivity.
			return (interval * time.Duration(defaultLazySleepPercent) / 100)
		} else if !p.buildingBlock {
			interval = p.config.Node.LazyBlockTime.Duration
		}
	}

	if elapsed < interval {
		return interval - elapsed
	}

	return 0
}

// getTxsFromBatch retrieves transactions from the batch queue
func (p *Producer) getTxsFromBatch() ([][]byte, *time.Time, [][]byte, error) {
	batch := p.bq.Next()
	if batch == nil {
		// batch is nil when there is nothing to process
		return nil, nil, nil, ErrNoBatch
	}
	return batch.Transactions, &batch.Time, batch.Data, nil
}

// getLastBlockTime returns the timestamp of the last block
func (p *Producer) getLastBlockTime() time.Time {
	state := p.stateManager.GetLastState()
	return state.LastBlockTime
}

// createBlock creates a new block
func (p *Producer) createBlock(ctx context.Context, height uint64, txs [][]byte, timestamp time.Time, lastSignature *types.Signature, lastHeaderHash types.Hash, batchData [][]byte) (*types.SignedHeader, *types.Data, error) {
	batchDataBytes := convertBatchDataToBytes(batchData)

	// Get current state
	state := p.stateManager.GetLastState()

	// Create the block header
	header := &types.SignedHeader{
		Header: types.Header{
			Version: types.Version{
				Block: state.Version.Block,
				App:   state.Version.App,
			},
			BaseHeader: types.BaseHeader{
				ChainID: state.ChainID,
				Height:  height,
				Time:    uint64(timestamp.UnixNano()),
			},
			DataHash:        batchDataBytes,
			ConsensusHash:   make(types.Hash, 32),
			AppHash:         state.AppHash,
			ProposerAddress: p.genesis.ProposerAddress,
			LastHeaderHash:  lastHeaderHash,
		},
		Signature: *lastSignature,
	}

	// Create the block data
	data := &types.Data{
		Txs: make(types.Txs, len(txs)),
		Metadata: &types.Metadata{
			ChainID: header.ChainID(),
			Height:  header.Height(),
			Time:    header.BaseHeader.Time,
		},
	}

	for i := range txs {
		data.Txs[i] = types.Tx(txs[i])
	}

	// Set the data hash
	header.DataHash = data.Hash()

	// Sign the header
	signature, err := p.getSignature(header.Header)
	if err != nil {
		return nil, nil, err
	}
	header.Signature = signature

	return header, data, nil
}

// getSignature signs a header
func (p *Producer) getSignature(header types.Header) (types.Signature, error) {
	b, err := header.MarshalBinary()
	if err != nil {
		return nil, err
	}
	return p.proposerKey.Sign(b)
}
