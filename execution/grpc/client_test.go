package grpc

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"
)

// mockExecutor is a mock implementation of execution.Executor for testing
type mockExecutor struct {
	initChainFunc  func(ctx context.Context, genesisTime time.Time, initialHeight uint64, chainID string) ([]byte, uint64, error)
	getTxsFunc     func(ctx context.Context) ([][]byte, error)
	executeTxsFunc func(ctx context.Context, txs [][]byte, blockHeight uint64, timestamp time.Time, prevStateRoot []byte) ([]byte, uint64, error)
	setFinalFunc   func(ctx context.Context, blockHeight uint64) error
}

func (m *mockExecutor) InitChain(ctx context.Context, genesisTime time.Time, initialHeight uint64, chainID string) ([]byte, uint64, error) {
	if m.initChainFunc != nil {
		return m.initChainFunc(ctx, genesisTime, initialHeight, chainID)
	}
	return []byte("mock_state_root"), 1000000, nil
}

func (m *mockExecutor) GetTxs(ctx context.Context) ([][]byte, error) {
	if m.getTxsFunc != nil {
		return m.getTxsFunc(ctx)
	}
	return [][]byte{[]byte("tx1"), []byte("tx2")}, nil
}

func (m *mockExecutor) ExecuteTxs(ctx context.Context, txs [][]byte, blockHeight uint64, timestamp time.Time, prevStateRoot []byte) ([]byte, uint64, error) {
	if m.executeTxsFunc != nil {
		return m.executeTxsFunc(ctx, txs, blockHeight, timestamp, prevStateRoot)
	}
	return []byte("updated_state_root"), 1000000, nil
}

func (m *mockExecutor) SetFinal(ctx context.Context, blockHeight uint64) error {
	if m.setFinalFunc != nil {
		return m.setFinalFunc(ctx, blockHeight)
	}
	return nil
}

func TestClient_InitChain(t *testing.T) {
	ctx := context.Background()
	expectedStateRoot := []byte("test_state_root")
	expectedMaxBytes := uint64(2000000)
	genesisTime := time.Now()
	initialHeight := uint64(1)
	chainID := "test-chain"

	mockExec := &mockExecutor{
		initChainFunc: func(ctx context.Context, gt time.Time, ih uint64, cid string) ([]byte, uint64, error) {
			if !gt.Equal(genesisTime) {
				t.Errorf("expected genesis time %v, got %v", genesisTime, gt)
			}
			if ih != initialHeight {
				t.Errorf("expected initial height %d, got %d", initialHeight, ih)
			}
			if cid != chainID {
				t.Errorf("expected chain ID %s, got %s", chainID, cid)
			}
			return expectedStateRoot, expectedMaxBytes, nil
		},
	}

	// Start test server
	handler := NewExecutorServiceHandler(mockExec)
	server := httptest.NewServer(handler)
	defer server.Close()

	// Create client
	client := NewClient(server.URL)

	// Test InitChain
	stateRoot, maxBytes, err := client.InitChain(ctx, genesisTime, initialHeight, chainID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(stateRoot) != string(expectedStateRoot) {
		t.Errorf("expected state root %s, got %s", expectedStateRoot, stateRoot)
	}
	if maxBytes != expectedMaxBytes {
		t.Errorf("expected max bytes %d, got %d", expectedMaxBytes, maxBytes)
	}
}

func TestClient_GetTxs(t *testing.T) {
	ctx := context.Background()
	expectedTxs := [][]byte{[]byte("tx1"), []byte("tx2"), []byte("tx3")}

	mockExec := &mockExecutor{
		getTxsFunc: func(ctx context.Context) ([][]byte, error) {
			return expectedTxs, nil
		},
	}

	// Start test server
	handler := NewExecutorServiceHandler(mockExec)
	server := httptest.NewServer(handler)
	defer server.Close()

	// Create client
	client := NewClient(server.URL)

	// Test GetTxs
	txs, err := client.GetTxs(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(txs) != len(expectedTxs) {
		t.Fatalf("expected %d txs, got %d", len(expectedTxs), len(txs))
	}

	for i, tx := range txs {
		if string(tx) != string(expectedTxs[i]) {
			t.Errorf("tx %d: expected %s, got %s", i, expectedTxs[i], tx)
		}
	}
}

func TestClient_ExecuteTxs(t *testing.T) {
	ctx := context.Background()
	txs := [][]byte{[]byte("tx1"), []byte("tx2")}
	blockHeight := uint64(10)
	timestamp := time.Now()
	prevStateRoot := []byte("prev_state_root")
	expectedStateRoot := []byte("new_state_root")
	expectedMaxBytes := uint64(3000000)

	mockExec := &mockExecutor{
		executeTxsFunc: func(ctx context.Context, txsIn [][]byte, bh uint64, ts time.Time, psr []byte) ([]byte, uint64, error) {
			if len(txsIn) != len(txs) {
				t.Errorf("expected %d txs, got %d", len(txs), len(txsIn))
			}
			if bh != blockHeight {
				t.Errorf("expected block height %d, got %d", blockHeight, bh)
			}
			if !ts.Equal(timestamp) {
				t.Errorf("expected timestamp %v, got %v", timestamp, ts)
			}
			if string(psr) != string(prevStateRoot) {
				t.Errorf("expected prev state root %s, got %s", prevStateRoot, psr)
			}
			return expectedStateRoot, expectedMaxBytes, nil
		},
	}

	// Start test server
	handler := NewExecutorServiceHandler(mockExec)
	server := httptest.NewServer(handler)
	defer server.Close()

	// Create client
	client := NewClient(server.URL)

	// Test ExecuteTxs
	stateRoot, maxBytes, err := client.ExecuteTxs(ctx, txs, blockHeight, timestamp, prevStateRoot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(stateRoot) != string(expectedStateRoot) {
		t.Errorf("expected state root %s, got %s", expectedStateRoot, stateRoot)
	}
	if maxBytes != expectedMaxBytes {
		t.Errorf("expected max bytes %d, got %d", expectedMaxBytes, maxBytes)
	}
}

func TestClient_SetFinal(t *testing.T) {
	ctx := context.Background()
	blockHeight := uint64(100)

	mockExec := &mockExecutor{
		setFinalFunc: func(ctx context.Context, bh uint64) error {
			if bh != blockHeight {
				t.Errorf("expected block height %d, got %d", blockHeight, bh)
			}
			return nil
		},
	}

	// Start test server
	handler := NewExecutorServiceHandler(mockExec)
	server := httptest.NewServer(handler)
	defer server.Close()

	// Create client
	client := NewClient(server.URL)

	// Test SetFinal
	err := client.SetFinal(ctx, blockHeight)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
