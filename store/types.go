package store

import (
	abci "github.com/cometbft/cometbft/abci/types"
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

	// GetBlock returns block at given height, or error if it's not found in Store.
	GetBlock(height uint64) (*types.Block, error)
	// GetBlockByHash returns block with given block header hash, or error if it's not found in Store.
	GetBlockByHash(hash types.Hash) (*types.Block, error)

	// SaveBlockResponses saves block responses (events, tx responses, validator set updates, etc) in Store.
	SaveBlockResponses(height uint64, responses *abci.ResponseFinalizeBlock) error

	// LoadBlockResponses returns block results at given height, or error if it's not found in Store.
	GetBlockResponses(height uint64) (*abci.ResponseFinalizeBlock, error)

	// GetCommit returns commit for a block at given height, or error if it's not found in Store.
	GetCommit(height uint64) (*types.Commit, error)
	// GetCommitByHash returns commit for a block with given block header hash, or error if it's not found in Store.
	GetCommitByHash(hash types.Hash) (*types.Commit, error)

	// UpdateState updates state saved in Store. Only one State is stored.
	// If there is no State in Store, state will be saved.
	UpdateState(state types.State) error
	// GetState returns last state saved with UpdateState.
	GetState() (types.State, error)

	SaveValidators(height uint64, validatorSet *cmtypes.ValidatorSet) error

	GetValidators(height uint64) (*cmtypes.ValidatorSet, error)
}
