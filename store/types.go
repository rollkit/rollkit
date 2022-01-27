package store

import (
	tmstate "github.com/tendermint/tendermint/proto/tendermint/state"

	"github.com/celestiaorg/optimint/state"
	"github.com/celestiaorg/optimint/types"
)

// Store is minimal interface for storing and retrieving blocks, commits and state.
type Store interface {
	// Height returns height of the highest block in store.
	Height() uint64

	// SaveBlock saves block along with its seen commit (which will be included in the next block).
	SaveBlock(block *types.Block, commit *types.Commit) error

	// LoadBlock returns block at given height, or error if it's not found in Store.
	LoadBlock(height uint64) (*types.Block, error)
	// LoadBlockByHash returns block with given block header hash, or error if it's not found in Store.
	LoadBlockByHash(hash [32]byte) (*types.Block, error)

	// SaveBlockResponses saves block responses (events, tx responses, validator set updates, etc) in Store.
	SaveBlockResponses(height uint64, responses *tmstate.ABCIResponses) error

	// LoadBlockResponses returns block results at given height, or error if it's not found in Store.
	LoadBlockResponses(height uint64) (*tmstate.ABCIResponses, error)

	// LoadCommit returns commit for a block at given height, or error if it's not found in Store.
	LoadCommit(height uint64) (*types.Commit, error)
	// LoadCommitByHash returns commit for a block with given block header hash, or error if it's not found in Store.
	LoadCommitByHash(hash [32]byte) (*types.Commit, error)

	// UpdateState updates state saved in Store. Only one State is stored.
	// If there is no State in Store, state will be saved.
	UpdateState(state state.State) error
	// LoadState returns last state saved with UpdateState.
	LoadState() (state.State, error)
}
