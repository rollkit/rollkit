package execution

import (
	"context"
	"time"
)

// Executor defines the interface that execution clients must implement to be compatible with Rollkit.
// This interface enables the separation between consensus and execution layers, allowing for modular
// and pluggable execution environments.
type Executor interface {
	// InitChain initializes a new blockchain instance with genesis parameters.
	// Requirements:
	// - Must generate initial state root representing empty/genesis state
	// - Must validate and store genesis parameters for future reference
	// - Must ensure idempotency (repeated calls with identical parameters should return same results)
	// - Must return error if genesis parameters are invalid
	// - Must return maxBytes indicating maximum allowed bytes for a set of transactions in a block
	//
	// Parameters:
	// - ctx: Context for timeout/cancellation control
	// - genesisTime: timestamp marking chain start time in UTC
	// - initialHeight: First block height (must be > 0)
	// - chainID: Unique identifier string for the blockchain
	//
	// Returns:
	// - stateRoot: Hash representing initial state
	// - maxBytes: Maximum allowed bytes for transacitons in a block
	// - err: Any initialization errors
	InitChain(ctx context.Context, genesisTime time.Time, initialHeight uint64, chainID string) (stateRoot []byte, maxBytes uint64, err error)

	// GetTxs fetches available transactions from the execution layer's mempool.
	// Requirements:
	// - Must return currently valid transactions only
	// - Must handle empty mempool case gracefully
	// - Must respect context cancellation/timeout
	// - Should perform basic transaction validation
	// - Should not remove transactions from mempool
	// - May remove invalid transactions from mempool
	//
	// Parameters:
	// - ctx: Context for timeout/cancellation control
	//
	// Returns:
	// - []types.Tx: Slice of valid transactions
	// - error: Any errors during transaction retrieval
	GetTxs(ctx context.Context) ([][]byte, error)

	// ExecuteTxs processes transactions to produce a new block state.
	// Requirements:
	// - Must validate state transition against previous state root
	// - Must handle empty transaction list
	// - Must maintain deterministic execution
	// - Must respect context cancellation/timeout
	// - The rest of the rules are defined by the specific execution layer
	//
	// Parameters:
	// - ctx: Context for timeout/cancellation control
	// - txs: Ordered list of transactions to execute
	// - blockHeight: Height of block being created (must be > 0)
	// - timestamp: Block creation time in UTC
	// - prevStateRoot: Previous block's state root hash
	//
	// Returns:
	// - updatedStateRoot: New state root after executing transactions
	// - maxBytes: Maximum allowed transaction size (may change with protocol updates)
	// - err: Any execution errors
	ExecuteTxs(ctx context.Context, txs [][]byte, blockHeight uint64, timestamp time.Time, prevStateRoot []byte) (updatedStateRoot []byte, maxBytes uint64, err error)

	// SetFinal marks a block as finalized at the specified height.
	// Requirements:
	// - Must verify block exists at specified height
	// - Must be idempotent
	// - Must maintain finality guarantees (no reverting finalized blocks)
	// - Must respect context cancellation/timeout
	// - Should clean up any temporary state/resources
	//
	// Parameters:
	// - ctx: Context for timeout/cancellation control
	// - blockHeight: Height of block to finalize
	//
	// Returns:
	// - error: Any errors during finalization
	SetFinal(ctx context.Context, blockHeight uint64) error

	// Rollback reverts the state to the previous block height.
	// This method allows recovery from unrecoverable errors by rolling back
	// the most recent block that has not been finalized.
	// Requirements:
	// - Must only rollback the most recent non-finalized block
	// - Must restore state to the exact state before the last block
	// - Must be atomic - either fully succeeds or leaves state unchanged
	// - Must respect context cancellation/timeout
	// - Must return error if rollback is not possible (e.g., no blocks to rollback)
	// - Must not rollback finalized blocks
	//
	// Parameters:
	// - ctx: Context for timeout/cancellation control
	// - currentHeight: Current block height to rollback from
	//
	// Returns:
	// - previousStateRoot: State root after rollback
	// - error: Any errors during rollback
	Rollback(ctx context.Context, currentHeight uint64) (previousStateRoot []byte, err error)
}
