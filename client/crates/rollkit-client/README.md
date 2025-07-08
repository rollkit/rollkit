# Rollkit Rust Client

[![Rust Tests](https://github.com/rollkit/rollkit/actions/workflows/rust-test.yml/badge.svg)](https://github.com/rollkit/rollkit/actions/workflows/rust-test.yml)
[![Rust Lint](https://github.com/rollkit/rollkit/actions/workflows/rust-lint.yml/badge.svg)](https://github.com/rollkit/rollkit/actions/workflows/rust-lint.yml)
<!-- markdown-link-check-disable -->
[![crates.io](https://img.shields.io/crates/v/rollkit-client.svg)](https://crates.io/crates/rollkit-client)
[![docs.rs](https://docs.rs/rollkit-client/badge.svg)](https://docs.rs/rollkit-client)
<!-- markdown-link-check-enable -->

A Rust client library for interacting with Rollkit nodes via gRPC.

## Features

- Full gRPC client implementation for all Rollkit services
- Type-safe interfaces using generated protobuf types
- Async/await support with Tokio
- Connection management with configurable timeouts
- Comprehensive error handling

## Installation

Add this to your `Cargo.toml`:

```toml
[dependencies]
rollkit-client = { path = "path/to/rollkit-client" }
tokio = { version = "1.45", features = ["full"] }
```

## Usage

### Basic Example

```rust
use rollkit_client::{RollkitClient, HealthClient};

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    // Connect to a Rollkit node
    let client = RollkitClient::connect("http://localhost:50051").await?;

    // Check health
    let health = HealthClient::new(&client);
    let is_healthy = health.is_healthy().await?;
    println!("Node healthy: {}", is_healthy);

    Ok(())
}
```

### Using the Builder Pattern

```rust
use rollkit_client::RollkitClient;
use std::time::Duration;

// Create a client with custom timeouts
let client = RollkitClient::builder()
    .endpoint("http://localhost:50051")
    .timeout(Duration::from_secs(30))
    .connect_timeout(Duration::from_secs(10))
    .build()
    .await?;
```

### TLS Configuration

```rust
use rollkit_client::{RollkitClient, ClientTlsConfig};

// Enable TLS with default configuration
let client = RollkitClient::builder()
    .endpoint("https://secure-node.rollkit.dev")
    .tls()
    .build()
    .await?;

// Or with custom TLS configuration
let tls_config = ClientTlsConfig::new()
    .domain_name("secure-node.rollkit.dev");

let client = RollkitClient::builder()
    .endpoint("https://secure-node.rollkit.dev")
    .tls_config(tls_config)
    .build()
    .await?;
```

### Legacy Connection Method

```rust
use rollkit_client::RollkitClient;
use std::time::Duration;

// Still supported for backward compatibility
let client = RollkitClient::connect_with_config(
    "http://localhost:50051",
    |endpoint| {
        endpoint
            .timeout(Duration::from_secs(30))
            .connect_timeout(Duration::from_secs(10))
            .tcp_keepalive(Some(Duration::from_secs(60)))
    }
).await?;
```

## Services

The client provides wrappers for all Rollkit gRPC services. All service methods are now thread-safe and can be called concurrently:

### Health Service

- `livez()` - Check if the node is alive
- `is_healthy()` - Check if the node is healthy

### P2P Service

- `get_peer_info()` - Get information about connected peers
- `get_net_info()` - Get network information

### Signer Service

- `sign(message)` - Sign a message
- `get_public_key()` - Get the node's public key

### Store Service

- `get_block_by_height(height)` - Get a block by height
- `get_block_by_hash(hash)` - Get a block by hash
- `get_state()` - Get the current state
- `get_metadata(key)` - Get metadata by key

## Examples

See the `examples` directory for more detailed usage examples:

```bash
cargo run --example basic
```

### Concurrent Usage Example

```rust
use rollkit_client::{RollkitClient, StoreClient};
use tokio::task;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let client = RollkitClient::connect("http://localhost:50051").await?;
    let store = StoreClient::new(&client);

    // Service clients can be used concurrently
    let mut handles = vec![];

    for height in 0..10 {
        let store_clone = store.clone();
        let handle = task::spawn(async move {
            store_clone.get_block_by_height(height).await
        });
        handles.push(handle);
    }

    // Wait for all tasks to complete
    for handle in handles {
        let result = handle.await??;
        println!("Got block: {:?}", result);
    }

    Ok(())
}
```

## Error Handling

All methods return `Result<T, RollkitClientError>` where `RollkitClientError` encompasses:

- Transport errors
- RPC errors
- Connection errors
- Invalid endpoint errors
- Timeout errors

## License

Apache-2.0
