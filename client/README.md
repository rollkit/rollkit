# Rollkit Client Libraries

This directory contains client libraries for interacting with Rollkit nodes in various programming languages.

## Structure

```ascii
client/
├── crates/           # Rust client libraries
│   ├── rollkit-types/    # Generated protobuf types for Rust
│   └── rollkit-client/   # High-level Rust client for gRPC services
└── README.md
```

## Rust Client

The Rust client consists of two crates:

### rollkit-types

Contains all the protobuf-generated types and service definitions. This crate is automatically generated from the proto files in `/proto/rollkit/v1/`.

### rollkit-client

A high-level client library that provides:

- Easy-to-use wrappers around the gRPC services
- Connection management with configurable timeouts
- Type-safe interfaces
- Comprehensive error handling
- Example usage code

See the [rollkit-client README](crates/rollkit-client/README.md) for detailed usage instructions.

## Future Client Libraries

This directory is structured to support additional client libraries in the future:

- JavaScript/TypeScript client
- Python client
- Go client

Each language will have its own subdirectory with generated types and high-level client implementations.
