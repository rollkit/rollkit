package executor

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestInitChain_Idempotency(t *testing.T) {
	exec := NewKVExecutor()
	ctx := context.Background()
	genesisTime := time.Now()
	initialHeight := uint64(1)
	chainID := "test-chain"

	// First call initializes genesis state
	stateRoot1, maxBytes1, err := exec.InitChain(ctx, genesisTime, initialHeight, chainID)
	if err != nil {
		t.Fatalf("InitChain failed on first call: %v", err)
	}
	if maxBytes1 != 1024 {
		t.Errorf("Expected maxBytes 1024, got %d", maxBytes1)
	}

	// Second call should return the same genesis state root
	stateRoot2, maxBytes2, err := exec.InitChain(ctx, genesisTime, initialHeight, chainID)
	if err != nil {
		t.Fatalf("InitChain failed on second call: %v", err)
	}
	if !reflect.DeepEqual(stateRoot1, stateRoot2) {
		t.Errorf("Genesis state roots do not match: %s vs %s", stateRoot1, stateRoot2)
	}
	if maxBytes2 != 1024 {
		t.Errorf("Expected maxBytes 1024, got %d", maxBytes2)
	}
}

func TestSetAndGetStoreValue(t *testing.T) {
	exec := NewKVExecutor()
	key := "foo"
	value := "bar"
	exec.SetStoreValue(key, value)

	retValue, exists := exec.GetStoreValue(key)
	if !exists {
		t.Fatalf("Expected key %s to exist", key)
	}
	if retValue != value {
		t.Errorf("Expected value %s, got %s", value, retValue)
	}
}

func TestGetTxs(t *testing.T) {
	exec := NewKVExecutor()
	ctx := context.Background()

	// Append transactions directly to mempool
	exec.mu.Lock()
	exec.mempool = append(exec.mempool, []byte("a=1"), []byte("b=2"))
	exec.mu.Unlock()

	txs, err := exec.GetTxs(ctx)
	if err != nil {
		t.Fatalf("GetTxs returned error: %v", err)
	}
	if len(txs) != 2 {
		t.Errorf("Expected 2 transactions, got %d", len(txs))
	}

	// Modify the returned slice and ensure it does not affect the original
	txs[0] = []byte("modified")
	exec.mu.Lock()
	originalTx := exec.mempool[0]
	exec.mu.Unlock()
	if string(originalTx) == "modified" {
		t.Errorf("GetTxs should return a copy, but original mempool was modified")
	}
}

func TestExecuteTxs_Valid(t *testing.T) {
	exec := NewKVExecutor()
	ctx := context.Background()

	// Clear store first
	exec.mu.Lock()
	exec.store = make(map[string]string)
	exec.mu.Unlock()

	// Prepare valid transactions
	txs := [][]byte{
		[]byte("key1=value1"),
		[]byte("key2=value2"),
	}

	stateRoot, maxBytes, err := exec.ExecuteTxs(ctx, txs, 1, time.Now(), []byte(""))
	if err != nil {
		t.Fatalf("ExecuteTxs failed: %v", err)
	}
	if maxBytes != 1024 {
		t.Errorf("Expected maxBytes 1024, got %d", maxBytes)
	}

	// Check that stateRoot contains the updated key-value pairs
	rootStr := string(stateRoot)
	if !strings.Contains(rootStr, "key1:value1;") || !strings.Contains(rootStr, "key2:value2;") {
		t.Errorf("State root does not contain expected key-values: %s", rootStr)
	}
}

func TestExecuteTxs_Invalid(t *testing.T) {
	exec := NewKVExecutor()
	ctx := context.Background()

	// Prepare an invalid transaction (missing '=')
	txs := [][]byte{
		[]byte("invalidformat"),
	}

	_, _, err := exec.ExecuteTxs(ctx, txs, 1, time.Now(), []byte(""))
	if err == nil {
		t.Fatal("Expected error for malformed transaction, got nil")
	}
}

func TestSetFinal(t *testing.T) {
	exec := NewKVExecutor()
	ctx := context.Background()

	// Test with valid blockHeight
	err := exec.SetFinal(ctx, 1)
	if err != nil {
		t.Errorf("Expected nil error for valid blockHeight, got %v", err)
	}

	// Test with invalid blockHeight (zero)
	err = exec.SetFinal(ctx, 0)
	if err == nil {
		t.Error("Expected error for blockHeight 0, got nil")
	}
}
