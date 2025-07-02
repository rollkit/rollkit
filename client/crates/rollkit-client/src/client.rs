use crate::error::{Result, RollkitClientError};
use std::time::Duration;
use tonic::transport::{Channel, Endpoint};

#[derive(Clone, Debug)]
pub struct RollkitClient {
    channel: Channel,
    endpoint: String,
}

impl RollkitClient {
    /// Create a new RollkitClient with the given endpoint
    pub async fn connect(endpoint: impl Into<String>) -> Result<Self> {
        let endpoint = endpoint.into();
        let channel = Self::create_channel(&endpoint).await?;
        
        Ok(Self { channel, endpoint })
    }
    
    /// Create a new RollkitClient with custom channel configuration
    pub async fn connect_with_config<F>(endpoint: impl Into<String>, config: F) -> Result<Self>
    where
        F: FnOnce(Endpoint) -> Endpoint,
    {
        let endpoint_str = endpoint.into();
        let endpoint = Endpoint::from_shared(endpoint_str.clone())
            .map_err(|e| RollkitClientError::InvalidEndpoint(e.to_string()))?;
        
        let endpoint = config(endpoint);
        let channel = endpoint
            .connect()
            .await
            .map_err(RollkitClientError::Transport)?;
        
        Ok(Self {
            channel,
            endpoint: endpoint_str,
        })
    }
    
    /// Get the underlying channel
    pub fn channel(&self) -> &Channel {
        &self.channel
    }
    
    /// Get the endpoint URL
    pub fn endpoint(&self) -> &str {
        &self.endpoint
    }
    
    async fn create_channel(endpoint: &str) -> Result<Channel> {
        let endpoint = Endpoint::from_shared(endpoint.to_string())
            .map_err(|e| RollkitClientError::InvalidEndpoint(e.to_string()))?
            .timeout(Duration::from_secs(10))
            .connect_timeout(Duration::from_secs(5));
        
        let channel = endpoint
            .connect()
            .await
            .map_err(RollkitClientError::Transport)?;
        
        Ok(channel)
    }
}