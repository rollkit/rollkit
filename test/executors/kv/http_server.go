package executor

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// HTTPServer wraps a KVExecutor and provides an HTTP interface for it
type HTTPServer struct {
	executor *KVExecutor
	server   *http.Server
}

// NewHTTPServer creates a new HTTP server for the KVExecutor
func NewHTTPServer(executor *KVExecutor, listenAddr string) *HTTPServer {
	hs := &HTTPServer{
		executor: executor,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/tx", hs.handleTx)
	mux.HandleFunc("/kv", hs.handleKV)
	mux.HandleFunc("/store", hs.handleStore)

	hs.server = &http.Server{
		Addr:              listenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	return hs
}

// Start begins listening for HTTP requests
func (hs *HTTPServer) Start() error {
	fmt.Printf("KV Executor HTTP server starting on %s\n", hs.server.Addr)
	return hs.server.ListenAndServe()
}

// Stop shuts down the HTTP server
func (hs *HTTPServer) Stop() error {
	return hs.server.Close()
}

// handleTx handles transaction submissions
// POST /tx with raw binary data or text in request body
// It is recommended to use transactions in the format "key=value" to be consistent
// with the KVExecutor implementation that parses transactions in this format.
// Example: "mykey=myvalue"
func (hs *HTTPServer) handleTx(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer func() {
		if err := r.Body.Close(); err != nil {
			fmt.Printf("Error closing request body: %v\n", err)
		}
	}()

	if len(body) == 0 {
		http.Error(w, "Empty transaction", http.StatusBadRequest)
		return
	}

	hs.executor.InjectTx(body)
	w.WriteHeader(http.StatusAccepted)
	_, err = w.Write([]byte("Transaction accepted"))
	if err != nil {
		fmt.Printf("Error writing response: %v\n", err)
	}
}

// handleKV handles direct key-value operations (GET/POST)
// GET /kv?key=somekey - retrieve a value
// POST /kv with JSON {"key": "somekey", "value": "somevalue"} - set a value
func (hs *HTTPServer) handleKV(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		key := r.URL.Query().Get("key")
		if key == "" {
			http.Error(w, "Missing key parameter", http.StatusBadRequest)
			return
		}

		value, exists := hs.executor.GetStoreValue(key)
		if !exists {
			http.Error(w, "Key not found", http.StatusNotFound)
			return
		}

		_, err := w.Write([]byte(value))
		if err != nil {
			fmt.Printf("Error writing response: %v\n", err)
		}

	case http.MethodPost:
		var data struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}

		if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}
		defer func() {
			if err := r.Body.Close(); err != nil {
				fmt.Printf("Error closing request body: %v\n", err)
			}
		}()

		if data.Key == "" {
			http.Error(w, "Missing key", http.StatusBadRequest)
			return
		}

		hs.executor.SetStoreValue(data.Key, data.Value)
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte("Value set"))
		if err != nil {
			fmt.Printf("Error writing response: %v\n", err)
		}

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleStore returns all key-value pairs in the store
// GET /store
func (hs *HTTPServer) handleStore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	hs.executor.mu.Lock()
	store := make(map[string]string)
	for k, v := range hs.executor.store {
		store[k] = v
	}
	hs.executor.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(store); err != nil {
		fmt.Printf("Error encoding JSON response: %v\n", err)
	}
}
