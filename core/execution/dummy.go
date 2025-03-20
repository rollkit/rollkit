package execution

import (
	"bytes"
	"context"
	"crypto/sha512"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/rollkit/rollkit/config"
)

//---------------------
// DummyExecutor
//---------------------

// dummyExecutor is a dummy implementation of the dummyExecutor interface for testing
type dummyExecutor struct {
	mu           sync.RWMutex // Add mutex for thread safety
	stateRoot    []byte
	pendingRoots map[uint64][]byte
	maxBytes     uint64
	injectedTxs  [][]byte
}

// BuildGenesis implements Executor.
func (e *dummyExecutor) BuildGenesis(config config.Config) (Genesis, error) {
	panic("unimplemented")
}

// NewDummyExecutor creates a new dummy DummyExecutor instance
func NewDummyExecutor() *dummyExecutor {
	return &dummyExecutor{
		stateRoot:    []byte{1, 2, 3},
		pendingRoots: make(map[uint64][]byte),
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
