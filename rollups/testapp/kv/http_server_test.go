package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
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
			exec := NewKVExecutor()
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

			// Verify the transaction was added to mempool if it was a valid POST
			if tt.method == http.MethodPost && tt.body != "" {
				if len(exec.mempool) != 1 {
					t.Errorf("expected 1 transaction in mempool, got %d", len(exec.mempool))
				}
				if string(exec.mempool[0]) != tt.body {
					t.Errorf("expected mempool to contain %q, got %q", tt.body, string(exec.mempool[0]))
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
			exec := NewKVExecutor()
			server := NewHTTPServer(exec, ":0")

			// Set up initial data if needed
			if tt.key != "" && tt.value != "" {
				exec.SetStoreValue(tt.key, tt.value)
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

func TestHandleKV_Post(t *testing.T) {
	tests := []struct {
		name           string
		jsonBody       string
		expectedStatus int
		expectedBody   string
		checkKey       string
		expectedValue  string
	}{
		{
			name:           "Set new key-value",
			jsonBody:       `{"key":"newkey","value":"newvalue"}`,
			expectedStatus: http.StatusOK,
			expectedBody:   "Value set",
			checkKey:       "newkey",
			expectedValue:  "newvalue",
		},
		{
			name:           "Update existing key",
			jsonBody:       `{"key":"existing","value":"updated"}`,
			expectedStatus: http.StatusOK,
			expectedBody:   "Value set",
			checkKey:       "existing",
			expectedValue:  "updated",
		},
		{
			name:           "Missing key",
			jsonBody:       `{"value":"somevalue"}`,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Missing key\n",
		},
		{
			name:           "Invalid JSON",
			jsonBody:       `{"key":`,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Invalid JSON\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := NewKVExecutor()
			server := NewHTTPServer(exec, ":0")

			// Set up initial data if needed
			if tt.name == "Update existing key" {
				exec.SetStoreValue("existing", "original")
			}

			req := httptest.NewRequest(http.MethodPost, "/kv", strings.NewReader(tt.jsonBody))
			rr := httptest.NewRecorder()

			server.handleKV(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rr.Code)
			}

			if rr.Body.String() != tt.expectedBody {
				t.Errorf("expected body %q, got %q", tt.expectedBody, rr.Body.String())
			}

			// Verify the key-value was actually set
			if tt.checkKey != "" {
				value, exists := exec.GetStoreValue(tt.checkKey)
				if !exists {
					t.Errorf("key %q was not set in store", tt.checkKey)
				}
				if value != tt.expectedValue {
					t.Errorf("expected value %q for key %q, got %q", tt.expectedValue, tt.checkKey, value)
				}
			}
		})
	}
}

func TestHandleStore(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		initialKVs     map[string]string
		expectedStatus int
		expectedStore  map[string]string
	}{
		{
			name:           "Get empty store",
			method:         http.MethodGet,
			initialKVs:     map[string]string{},
			expectedStatus: http.StatusOK,
			expectedStore:  map[string]string{},
		},
		{
			name:   "Get populated store",
			method: http.MethodGet,
			initialKVs: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			expectedStatus: http.StatusOK,
			expectedStore: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
		},
		{
			name:           "Invalid method",
			method:         http.MethodPost,
			initialKVs:     map[string]string{},
			expectedStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := NewKVExecutor()
			server := NewHTTPServer(exec, ":0")

			// Set up initial data
			for k, v := range tt.initialKVs {
				exec.SetStoreValue(k, v)
			}

			req := httptest.NewRequest(tt.method, "/store", nil)
			rr := httptest.NewRecorder()

			server.handleStore(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rr.Code)
			}

			// For success cases, verify the JSON response
			if tt.expectedStatus == http.StatusOK {
				var got map[string]string
				err := json.NewDecoder(rr.Body).Decode(&got)
				if err != nil {
					t.Errorf("failed to decode response body: %v", err)
				}

				if !reflect.DeepEqual(got, tt.expectedStore) {
					t.Errorf("expected store %v, got %v", tt.expectedStore, got)
				}
			}
		})
	}
}

func TestHTTPServerStartStop(t *testing.T) {
	// Create a test server that listens on a random port
	exec := NewKVExecutor()
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
	exec := NewKVExecutor()

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
