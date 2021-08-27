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

// aggregator is responsible for aggregating transactions into blocks.
type aggregator struct {
	lastState state.State

	conf    config.AggregatorConfig
	genesis *lltypes.GenesisDoc

	proposerKey crypto.PrivKey

	store    store.Store
	dalc     da.DataAvailabilityLayerClient
	executor *state.BlockExecutor

	newHeaders chan *types.Header

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

func newAggregator(
	proposerKey crypto.PrivKey,
	conf config.AggregatorConfig,
	genesis *lltypes.GenesisDoc,
	store store.Store,
	mempool mempool.Mempool,
	proxyApp proxy.AppConnConsensus,
	dalc da.DataAvailabilityLayerClient,
	logger log.Logger,
) (*aggregator, error) {
	s, err := getInitialState(store, genesis)
	if err != nil {
		return nil, err
	}

	proposerAddress, err := proposerKey.GetPublic().Raw()
	if err != nil {
		return nil, err
	}

	exec := state.NewBlockExecutor(proposerAddress, conf.NamespaceID, mempool, proxyApp, logger)

	agg := &aggregator{
		proposerKey: proposerKey,
		conf:        conf,
		genesis:     genesis,
		lastState:   s,
		store:       store,
		executor:    exec,
		dalc:        dalc,
		newHeaders:  make(chan *types.Header),
		logger:      logger,
	}

	return agg, nil
}

func (a *aggregator) aggregationLoop(ctx context.Context) {
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

func (a *aggregator) getRemainingSleep(start time.Time) time.Duration {
	publishingDuration := time.Since(start)
	sleepDuration := a.conf.BlockTime - publishingDuration
	if sleepDuration < 0 {
		sleepDuration = 0
	}
	return sleepDuration
}

func (a *aggregator) publishBlock(ctx context.Context) error {
	a.logger.Info("Creating and publishing block", "height", a.store.Height())

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

func (a *aggregator) broadcastBlock(ctx context.Context, block *types.Block) error {
	res := a.dalc.SubmitBlock(block)
	if res.Code != da.StatusSuccess {
		return fmt.Errorf("DA layer submission failed: %s", res.Message)
	}

	a.newHeaders <- &block.Header

	return nil
}
