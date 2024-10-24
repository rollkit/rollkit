package execution

import (
	"time"

	"github.com/rollkit/rollkit/types"
)

/*
 ******************************
 * TO-DO move to go-execution *
 ******************************
 */

// Execute defines a common interface for interacting with the execution client.
type Execute interface {
	// InitChain initializes the blockchain with genesis information.
	InitChain(
		genesisTime time.Time,
		initialHeight uint64,
		chainID string,
	) (
		stateRoot types.Hash,
		maxBytes uint64,
		err error,
	)

	// GetTxs retrieves all available transactions from the execution client's mempool.
	GetTxs() ([]types.Tx, error)

	// ExecuteTxs executes a set of transactions to produce a new block header.
	ExecuteTxs(
		txs []types.Tx,
		blockHeight uint64,
		timestamp time.Time,
		prevStateRoot types.Hash,
	) (
		updatedStateRoot types.Hash,
		maxBytes uint64,
		err error,
	)

	// SetFinal marks a block at the given height as final.
	SetFinal(
		blockHeight uint64,
	) error
}
