package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/pkg/store"
)

func TestRegisterCustomHTTPEndpoints(t *testing.T) {
	// Create a new ServeMux
	mux := http.NewServeMux()

	// Register custom HTTP endpoints
	RegisterCustomHTTPEndpoints(mux)

	// Create a new HTTP test server with the mux
	testServer := httptest.NewServer(mux)
	defer testServer.Close()

	// Make an HTTP GET request to the /health/live endpoint
	resp, err := http.Get(testServer.URL + "/health/live")
	assert.NoError(t, err)
	defer resp.Body.Close()

	// Check the status code
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)

	// Check the response body content
	assert.Equal(t, "OK\n", string(body)) // fmt.Fprintln adds a newline
}

func TestMetadataEndpoint(t *testing.T) {
	// Create a new in-memory store
	kv, err := store.NewDefaultInMemoryKVStore()
	require.NoError(t, err)
	s := store.New(kv)

	// Set some metadata
	ctx := context.Background()
	require.NoError(t, s.SetMetadata(ctx, "d", []byte{1, 2, 3, 4, 5, 6, 7, 8}))
	require.NoError(t, s.SetMetadata(ctx, "l", []byte("test-batch-data")))

	// Create a new ServeMux
	mux := http.NewServeMux()

	// Register the metadata endpoint
	RegisterMetadataEndpoint(mux, s)

	// Create a new HTTP test server with the mux
	testServer := httptest.NewServer(mux)
	defer testServer.Close()

	// Make an HTTP GET request to the /metadata endpoint
	resp, err := http.Get(testServer.URL + "/metadata")
	require.NoError(t, err)
	defer resp.Body.Close()

	// Check the status code
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Check the content type
	require.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	// Read and parse the response body
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var response MetadataEndpointResponse
	require.NoError(t, json.Unmarshal(body, &response))

	// Check that we got metadata entries
	require.NotEmpty(t, response.Metadata)

	// Create a map for easier lookup
	entryMap := make(map[string]MetadataEntryJSON)
	for _, entry := range response.Metadata {
		entryMap[entry.Key] = entry
	}

	// Verify that our set metadata is included
	require.Contains(t, entryMap, "d")
	require.Equal(t, "0102030405060708", entryMap["d"].Value) // hex encoded
	require.Contains(t, entryMap["d"].Description, "DA included height")

	require.Contains(t, entryMap, "l")
	require.Equal(t, "746573742d62617463682d64617461", entryMap["l"].Value) // hex encoded "test-batch-data"
	require.Contains(t, entryMap["l"].Description, "Last batch data")
}

func TestMetadataEndpointMethodNotAllowed(t *testing.T) {
	// Create a store
	kv, err := store.NewDefaultInMemoryKVStore()
	require.NoError(t, err)
	s := store.New(kv)

	// Create a new ServeMux
	mux := http.NewServeMux()

	// Register the metadata endpoint
	RegisterMetadataEndpoint(mux, s)

	// Create a new HTTP test server with the mux
	testServer := httptest.NewServer(mux)
	defer testServer.Close()

	// Make an HTTP POST request (should be rejected)
	resp, err := http.Post(testServer.URL+"/metadata", "application/json", nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Check that it returns Method Not Allowed
	require.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
}
