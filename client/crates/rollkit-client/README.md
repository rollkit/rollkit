# Rollkit Rust Client

[![Rust Tests](https://github.com/rollkit/rollkit/actions/workflows/rust-test.yml/badge.svg)](https://github.com/rollkit/rollkit/actions/workflows/rust-test.yml)
[![Rust Lint](https://github.com/rollkit/rollkit/actions/workflows/rust-lint.yml/badge.svg)](https://github.com/rollkit/rollkit/actions/workflows/rust-lint.yml)
[![crates.io](https://img.shields.io/crates/v/rollkit-client.svg)](https://crates.io/crates/rollkit-client)
[![docs.rs](https://docs.rs/rollkit-client/badge.svg)](https://docs.rs/rollkit-client)

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
    let mut health = HealthClient::new(&client);
    let is_healthy = health.is_healthy().await?;
    println!("Node healthy: {}", is_healthy);
    
    Ok(())
}
```

### Advanced Configuration

```rust
use rollkit_client::RollkitClient;
use std::time::Duration;

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

The client provides wrappers for all Rollkit gRPC services:

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
- `get_block(height)` - Get a block by height
- `get_state(height)` - Get state at a specific height
- `get_metadata(initial_height, latest_height)` - Get metadata for a height range

## Examples

See the `examples` directory for more detailed usage examples:

```bash
cargo run --example basic
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