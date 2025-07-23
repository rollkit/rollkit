# gRPC Execution Client

This package provides a gRPC-based implementation of the Rollkit execution interface. It allows Rollkit to communicate with remote execution clients via gRPC using the Connect-RPC framework.

## Overview

The gRPC execution client enables separation between the consensus layer (Rollkit) and the execution layer by providing a network interface for communication. This allows execution clients to run in separate processes or even on different machines.

## Usage

### Client

To connect to a remote execution service:

```go
import (
    "github.com/rollkit/rollkit/execution/grpc"
)

// Create a new gRPC client
client := grpc.NewClient("http://localhost:50051")

// Use the client as an execution.Executor
ctx := context.Background()
stateRoot, maxBytes, err := client.InitChain(ctx, time.Now(), 1, "my-chain")
```

### Server

To serve an execution implementation via gRPC:

```go
import (
    "net/http"
    "github.com/rollkit/rollkit/execution/grpc"
)

// Wrap your executor implementation
handler := grpc.NewExecutorServiceHandler(myExecutor)

// Start the HTTP server
http.ListenAndServe(":50051", handler)
```

## Protocol

The gRPC service is defined in `proto/rollkit/v1/execution.proto` and provides the following methods:

- `InitChain`: Initialize the blockchain with genesis parameters
- `GetTxs`: Fetch transactions from the mempool
- `ExecuteTxs`: Execute transactions and update state
- `SetFinal`: Mark a block as finalized

## Features

- Full implementation of the `execution.Executor` interface
- Support for HTTP/1.1 and HTTP/2 (via h2c)
- gRPC reflection for debugging and service discovery
- Compression for efficient data transfer
- Comprehensive error handling and validation

## Testing

Run the tests with:

```bash
go test ./execution/grpc/...
```

The package includes comprehensive unit tests for both client and server implementations.
