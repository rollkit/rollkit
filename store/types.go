package store

import (
	cmstate "github.com/cometbft/cometbft/proto/tendermint/state"
	cmtypes "github.com/cometbft/cometbft/types"

	"github.com/rollkit/rollkit/types"
)

// Store is minimal interface for storing and retrieving blocks, commits and state.
type Store interface {
	// Height returns height of the highest block in store.
	Height() uint64

	// SetHeight sets the height saved in the Store if it is higher than the existing height.
	SetHeight(height uint64)

	// SaveBlock saves block along with its seen commit (which will be included in the next block).
	SaveBlock(block *types.Block, commit *types.Commit) error

	// LoadBlock returns block at given height, or error if it's not found in Store.
	LoadBlock(height uint64) (*types.Block, error)
	// LoadBlockByHash returns block with given block header hash, or error if it's not found in Store.
	LoadBlockByHash(hash types.Hash) (*types.Block, error)

	// SaveBlockResponses saves block responses (events, tx responses, validator set updates, etc) in Store.
	SaveBlockResponses(height uint64, responses *cmstate.ABCIResponses) error

	// LoadBlockResponses returns block results at given height, or error if it's not found in Store.
	LoadBlockResponses(height uint64) (*cmstate.ABCIResponses, error)

	// LoadCommit returns commit for a block at given height, or error if it's not found in Store.
	LoadCommit(height uint64) (*types.Commit, error)
	// LoadCommitByHash returns commit for a block with given block header hash, or error if it's not found in Store.
	LoadCommitByHash(hash types.Hash) (*types.Commit, error)

	// UpdateState updates state saved in Store. Only one State is stored.
	// If there is no State in Store, state will be saved.
	UpdateState(state types.State) error
	// LoadState returns last state saved with UpdateState.
	LoadState() (types.State, error)

	SaveValidators(height uint64, validatorSet *cmtypes.ValidatorSet) error

	LoadValidators(height uint64) (*cmtypes.ValidatorSet, error)
}
