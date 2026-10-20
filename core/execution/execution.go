package execution

import (
	"context"
	"time"
)

// Executor defines the interface that execution clients must implement to be compatible with Evolve.
// This interface enables the separation between consensus and execution layers, allowing for modular
// and pluggable execution environments.
//
// Note: if you are modifying this interface, ensure that all implementations are compatible (evm, abci, protobuf/grpc, etc.)
type Executor interface {
	// InitChain initializes a new blockchain instance with genesis parameters.
	// Requirements:
	// - Must generate initial state root representing empty/genesis state
	// - Must validate and store genesis parameters for future reference
	// - Must ensure idempotency (repeated calls with identical parameters should return same results)
	// - Must return error if genesis parameters are invalid
	//
	// Parameters:
	// - ctx: Context for timeout/cancellation control
	// - genesisTime: timestamp marking chain start time in UTC
	// - initialHeight: First block height (must be > 0)
	// - chainID: Unique identifier string for the blockchain
	//
	// Returns:
	// - stateRoot: Hash representing initial state
	// - err: Any initialization errors
	InitChain(ctx context.Context, genesisTime time.Time, initialHeight uint64, chainID string) (stateRoot []byte, err error)

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
	// - [][]byte: Slice of valid transactions
	// - error: Any errors during transaction retrieval
	GetTxs(ctx context.Context) ([][]byte, error)

	// ExecuteTxs processes transactions to produce a new block state.
	// Requirements:
	// - Must validate state transition against previous state root
	// - Must handle empty transaction list
	// - Must handle gracefully gibberish transactions
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
	// - result: New execution result after executing transactions
	// - err: Any execution errors
	ExecuteTxs(ctx context.Context, txs [][]byte, blockHeight uint64, timestamp time.Time, prevStateRoot []byte) (result ExecuteResult, err error)

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

	// GetExecutionInfo returns current execution layer parameters.
	//
	// Parameters:
	// - ctx: Context for timeout/cancellation control
	//
	// Returns:
	// - info: Current execution parameters
	// - error: Any errors during retrieval
	GetExecutionInfo(ctx context.Context) (ExecutionInfo, error)

	// FilterTxs validates force-included transactions and applies gas and size filtering for all passed txs.
	//
	// The function marks transaction with a filter status. The sequencer knows how to proceed with it:
	// - Transactions passing all filters constraints and that can be included (FilterOK)
	// - Invalid/unparsable force-included transactions (gibberish) (FilterRemove)
	// - Any transactions that would exceed the cumulative gas limit (FilterPostpone)
	//
	// For non-gas-based execution layers (maxGas=0) should not filter by gas.
	//
	// Parameters:
	// - ctx: Context for timeout/cancellation control
	// - txs: All transactions (force-included + mempool)
	// - maxBytes: Maximum cumulative size allowed (0 means no size limit)
	// - maxGas: Maximum cumulative gas allowed (0 means no gas limit)
	// - hasForceIncludedTransaction: Boolean whether force included txs are present
	//
	// Returns:
	// - result: The filter status of all txs. The len(txs) == len(result).
	// - err: Any errors during filtering (not validation errors, which result in filtering)
	FilterTxs(ctx context.Context, txs [][]byte, maxBytes, maxGas uint64, hasForceIncludedTransaction bool) ([]FilterStatus, error)
}

// FilterStatus is the result of FilterTxs tx status.
type FilterStatus int

const (
	// FilterOK is the result of a transaction that will make it to the next batch
	FilterOK FilterStatus = iota
	// FilterRemove is the result of a transaction that will be filtered out because invalid (too big, malformed, etc.)
	FilterRemove
	// FilterPostpone is the result of a valid transaction postponed because it would exceed a size or gas limit.
	FilterPostpone
)

// ExecutionInfo contains execution layer parameters that may change per block.
type ExecutionInfo struct {
	// MaxGas is the maximum gas allowed for transactions in a block.
	// For non-gas-based execution layers, this should be 0.
	MaxGas uint64
}

// ExecuteResult contains execution output that consensus must persist.
type ExecuteResult = struct {
	// UpdatedStateRoot is the new state root after executing transactions.
	UpdatedStateRoot []byte

	// NextProposerAddress is the authoritative proposer address selected by
	// the execution layer to sign block blockHeight+1 (the block immediately
	// after the one just executed). An empty value means the current proposer
	// remains active; execution layers that do not support proposer rotation
	// MUST leave this field empty.
	NextProposerAddress []byte
}

// HeightProvider is an optional interface that execution clients can implement
// to support height synchronization checks between ev-node and the execution layer.
type HeightProvider interface {
	// GetLatestHeight returns the current block height of the execution layer.
	// This is useful for detecting desynchronization between ev-node and the execution layer
	// after crashes or restarts.
	//
	// Parameters:
	// - ctx: Context for timeout/cancellation control
	//
	// Returns:
	// - height: Current block height of the execution layer
	// - error: Any errors during height retrieval
	GetLatestHeight(ctx context.Context) (uint64, error)
}

// Rollbackable is an optional interface that execution clients can implement
// to support automatic rollback when the execution layer is ahead of the target height.
// This enables automatic recovery during rolling restarts when the EL has committed
// blocks that were not replicated to the consensus layer.
//
// Requirements:
// - Only execution layers supporting in-flight rollback should implement this.
type Rollbackable interface {
	// Rollback resets the execution layer head to the specified height.
	Rollback(ctx context.Context, targetHeight uint64) error
}

// ExecPruner is an optional interface that execution clients can implement
// to support height-based pruning of their execution metadata.
type ExecPruner interface {
	// PruneExec should delete execution metadata for all heights up to and
	// including the given height. Implementations should be idempotent and track
	// their own progress so that repeated calls with the same or decreasing
	// heights are cheap no-ops.
	PruneExec(ctx context.Context, height uint64) error
}
