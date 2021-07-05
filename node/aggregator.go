package node

import (
	"context"
	"fmt"
	"time"

	"github.com/lazyledger/lazyledger-core/proxy"
	lltypes "github.com/lazyledger/lazyledger-core/types"

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
	state state.State

	conf config.AggregatorConfig

	store    store.Store
	dalc     da.DataAvailabilityLayerClient
	executor *state.BlockExecutor

	logger log.Logger
}

// GetInitialState tries to load state from Store, and if it's not available it reads GenesisDoc.
func getInitialState(store store.Store, genesis *lltypes.GenesisDoc) (state.State, error) {
	s, err := store.LoadState()
	if err != nil {
		s, err = state.NewFromGenesisDoc(genesis)
	}
	return s, err
}

func newAggregator(
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

	exec := state.NewBlockExecutor(conf.ProposerAddress, conf.NamespaceID, mempool, proxyApp, logger)

	agg := &aggregator{
		state:    s,
		store:    store,
		executor: exec,
		dalc:     dalc,
	}

	return agg, nil
}

func (n *aggregator) aggregationLoop(ctx context.Context) {
	tick := time.NewTicker(n.conf.BlockTime)
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			err := n.publishBlock(ctx)
			if err != nil {
				n.logger.Error("error while publishing block", "error", err)
			}
		}
	}
}

func (n *aggregator) publishBlock(ctx context.Context) error {
	n.logger.Info("Creating and publishing block")

	// TODO(tzdybal): use block executor here
	var err error
	var block *types.Block
	var commit *types.Commit
	var state state.State

	block = n.executor.CreateBlock(n.store.Height()+1, commit, state)
	n.state, _, err = n.executor.ApplyBlock(ctx, state, block)
	if err != nil {
		return err
	}

	err = n.store.SaveBlock(block, commit)
	if err != nil {
		return err
	}
	return n.broadcastBlock(ctx, block)
}

func (n *aggregator) broadcastBlock(ctx context.Context, block *types.Block) error {
	res := n.dalc.SubmitBlock(block)
	if res.Code != da.StatusSuccess {
		return fmt.Errorf("DA layer submission failed: %s", res.Message)

	}
	return nil
}
