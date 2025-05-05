package executor

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHandleTx(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		body           string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "Valid transaction",
			method:         http.MethodPost,
			body:           "testkey=testvalue",
			expectedStatus: http.StatusAccepted,
			expectedBody:   "Transaction accepted",
		},
		{
			name:           "Empty transaction",
			method:         http.MethodPost,
			body:           "",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Empty transaction\n",
		},
		{
			name:           "Invalid method",
			method:         http.MethodGet,
			body:           "testkey=testvalue",
			expectedStatus: http.StatusMethodNotAllowed,
			expectedBody:   "Method not allowed\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec, err := NewKVExecutor(t.TempDir(), "testdb")
			if err != nil {
				t.Fatalf("Failed to create KVExecutor: %v", err)
			}
			server := NewHTTPServer(exec, ":0")

			req := httptest.NewRequest(tt.method, "/tx", strings.NewReader(tt.body))
			rr := httptest.NewRecorder()

			server.handleTx(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rr.Code)
			}

			if rr.Body.String() != tt.expectedBody {
				t.Errorf("expected body %q, got %q", tt.expectedBody, rr.Body.String())
			}

			// Verify the transaction was added to the channel if it was a valid POST
			if tt.method == http.MethodPost && tt.expectedStatus == http.StatusAccepted {
				// Allow a moment for the channel send to potentially complete
				time.Sleep(10 * time.Millisecond)
				ctx := context.Background()
				retrievedTxs, err := exec.GetTxs(ctx)
				if err != nil {
					t.Fatalf("GetTxs failed: %v", err)
				}
				if len(retrievedTxs) != 1 {
					t.Errorf("expected 1 transaction in channel, got %d", len(retrievedTxs))
				} else if string(retrievedTxs[0]) != tt.body {
					t.Errorf("expected channel to contain %q, got %q", tt.body, string(retrievedTxs[0]))
				}
			} else if tt.method == http.MethodPost {
				// If it was a POST but not accepted, ensure nothing ended up in the channel
				ctx := context.Background()
				retrievedTxs, err := exec.GetTxs(ctx)
				if err != nil {
					t.Fatalf("GetTxs failed: %v", err)
				}
				if len(retrievedTxs) != 0 {
					t.Errorf("expected 0 transactions in channel for failed POST, got %d", len(retrievedTxs))
				}
			}
		})
	}
}

func TestHandleKV_Get(t *testing.T) {
	tests := []struct {
		name           string
		key            string
		value          string
		queryParam     string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "Get existing key",
			key:            "mykey",
			value:          "myvalue",
			queryParam:     "mykey",
			expectedStatus: http.StatusOK,
			expectedBody:   "myvalue",
		},
		{
			name:           "Get non-existent key",
			key:            "mykey",
			value:          "myvalue",
			queryParam:     "nonexistent",
			expectedStatus: http.StatusNotFound,
			expectedBody:   "Key not found\n",
		},
		{
			name:           "Missing key parameter",
			key:            "mykey",
			value:          "myvalue",
			queryParam:     "",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Missing key parameter\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec, err := NewKVExecutor(t.TempDir(), "testdb")
			if err != nil {
				t.Fatalf("Failed to create KVExecutor: %v", err)
			}
			server := NewHTTPServer(exec, ":0")

			// Set up initial data if needed
			if tt.key != "" && tt.value != "" {
				// Create and execute the transaction directly
				tx := []byte(fmt.Sprintf("%s=%s", tt.key, tt.value))
				ctx := context.Background()
				_, _, err := exec.ExecuteTxs(ctx, [][]byte{tx}, 1, time.Now(), []byte(""))
				if err != nil {
					t.Fatalf("Failed to execute setup transaction: %v", err)
				}
			}

			url := "/kv"
			if tt.queryParam != "" {
				url += "?key=" + tt.queryParam
			}

			req := httptest.NewRequest(http.MethodGet, url, nil)
			rr := httptest.NewRecorder()

			server.handleKV(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rr.Code)
			}

			if rr.Body.String() != tt.expectedBody {
				t.Errorf("expected body %q, got %q", tt.expectedBody, rr.Body.String())
			}
		})
	}
}

func TestHTTPServerStartStop(t *testing.T) {
	// Create a test server that listens on a random port
	exec, err := NewKVExecutor(t.TempDir(), "testdb")
	if err != nil {
		t.Fatalf("Failed to create KVExecutor: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// This is just a placeholder handler
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Test the NewHTTPServer function
	httpServer := NewHTTPServer(exec, server.URL)
	if httpServer == nil {
		t.Fatal("NewHTTPServer returned nil")
	}

	if httpServer.executor != exec {
		t.Error("HTTPServer.executor not set correctly")
	}

	// Note: We don't test Start() and Stop() methods directly
	// as they actually bind to ports, which can be problematic in unit tests.
	// In a real test environment, you might want to use integration tests for these.

	// Test with context (minimal test just to verify it compiles)
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Just create a mock test to ensure the context parameter is accepted
	// Don't actually start the server in the test
	testServer := &HTTPServer{
		server: &http.Server{
			Addr:              ":0",             // Use a random port
			ReadHeaderTimeout: 10 * time.Second, // Add timeout to prevent Slowloris attacks
		},
		executor: exec,
	}

	// Just verify the method signature works
	_ = testServer.Start
}

// TestHTTPServerContextCancellation tests that the server shuts down properly when the context is cancelled
func TestHTTPServerContextCancellation(t *testing.T) {
	exec, err := NewKVExecutor(t.TempDir(), "testdb")
	if err != nil {
		t.Fatalf("Failed to create KVExecutor: %v", err)
	}

	// Use a random available port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to find available port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("Failed to close listener: %v", err)
	}

	serverAddr := fmt.Sprintf("127.0.0.1:%d", port)
	server := NewHTTPServer(exec, serverAddr)

	// Create a context with cancel function
	ctx, cancel := context.WithCancel(context.Background())

	// Start the server
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start(ctx)
	}()

	// Give it time to start
	time.Sleep(100 * time.Millisecond)

	// Send a request to confirm it's running
	client := &http.Client{Timeout: 1 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://%s/store", serverAddr))
	if err != nil {
		t.Fatalf("Failed to connect to server: %v", err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatalf("Failed to close response body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	// Cancel the context to shut down the server
	cancel()

	// Wait for shutdown to complete with timeout
	select {
	case err := <-errCh:
		if err != nil && errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("Server shutdown error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Server shutdown timed out")
	}

	// Verify server is actually shutdown by attempting a new connection
	_, err = client.Get(fmt.Sprintf("http://%s/store", serverAddr))
	if err == nil {
		t.Fatal("Expected connection error after shutdown, but got none")
	}
}
