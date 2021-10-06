package node

import (
	"context"
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/tendermint/tendermint/proxy"
	lltypes "github.com/tendermint/tendermint/types"

	"github.com/celestiaorg/optimint/config"
	"github.com/celestiaorg/optimint/da"
	"github.com/celestiaorg/optimint/log"
	"github.com/celestiaorg/optimint/mempool"
	"github.com/celestiaorg/optimint/state"
	"github.com/celestiaorg/optimint/store"
	"github.com/celestiaorg/optimint/types"
)

// blockManager is responsible for aggregating transactions into blocks.
type blockManager struct {
	lastState state.State

	conf    config.BlockManagerConfig
	genesis *lltypes.GenesisDoc

	proposerKey crypto.PrivKey

	store    store.Store
	executor *state.BlockExecutor

	dalc      da.DataAvailabilityLayerClient
	retriever da.BlockRetriever

	headerOutCh chan *types.Header
	headerInCh  chan *types.Header

	syncTarget uint64
	blockInCh  chan *types.Block
	retrieveCh chan uint64
	syncCache  map[uint64]*types.Block

	logger log.Logger
}

// getInitialState tries to load lastState from Store, and if it's not available it reads GenesisDoc.
func getInitialState(store store.Store, genesis *lltypes.GenesisDoc) (state.State, error) {
	s, err := store.LoadState()
	if err != nil {
		s, err = state.NewFromGenesisDoc(genesis)
	}
	return s, err
}

func newBlockManager(
	proposerKey crypto.PrivKey,
	conf config.BlockManagerConfig,
	genesis *lltypes.GenesisDoc,
	store store.Store,
	mempool mempool.Mempool,
	proxyApp proxy.AppConnConsensus,
	dalc da.DataAvailabilityLayerClient,
	logger log.Logger,
) (*blockManager, error) {
	s, err := getInitialState(store, genesis)
	if err != nil {
		return nil, err
	}

	proposerAddress, err := proposerKey.GetPublic().Raw()
	if err != nil {
		return nil, err
	}

	exec := state.NewBlockExecutor(proposerAddress, conf.NamespaceID, mempool, proxyApp, logger)

	agg := &blockManager{
		proposerKey: proposerKey,
		conf:        conf,
		genesis:     genesis,
		lastState:   s,
		store:       store,
		executor:    exec,
		dalc:        dalc,
		retriever:   dalc.(da.BlockRetriever), // TODO(tzdybal): do it in more gentle way (after MVP)
		headerOutCh: make(chan *types.Header),
		headerInCh:  make(chan *types.Header),
		blockInCh:   make(chan *types.Block),
		retrieveCh:  make(chan uint64),
		syncCache:   make(map[uint64]*types.Block),
		logger:      logger,
	}

	return agg, nil
}

func (a *blockManager) aggregationLoop(ctx context.Context) {
	timer := time.NewTimer(a.conf.BlockTime)
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			start := time.Now()
			err := a.publishBlock(ctx)
			if err != nil {
				a.logger.Error("error while publishing block", "error", err)
			}
			timer.Reset(a.getRemainingSleep(start))
		}
	}
}

