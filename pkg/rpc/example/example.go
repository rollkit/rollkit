package example

import (
	"context"
	"errors"
	"log"
	"net/http"

	"github.com/rollkit/rollkit/pkg/rpc/client"
	"github.com/rollkit/rollkit/pkg/rpc/server"
	"github.com/rollkit/rollkit/pkg/store"
)

// StartStoreServer starts a Store RPC server with the provided store instance
func StartStoreServer(s store.Store, address string) {
	// Create and start the server
	log.Printf("Starting Store RPC server on %s", address)
	if err := server.StartServer(s, address); err != nil {
		log.Fatalf("Failed to start Store RPC server: %v", err)
	}
}

// ExampleClient demonstrates how to use the Store RPC client
func ExampleClient() {
	// Create a new client
	client := client.NewStoreClient("http://localhost:8080")
	ctx := context.Background()

	// Get the current state
	state, err := client.GetState(ctx)
	if err != nil {
		log.Fatalf("Failed to get state: %v", err)
	}
	log.Printf("Current state: %+v", state)

	// Get metadata
	metadataKey := "example_key"
	metadataValue, err := client.GetMetadata(ctx, metadataKey)
	if err != nil {
		log.Printf("Metadata not found: %v", err)
	} else {
		log.Printf("Metadata value: %s", string(metadataValue))
	}

	// Get a block by height
	height := uint64(10)
	block, err := client.GetBlockByHeight(ctx, height)
	if err != nil {
		log.Fatalf("Failed to get block: %v", err)
	}
	log.Printf("Block at height %d: %+v", height, block)
}

// ExampleServer demonstrates how to create and start a Store RPC server
func ExampleServer(s store.Store) {
	// Start the server in a separate goroutine
	go func() {
		if err := server.StartServer(s, "localhost:8080"); err != nil && errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Server error: %v", err)
		}
	}()

	log.Println("Store RPC server started on localhost:8080")
	// The server will continue running until the program exits
}
