//! Rollkit Rust Client Library
//! 
//! This library provides a Rust client for interacting with Rollkit nodes via gRPC.
//! 
//! # Example
//! 
//! ```no_run
//! use rollkit_client::{RollkitClient, HealthClient};
//! 
//! #[tokio::main]
//! async fn main() -> Result<(), Box<dyn std::error::Error>> {
//!     // Connect to a Rollkit node
//!     let client = RollkitClient::connect("http://localhost:50051").await?;
//!     
//!     // Check health
//!     let mut health = HealthClient::new(&client);
//!     let is_healthy = health.is_healthy().await?;
//!     println!("Node healthy: {}", is_healthy);
//!     
//!     Ok(())
//! }
//! ```

pub mod client;
pub mod error;
pub mod health;
pub mod p2p;
pub mod signer;
pub mod store;

// Re-export main types for convenience
pub use client::RollkitClient;
pub use error::{RollkitClientError, Result};
pub use health::HealthClient;
pub use p2p::P2PClient;
pub use signer::SignerClient;
pub use store::StoreClient;

// Re-export types from rollkit-types for convenience
pub use rollkit_types::v1;