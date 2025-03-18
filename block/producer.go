package block

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"

	"github.com/rollkit/rollkit/config"
	coreexecutor "github.com/rollkit/rollkit/core/execution"
	"github.com/rollkit/rollkit/events"
	"github.com/rollkit/rollkit/store"
	"github.com/rollkit/rollkit/types"
)

// Producer is responsible for creating new blocks
type Producer struct {
	proposerKey      crypto.PrivKey
	store            store.Store
	eventBus         *events.Bus
	config           config.Config
	genesis          *RollkitGenesis
	exec             coreexecutor.Executor
	bq               *BatchQueue
	buildingBlock    bool
	lastStateMtx     *sync.RWMutex
	lastState        types.State
	logger           Logger
	isProposer       bool
	daIncludedHeight uint64
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
	IsProposer       bool
	DAIncludedHeight uint64
}

// NewProducer creates a new block producer
func NewProducer(opts ProducerOptions) *Producer {
	return &Producer{
		proposerKey:      opts.ProposerKey,
		store:            opts.Store,
		eventBus:         opts.EventBus,
		config:           opts.Config,
		genesis:          opts.Genesis,
		exec:             opts.Exec,
		bq:               NewBatchQueue(),
		buildingBlock:    false,
		lastStateMtx:     new(sync.RWMutex),
		lastState:        opts.InitialState,
		logger:           opts.Logger,
		isProposer:       opts.IsProposer,
		daIncludedHeight: opts.DAIncludedHeight,
	}
}

// Start starts the producer
func (p *Producer) Start(ctx context.Context) error {
	// Subscribe to events
	p.eventBus.Subscribe(EventSequencerBatch, p.handleSequencerBatch)
	p.eventBus.Subscribe(EventStateUpdated, p.handleStateUpdated)

	// Start batch processing loop
	go p.batchProcessingLoop(ctx)

	if p.isProposer {
		// Start block creation loop based on configuration
		if p.config.Node.LazyAggregator {
			go p.lazyAggregationLoop(ctx)
		} else {
			go p.aggregationLoop(ctx)
		}
	}

	return nil
}

// handleSequencerBatch handles SequencerBatchEvent
func (p *Producer) handleSequencerBatch(ctx context.Context, evt events.Event) {
	batchEvent, ok := evt.(SequencerBatchEvent)
	if !ok {
		p.logger.Error("invalid event type", "expected", "SequencerBatchEvent", "got", fmt.Sprintf("%T", evt))
		return
	}

	if batchEvent.Batch != nil {
		p.bq.AddBatch(*batchEvent.Batch)
	}
}

// handleStateUpdated handles StateUpdatedEvent
func (p *Producer) handleStateUpdated(ctx context.Context, evt events.Event) {
	stateEvent, ok := evt.(StateUpdatedEvent)
	if !ok {
		p.logger.Error("invalid event type", "expected", "StateUpdatedEvent", "got", fmt.Sprintf("%T", evt))
		return
	}

	p.lastStateMtx.Lock()
	defer p.lastStateMtx.Unlock()
	p.lastState = stateEvent.State
}

// batchProcessingLoop processes batches from the sequencer
func (p *Producer) batchProcessingLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(100 * time.Millisecond):
			// Process any new batches periodically
		}
	}
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
		lastBlockTime := p.getLastBlockTime()
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

	txs, timestamp, err := p.getTxsFromBatch()
	if err != nil && err != ErrNoBatch {
		return fmt.Errorf("failed to get transactions from batch: %w", err)
	}

	if err == ErrNoBatch {
		// Create an empty block instead of returning
		txs = [][]byte{}
		now := time.Now()
		timestamp = &now
	}

	// Create the block
	header, data, err := p.createBlock(ctx, p.store.Height()+1, txs, *timestamp)
	if err != nil {
		return err
	}

	// Publish the block created event
	p.eventBus.Publish(BlockCreatedEvent{
		Header: header,
		Data:   data,
		Height: header.Height(),
	})

	return nil
}

// getRemainingSleep calculates the remaining sleep time based on config and a start time.
func (p *Producer) getRemainingSleep(start time.Time) time.Duration {
	elapsed := time.Since(start)
	interval := p.config.Node.BlockTime

	if p.config.Node.LazyAggregator {
		if p.buildingBlock && elapsed >= interval.Duration {
			// Special case to give time for transactions to accumulate if we
			// are coming out of a period of inactivity.
			return (interval.Duration * time.Duration(defaultLazySleepPercent) / 100)
		} else if !p.buildingBlock {
			interval = p.config.Node.LazyBlockTime
		}
	}

	if elapsed < interval.Duration {
		return interval.Duration - elapsed
	}

	return 0
}

// getTxsFromBatch retrieves transactions from the batch queue
func (p *Producer) getTxsFromBatch() ([][]byte, *time.Time, error) {
	batch := p.bq.Next()
	if batch == nil {
		// batch is nil when there is nothing to process
		return nil, nil, ErrNoBatch
	}
	return batch.Transactions, &batch.Time, nil
}

// getLastBlockTime returns the timestamp of the last block
func (p *Producer) getLastBlockTime() time.Time {
	p.lastStateMtx.RLock()
	defer p.lastStateMtx.RUnlock()
	return p.lastState.LastBlockTime
}

// createBlock creates a new block
func (p *Producer) createBlock(ctx context.Context, height uint64, txs [][]byte, timestamp time.Time) (*types.SignedHeader, *types.Data, error) {
	p.lastStateMtx.RLock()
	defer p.lastStateMtx.RUnlock()

	// Get the last block's signature to include in this block
	var lastSignature *types.Signature
	var lastHeaderHash types.Hash
	var lastDataHash types.Hash
	var err error

	// Special case for genesis block
	if height <= p.genesis.InitialHeight {
		lastSignature = &types.Signature{}
	} else {
		lastSignature, err = p.store.GetSignature(ctx, height-1)
		if err != nil {
			return nil, nil, fmt.Errorf("error while loading last commit: %w", err)
		}
		lastHeader, lastData, err := p.store.GetBlockData(ctx, height-1)
		if err != nil {
			return nil, nil, fmt.Errorf("error while loading last block: %w", err)
		}
		lastHeaderHash = lastHeader.Hash()
		lastDataHash = lastData.Hash()
	}

	// Create the block header and data
	header := &types.SignedHeader{
		Header: types.Header{
			Version: types.Version{
				Block: p.lastState.Version.Consensus.Block,
				App:   p.lastState.Version.Consensus.App,
			},
			BaseHeader: types.BaseHeader{
				ChainID: p.lastState.ChainID,
				Height:  height,
				Time:    uint64(timestamp.UnixNano()),
			},
			DataHash:        make(types.Hash, 32),
			ConsensusHash:   make(types.Hash, 32),
			AppHash:         p.lastState.AppHash,
			ProposerAddress: p.genesis.ProposerAddress,
			LastHeaderHash:  lastHeaderHash,
		},
		Signature: *lastSignature,
	}

	data := &types.Data{
		Txs: make(types.Txs, len(txs)),
		Metadata: &types.Metadata{
			ChainID:      header.ChainID(),
			Height:       header.Height(),
			Time:         header.BaseHeader.Time,
			LastDataHash: lastDataHash,
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