func (a *blockManager) syncLoop(ctx context.Context) {
	for {
		select {
		case header := <-a.headerInCh:
			a.logger.Debug("block header received", "height", header.Height, "hash", header.Hash())
			newHeight := header.Height
			currentHeight := a.store.Height()
			if newHeight > currentHeight {
				// TODO(tzdybal): syncTarget should be atomic
				a.syncTarget = newHeight
				a.retrieveCh <- newHeight
			}
		case block := <-a.blockInCh:
			a.logger.Debug("block body retrieved from DALC",
				"height", block.Header.Height,
				"hash", block.Hash(),
			)
			a.syncCache[block.Header.Height] = block
			currentHeight := a.store.Height() // TODO(tzdybal): maybe store a copy in memory
			b1, ok1 := a.syncCache[currentHeight+1]
			b2, ok2 := a.syncCache[currentHeight+2]
			if ok1 && ok2 {
				newState, _, err := a.executor.ApplyBlock(ctx, a.lastState, b1)
				if err != nil {
					a.logger.Error("failed to ApplyBlock", "error", err)
					continue
				}
				err = a.store.SaveBlock(b1, &b2.LastCommit)
				if err != nil {
					a.logger.Error("failed to save block", "error", err)
					continue
				}

				a.lastState = newState
				err = a.store.UpdateState(a.lastState)
				if err != nil {
					a.logger.Error("failed to save updated state", "error", err)
					continue
				}
				delete(a.syncCache, currentHeight+1)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (a *blockManager) retrieveLoop(ctx context.Context) {
	for {
		select {
		case _ = <-a.retrieveCh:
			// TODO(tzdybal): syncTarget should be atomic
			for h := a.store.Height() + 1; h <= a.syncTarget; h++ {
				a.logger.Debug("trying to retrieve block from DALC", "height", h)
				a.mustRetrieveBlock(ctx, h)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (a *blockManager) mustRetrieveBlock(ctx context.Context, height uint64) {
	// TOOD(tzdybal): extract configuration option
	maxRetries := 10

	for r := 0; r < maxRetries; r++ {
		err := a.fetchBlock(ctx, height)
		if err == nil {
			return
		}
		// TODO(tzdybal): configuration option
		// TODO(tzdybal): exponential backoff
		time.Sleep(100 * time.Millisecond)
	}
	// TODO(tzdybal): this is only temporary solution, for MVP
	panic("failed to retrieve block with DALC")
}

func (a *blockManager) fetchBlock(ctx context.Context, height uint64) error {
	var err error
	blockRes := a.retriever.RetrieveBlock(height)
	switch blockRes.Code {
	case da.StatusSuccess:
		a.blockInCh <- blockRes.Block
	case da.StatusError:
		err = fmt.Errorf("failed to retrieve block: %s", blockRes.Message)
	case da.StatusTimeout:
		err = fmt.Errorf("timeout during retrieve block: %s", blockRes.Message)
	}
	return err
}

func (a *blockManager) getRemainingSleep(start time.Time) time.Duration {
	publishingDuration := time.Since(start)
	sleepDuration := a.conf.BlockTime - publishingDuration
	if sleepDuration < 0 {
		sleepDuration = 0
	}
	return sleepDuration
}

func (a *blockManager) publishBlock(ctx context.Context) error {
	var lastCommit *types.Commit
	var err error
	newHeight := a.store.Height() + 1

	// this is a special case, when first block is produced - there is no previous commit
	if newHeight == uint64(a.genesis.InitialHeight) {
		lastCommit = &types.Commit{Height: a.store.Height(), HeaderHash: [32]byte{}}
	} else {
		lastCommit, err = a.store.LoadCommit(a.store.Height())
		if err != nil {
			return fmt.Errorf("error while loading last commit: %w", err)
		}
	}

	a.logger.Info("Creating and publishing block", "height", newHeight)

	block := a.executor.CreateBlock(newHeight, lastCommit, a.lastState)
	a.logger.Debug("block info", "num_tx", len(block.Data.Txs))
	newState, _, err := a.executor.ApplyBlock(ctx, a.lastState, block)
	if err != nil {
		return err
	}

	headerBytes, err := block.Header.MarshalBinary()
	if err != nil {
		return err
	}
	sign, err := a.proposerKey.Sign(headerBytes)
	if err != nil {
		return err
	}

	commit := &types.Commit{
		Height:     block.Header.Height,
		HeaderHash: block.Header.Hash(),
		Signatures: []types.Signature{sign},
	}
	err = a.store.SaveBlock(block, commit)
	if err != nil {
		return err
	}

	a.lastState = newState
	err = a.store.UpdateState(a.lastState)
	if err != nil {
		return err
	}

	return a.broadcastBlock(ctx, block)
}

func (a *blockManager) broadcastBlock(ctx context.Context, block *types.Block) error {
	res := a.dalc.SubmitBlock(block)
	if res.Code != da.StatusSuccess {
		return fmt.Errorf("DA layer submission failed: %s", res.Message)
	}

	a.headerOutCh <- &block.Header

	return nil
}
