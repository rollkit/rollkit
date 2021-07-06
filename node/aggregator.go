package node

import (
	"context"
	"fmt"
	"time"

	"github.com/lazyledger/lazyledger-core/proxy"
	lltypes "github.com/lazyledger/lazyledger-core/types"
	"github.com/libp2p/go-libp2p-core/crypto"

	"github.com/lazyledger/optimint/config"
	"github.com/lazyledger/optimint/da"
	"github.com/lazyledger/optimint/log"
	"github.com/lazyledger/optimint/mempool"
	"github.com/lazyledger/optimint/state"
	"github.com/lazyledger/optimint/store"
	"github.com/lazyledger/optimint/types"
)

// aggregator is responsible for aggregating transactions into blocks.
type aggregator struct {
	lastState state.State

	conf config.AggregatorConfig

	store    store.Store
	dalc     da.DataAvailabilityLayerClient
	executor *state.BlockExecutor

	logger log.Logger
}

// initialize tries to load lastState from Store, and if it's not available it reads GenesisDoc.
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

	exec := state.NewBlockExecutor(proposerKey, conf.NamespaceID, mempool, proxyApp, logger)

	agg := &aggregator{
		lastState: s,
		store:     store,
		executor:  exec,
		dalc:      dalc,
	}

	return agg, nil
}

func (a *aggregator) aggregationLoop(ctx context.Context) {
	tick := time.NewTicker(a.conf.BlockTime)
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			err := a.publishBlock(ctx)
			if err != nil {
				a.logger.Error("error while publishing block", "error", err)
			}
		}
	}
}

func (a *aggregator) publishBlock(ctx context.Context) error {
	a.logger.Info("Creating and publishing block")

	block := a.executor.CreateBlock(a.store.Height()+1, lastCommit, a.lastState)
	newState, _, err := a.executor.ApplyBlock(ctx, a.lastState, block)
	if err != nil {
		return err
	}

	block := n.executor.CreateBlock(n.store.Height()+1, lastCommit, n.lastState)
	newState, _, err := n.executor.ApplyBlock(ctx, n.lastState, block)
	if err != nil {
		return err
	}

	commit := &types.Commit{
		Height:     block.Header.Height,
		HeaderHash: block.Header.Hash(),
		// TODO(tzdybal): sign
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
	return nil
}
