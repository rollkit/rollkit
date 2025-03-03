package node

import (
	"bytes"
	"context"
	"crypto/sha512"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	coresequencer "github.com/rollkit/rollkit/core/sequencer"
	"github.com/rollkit/rollkit/types"
)

//---------------------
// DummyExecutor
//---------------------

// dummyExecutor is a dummy implementation of the dummyExecutor interface for testing
type dummyExecutor struct {
	mu           sync.RWMutex // Add mutex for thread safety
	stateRoot    types.Hash
	pendingRoots map[uint64]types.Hash
	maxBytes     uint64
	injectedTxs  [][]byte
}

// NewDummyExecutor creates a new dummy DummyExecutor instance
func NewDummyExecutor() *dummyExecutor {
	return &dummyExecutor{
		stateRoot:    types.Hash{1, 2, 3},
		pendingRoots: make(map[uint64]types.Hash),
		maxBytes:     1000000,
	}
}

// InitChain initializes the chain state with the given genesis time, initial height, and chain ID.
// It returns the state root hash, the maximum byte size, and an error if the initialization fails.
func (e *dummyExecutor) InitChain(ctx context.Context, genesisTime time.Time, initialHeight uint64, chainID string) ([]byte, uint64, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	hash := sha512.New()
	hash.Write(e.stateRoot)
	e.stateRoot = hash.Sum(nil)
	return e.stateRoot, e.maxBytes, nil
}

// GetTxs returns the list of transactions (types.Tx) within the DummyExecutor instance and an error if any.
func (e *dummyExecutor) GetTxs(context.Context) ([][]byte, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	txs := make([][]byte, len(e.injectedTxs))
	copy(txs, e.injectedTxs) // Create a copy to avoid external modifications
	return txs, nil
}

// InjectTx adds a transaction to the internal list of injected transactions in the DummyExecutor instance.
func (e *dummyExecutor) InjectTx(tx []byte) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.injectedTxs = append(e.injectedTxs, tx)
}

// ExecuteTxs simulate execution of transactions.
func (e *dummyExecutor) ExecuteTxs(ctx context.Context, txs [][]byte, blockHeight uint64, timestamp time.Time, prevStateRoot []byte) ([]byte, uint64, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	hash := sha512.New()
	hash.Write(prevStateRoot)
	for _, tx := range txs {
		hash.Write(tx)
	}
	pending := hash.Sum(nil)
	e.pendingRoots[blockHeight] = pending
	e.removeExecutedTxs(txs)
	return pending, e.maxBytes, nil
}

// SetFinal marks block at given height as finalized.
func (e *dummyExecutor) SetFinal(ctx context.Context, blockHeight uint64) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if pending, ok := e.pendingRoots[blockHeight]; ok {
		e.stateRoot = pending
		delete(e.pendingRoots, blockHeight)
		return nil
	}
	return fmt.Errorf("cannot set finalized block at height %d", blockHeight)
}

func (e *dummyExecutor) removeExecutedTxs(txs [][]byte) {
	e.injectedTxs = slices.DeleteFunc(e.injectedTxs, func(tx []byte) bool {
		return slices.ContainsFunc(txs, func(t []byte) bool { return bytes.Equal(tx, t) })
	})
}

// GetStateRoot returns the current state root in a thread-safe manner
func (e *dummyExecutor) GetStateRoot() []byte {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.stateRoot
}

//---------------------
// DummySequencer
//---------------------

// dummySequencer is a dummy implementation of the Sequencer interface for testing
type dummySequencer struct {
	mu       sync.RWMutex
	batches  map[string]*coresequencer.Batch
	maxBytes atomic.Uint64
	maxGas   atomic.Uint64
}

// NewDummySequencer creates a new dummy Sequencer instance
func NewDummySequencer() coresequencer.Sequencer {
	return &dummySequencer{
		batches: make(map[string]*coresequencer.Batch),
	}
}

// SetMaxBytes sets the max bytes
func (s *dummySequencer) SetMaxBytes(size uint64) {
	s.maxBytes.Store(size)
}

// SetMaxGas sets the max gas
func (s *dummySequencer) SetMaxGas(size uint64) {
	s.maxGas.Store(size)
}

// SubmitRollupBatchTxs submits a batch of transactions to the sequencer
func (s *dummySequencer) SubmitRollupBatchTxs(ctx context.Context, req coresequencer.SubmitRollupBatchTxsRequest) (*coresequencer.SubmitRollupBatchTxsResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.batches[string(req.RollupId)] = req.Batch
	return &coresequencer.SubmitRollupBatchTxsResponse{}, nil
}

// GetNextBatch gets the next batch from the sequencer
func (s *dummySequencer) GetNextBatch(ctx context.Context, req coresequencer.GetNextBatchRequest) (*coresequencer.GetNextBatchResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	batch, ok := s.batches[string(req.RollupId)]
	if !ok {
		return nil, fmt.Errorf("no batch found for rollup ID: %s", string(req.RollupId))
	}

	return &coresequencer.GetNextBatchResponse{
		Batch:     batch,
		Timestamp: time.Now(),
	}, nil
}

// VerifyBatch verifies a batch of transactions received from the sequencer
func (s *dummySequencer) VerifyBatch(ctx context.Context, req coresequencer.VerifyBatchRequest) (*coresequencer.VerifyBatchResponse, error) {
	return &coresequencer.VerifyBatchResponse{
		Status: true,
	}, nil
}
