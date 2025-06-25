package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/rollkit/rollkit/pkg/store"
)

// RegisterCustomHTTPEndpoints is the designated place to add new, non-gRPC, plain HTTP handlers.
// Additional custom HTTP endpoints can be registered on the mux here.
func RegisterCustomHTTPEndpoints(mux *http.ServeMux) {
	mux.HandleFunc("/health/live", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "OK")
	})

	// Example for adding more custom endpoints:
	// mux.HandleFunc("/custom/myendpoint", func(w http.ResponseWriter, r *http.Request) {
	//     // Your handler logic here
	//     w.WriteHeader(http.StatusOK)
	//     fmt.Fprintln(w, "My custom endpoint!")
	// })
}

// MetadataEndpointResponse represents the JSON response for the metadata endpoint
type MetadataEndpointResponse struct {
	Metadata []MetadataEntryJSON `json:"metadata"`
}

// MetadataEntryJSON represents a single metadata entry in JSON format
type MetadataEntryJSON struct {
	Key         string `json:"key"`
	Value       string `json:"value"` // base64 encoded
	Description string `json:"description"`
}

// RegisterMetadataEndpoint registers the /metadata endpoint for getting all metadata
func RegisterMetadataEndpoint(mux *http.ServeMux, store store.Store) {
	mux.HandleFunc("/metadata", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		ctx := context.Background()
		entries, err := store.GetAllMetadata(ctx)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, "Error getting metadata: %v", err)
			return
		}

		// Convert to JSON-friendly format (base64 encode the values)
		jsonEntries := make([]MetadataEntryJSON, len(entries))
		for i, entry := range entries {
			jsonEntries[i] = MetadataEntryJSON{
				Key:         entry.Key,
				Value:       fmt.Sprintf("%x", entry.Value), // hex encode for readability
				Description: entry.Description,
			}
		}

		response := MetadataEndpointResponse{
			Metadata: jsonEntries,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	})
}
