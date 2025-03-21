package block

import (
	"context"
	"errors"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"

	coreexecutor "github.com/rollkit/rollkit/core/execution"
	"github.com/rollkit/rollkit/store"
	"github.com/rollkit/rollkit/third_party/log"
	"github.com/rollkit/rollkit/types"
)

// getInitialState tries to load lastState from Store, and if it's not available it reads GenesisDoc.
func getInitialState(ctx context.Context, genesis *RollkitGenesis, store store.Store, exec coreexecutor.Executor, logger log.Logger) (types.State, error) {
	// Load the state from store.
	s, err := store.GetState(ctx)

	if errors.Is(err, ds.ErrNotFound) {
		logger.Info("No state found in store, initializing new state")

		// Initialize genesis block explicitly
		err = store.SaveBlockData(ctx,
			&types.SignedHeader{Header: types.Header{
				BaseHeader: types.BaseHeader{
					Height: genesis.InitialHeight,
					Time:   uint64(genesis.GenesisTime.UnixNano()),
				}}},
			&types.Data{},
			&types.Signature{},
		)
		if err != nil {
			return types.State{}, fmt.Errorf("failed to save genesis block: %w", err)
		}

		// If the user is starting a fresh chain (or hard-forking), we assume the stored state is empty.
		// TODO(tzdybal): handle max bytes
		stateRoot, _, err := exec.InitChain(ctx, genesis.GenesisTime, genesis.InitialHeight, genesis.ChainID)
		if err != nil {
			logger.Error("error while initializing chain", "error", err)
			return types.State{}, err
		}

		s := types.State{
			ChainID:         genesis.ChainID,
			InitialHeight:   genesis.InitialHeight,
			LastBlockHeight: genesis.InitialHeight - 1,
			LastBlockID:     cmtypes.BlockID{},
			LastBlockTime:   genesis.GenesisTime,
			AppHash:         stateRoot,
			DAHeight:        0,
			// TODO(tzdybal): we don't need fields below
			Version:                          cmstate.Version{},
			ConsensusParams:                  cmproto.ConsensusParams{},
			LastHeightConsensusParamsChanged: 0,
			LastResultsHash:                  nil,
			Validators:                       nil,
			NextValidators:                   nil,
			LastValidators:                   nil,
			LastHeightValidatorsChanged:      0,
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

// isProposer returns whether or not the manager is a proposer
func isProposer(_ crypto.PrivKey, _ types.State) (bool, error) {
	return true, nil
}
