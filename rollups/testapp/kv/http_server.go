package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	ds "github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/query"
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
func (hs *HTTPServer) Start(ctx context.Context) error {
	// Start the server in a goroutine
	errCh := make(chan error, 1)
	go func() {
		fmt.Printf("KV Executor HTTP server starting on %s\n", hs.server.Addr)
		if err := hs.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Monitor for context cancellation
	go func() {
		<-ctx.Done()
		// Create a timeout context for shutdown
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		fmt.Printf("KV Executor HTTP server shutting down on %s\n", hs.server.Addr)
		if err := hs.server.Shutdown(shutdownCtx); err != nil {
			fmt.Printf("KV Executor HTTP server shutdown error: %v\n", err)
		}
	}()

	// Check if the server started successfully
	select {
	case err := <-errCh:
		return err
	case <-time.After(100 * time.Millisecond): // Give it a moment to start
		// Server started successfully
		return nil
	}
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

// handleKV handles direct key-value operations (GET/POST) against the database
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

		// Use r.Context() when calling the executor method
		value, exists := hs.executor.GetStoreValue(r.Context(), key)
		if !exists {
			// GetStoreValue now returns false on error too, check logs for details
			// Check if the key truly doesn't exist vs a DB error occurred.
			// For simplicity here, we treat both as Not Found for the client.
			// A more robust implementation might check the error type.
			_, err := hs.executor.db.Get(r.Context(), ds.NewKey(key))
			if errors.Is(err, ds.ErrNotFound) {
				http.Error(w, "Key not found", http.StatusNotFound)
			} else {
				// Some other DB error occurred
				http.Error(w, "Failed to retrieve key", http.StatusInternalServerError)
				fmt.Printf("Error retrieving key '%s' from DB: %v\n", key, err)
			}
			return
		}

		_, err := w.Write([]byte(value))
		if err != nil {
			fmt.Printf("Error writing response: %v\n", err)
		}

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleStore returns all non-reserved key-value pairs in the store by querying the database
// GET /store
func (hs *HTTPServer) handleStore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	store := make(map[string]string)
	q := query.Query{} // Query all entries
	results, err := hs.executor.db.Query(r.Context(), q)
	if err != nil {
		http.Error(w, "Failed to query store", http.StatusInternalServerError)
		fmt.Printf("Error querying datastore: %v\n", err)
		return
	}
	defer results.Close()

	for result := range results.Next() {
		if result.Error != nil {
			http.Error(w, "Failed during store iteration", http.StatusInternalServerError)
			fmt.Printf("Error iterating datastore results: %v\n", result.Error)
			return
		}
		// Exclude reserved genesis keys from the output
		dsKey := ds.NewKey(result.Key)
		if dsKey.Equal(genesisInitializedKey) || dsKey.Equal(genesisStateRootKey) {
			continue
		}
		store[result.Key] = string(result.Value)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(store); err != nil {
		// Error already sent potentially, just log
		fmt.Printf("Error encoding JSON response: %v\n", err)
	}
}
