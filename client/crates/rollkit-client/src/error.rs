use thiserror::Error;

#[derive(Error, Debug)]
pub enum RollkitClientError {
    #[error("Transport error: {0}")]
    Transport(#[from] tonic::transport::Error),
    
    #[error("RPC error: {0}")]
    Rpc(#[from] tonic::Status),
    
    #[error("Connection error: {0}")]
    Connection(String),
    
    #[error("Invalid endpoint: {0}")]
    InvalidEndpoint(String),
    
    #[error("Timeout error")]
    Timeout,
}

pub type Result<T> = std::result::Result<T, RollkitClientError>;