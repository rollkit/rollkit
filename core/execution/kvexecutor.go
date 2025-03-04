package execution

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// KVExecutor is a simple in-memory key-value store that implements the Executor interface for testing purposes.
// It maintains an in-memory store and a mempool for transactions. I also add fields to track genesis initialization.

type KVExecutor struct {
	store   map[string]string
	mu      sync.Mutex
	mempool [][]byte
	// genesisInitialized indicates if InitChain has been called
	genesisInitialized bool
	// genesisStateRoot holds the state root computed during genesis initialization
	genesisStateRoot []byte
}

// NewKVExecutor creates a new instance of KVExecutor with initialized store and mempool.
func NewKVExecutor() *KVExecutor {
	return &KVExecutor{
		store:   make(map[string]string),
		mempool: make([][]byte, 0),
	}
}

// SetStoreValue is a helper for the HTTP interface to directly set a key-value pair.
func (k *KVExecutor) SetStoreValue(key, value string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.store[key] = value
}

// GetStoreValue is a helper for the HTTP interface to retrieve the value for a key.
func (k *KVExecutor) GetStoreValue(key string) (string, bool) {
	k.mu.Lock()
	defer k.mu.Unlock()
	value, exists := k.store[key]
	return value, exists
}

// computeStateRoot computes a deterministic state root by sorting keys and concatenating key-value pairs.
func (k *KVExecutor) computeStateRoot() []byte {
	keys := make([]string, 0, len(k.store))
	for key := range k.store {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for _, key := range keys {
		sb.WriteString(fmt.Sprintf("%s:%s;", key, k.store[key]))
	}
	return []byte(sb.String())
}

// InitChain initializes the chain state with genesis parameters.
// If genesis has already been set, it returns the previously computed genesis state root to ensure idempotency.
func (k *KVExecutor) InitChain(ctx context.Context, genesisTime time.Time, initialHeight uint64, chainID string) ([]byte, uint64, error) {
	select {
	case <-ctx.Done():
		return nil, 0, ctx.Err()
	default:
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.genesisInitialized {
		// Genesis already initialized; return stored genesis state root
		return k.genesisStateRoot, 1024, nil
	}
	// Clear the store to simulate genesis initialization
	k.store = make(map[string]string)
	stateRoot := k.computeStateRoot()
	k.genesisStateRoot = stateRoot
	k.genesisInitialized = true
	return stateRoot, 1024, nil
}

// GetTxs retrieves transactions from the mempool without removing them.
func (k *KVExecutor) GetTxs(ctx context.Context) ([][]byte, error) {
	sel := func() ([][]byte, error) {
		k.mu.Lock()
		defer k.mu.Unlock()
		txs := make([][]byte, len(k.mempool))
		copy(txs, k.mempool)
		return txs, nil
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		return sel()
	}
}

// ExecuteTxs processes each transaction assumed to be in the format "key=value".
// It updates the in-memory store accordingly. If a transaction is malformed, an error is returned.
func (k *KVExecutor) ExecuteTxs(ctx context.Context, txs [][]byte, blockHeight uint64, timestamp time.Time, prevStateRoot []byte) ([]byte, uint64, error) {
	select {
	case <-ctx.Done():
		return nil, 0, ctx.Err()
	default:
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	for _, tx := range txs {
		parts := strings.SplitN(string(tx), "=", 2)
		if len(parts) != 2 {
			return nil, 0, errors.New("malformed transaction; expected format key=value")
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key == "" {
			return nil, 0, errors.New("empty key in transaction")
		}
		k.store[key] = value
	}
	return k.computeStateRoot(), 1024, nil
}

// SetFinal marks a block as finalized. In this simple implementation, it validates the block height.
func (k *KVExecutor) SetFinal(ctx context.Context, blockHeight uint64) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if blockHeight == 0 {
		return errors.New("blockHeight must be greater than 0")
	}
	// No additional finalization logic in this minimal implementation.
	return nil
}
