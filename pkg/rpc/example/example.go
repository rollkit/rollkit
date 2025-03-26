package example

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/rollkit/rollkit/pkg/rpc/client"
	"github.com/rollkit/rollkit/pkg/rpc/server"
	"github.com/rollkit/rollkit/pkg/store"
)

// StartStoreServer starts a Store RPC server with the provided store instance
func StartStoreServer(s store.Store, address string) {
	// Create and start the server
	// Start RPC server
	rpcAddr := fmt.Sprintf("%s:%d", "localhost", 8080)
	handler, err := server.NewStoreServiceHandler(s)
	if err != nil {
		panic(err)
	}

	rpcServer := &http.Server{
		Addr:         rpcAddr,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start the server in a separate goroutine
	go func() {
		if err := rpcServer.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("RPC server error: %v", err)
		}
	}()
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

	// Start RPC server
	rpcAddr := fmt.Sprintf("%s:%d", "localhost", 8080)
	handler, err := server.NewStoreServiceHandler(s)
	if err != nil {
		panic(err)
	}

	rpcServer := &http.Server{
		Addr:         rpcAddr,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start the server in a separate goroutine
	go func() {
		if err := rpcServer.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("RPC server error: %v", err)
		}
	}()

	log.Println("Store RPC server started on localhost:8080")
	// The server will continue running until the program exits
}
