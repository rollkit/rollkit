# ADR 017: Store RPC Layer Implementation using Connect-RPC

## Changelog

- 2024-03-25: Initial proposal

## Context

The Rollkit store package provides a critical interface for storing and retrieving blocks, commits, and state data. Currently, this functionality is only available locally through direct Go package imports. To enable remote access to this data and improve the system's scalability and interoperability, we need to implement a remote procedure call (RPC) layer.

Connect-Go has been chosen as the RPC framework due to its modern features, excellent developer experience, and compatibility with both gRPC and HTTP/1.1 protocols.

## Alternative Approaches

### Pure gRPC

- Pros: Mature ecosystem, wide adoption
- Cons: More complex setup, less flexible protocol support, requires more boilerplate
- Not chosen because Connect-Go provides a more modern and streamlined developer experience while maintaining gRPC compatibility

### REST API

- Pros: Familiar, widely supported
- Cons: No built-in streaming, manual schema definition required
- Not chosen because Connect-RPC provides better performance and type safety

## Decision

Implement a Connect-Go service layer that exposes the Store interface functionality through a well-defined protocol buffer schema and Connect-RPC service definitions.

## Detailed Design

### Protocol Buffer Definitions

```protobuf
syntax = "proto3";

package rollkit.store.v1;

message Block {
  SignedHeader header = 1;
  Data data = 2;
  bytes signature = 3;
}

message GetBlockRequest {
  oneof identifier {
    uint64 height = 1;
    bytes hash = 2;
  }
}

message GetBlockResponse {
  Block block = 1;
}

message GetSignatureRequest {
  oneof identifier {
    uint64 height = 1;
    bytes hash = 2;
  }
}

message GetSignatureResponse {
  bytes signature = 1;
}

message GetStateResponse {
  State state = 1;
}

service StoreService {
  // Query Methods
  rpc GetBlock(GetBlockRequest) returns (GetBlockResponse) {}
  rpc GetState(google.protobuf.Empty) returns (GetStateResponse) {}
}
```

### Implementation Structure

```tree
pkg/
    rpc/
      connect/
        store.connect.go    // Generated Connect service code
      pb/
        store.proto        // Protocol buffer definitions
        store.pb.go        // Generated protobuf code
      server/
        server.go         // Connect-RPC server implementation
      client/
        client.go         // Connect-RPC client implementation
```

## Status

Proposed

## Consequences

### Positive

- Enables remote access to store data
- Type-safe API interactions
- Protocol flexibility (gRPC and HTTP/1.1)
- Modern developer experience
- Built-in streaming support

### Negative

- Additional dependency on Connect-Go
- Need to maintain protocol buffer definitions
- Potential version compatibility challenges

### Neutral

- Requires generating and maintaining additional code
- Need for proper API versioning strategy

## References

- [Connect-RPC Documentation](https://connectrpc.com/docs/go/getting-started/)
- [Protocol Buffers Documentation](https://protobuf.dev)
- [Store Interface Documentation](pkg/store/types.go)
