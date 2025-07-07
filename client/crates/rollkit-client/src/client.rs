use crate::error::{Result, RollkitClientError};
use std::time::Duration;
use tonic::transport::{Channel, ClientTlsConfig, Endpoint};

#[derive(Clone, Debug)]
pub struct RollkitClient {
    channel: Channel,
    endpoint: String,
}

/// Builder for configuring a RollkitClient
#[derive(Debug)]
pub struct RollkitClientBuilder {
    endpoint: String,
    timeout: Option<Duration>,
    connect_timeout: Option<Duration>,
    tls_config: Option<ClientTlsConfig>,
}

impl RollkitClient {
    /// Create a new RollkitClient with the given endpoint
    pub async fn connect(endpoint: impl Into<String>) -> Result<Self> {
        let endpoint = endpoint.into();
        let channel = Self::create_channel(&endpoint).await?;

        Ok(Self { channel, endpoint })
    }

    /// Create a new RollkitClient builder
    pub fn builder() -> RollkitClientBuilder {
        RollkitClientBuilder {
            endpoint: String::new(),
            timeout: None,
            connect_timeout: None,
            tls_config: None,
        }
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

impl RollkitClientBuilder {
    /// Set the endpoint URL
    pub fn endpoint(mut self, endpoint: impl Into<String>) -> Self {
        self.endpoint = endpoint.into();
        self
    }

    /// Set the request timeout
    pub fn timeout(mut self, timeout: Duration) -> Self {
        self.timeout = Some(timeout);
        self
    }

    /// Set the connection timeout
    pub fn connect_timeout(mut self, timeout: Duration) -> Self {
        self.connect_timeout = Some(timeout);
        self
    }

    /// Enable TLS with default configuration
    pub fn tls(mut self) -> Self {
        self.tls_config = Some(ClientTlsConfig::new());
        self
    }

    /// Set custom TLS configuration
    pub fn tls_config(mut self, config: ClientTlsConfig) -> Self {
        self.tls_config = Some(config);
        self
    }


    /// Build the RollkitClient
    pub async fn build(self) -> Result<RollkitClient> {
        if self.endpoint.is_empty() {
            return Err(RollkitClientError::InvalidEndpoint(
                "Endpoint cannot be empty".to_string(),
            ));
        }

        let endpoint = Endpoint::from_shared(self.endpoint.clone())
            .map_err(|e| RollkitClientError::InvalidEndpoint(e.to_string()))?;

        // Apply timeout configurations
        let endpoint = if let Some(timeout) = self.timeout {
            endpoint.timeout(timeout)
        } else {
            endpoint.timeout(Duration::from_secs(10))
        };

        let endpoint = if let Some(connect_timeout) = self.connect_timeout {
            endpoint.connect_timeout(connect_timeout)
        } else {
            endpoint.connect_timeout(Duration::from_secs(5))
        };

        // Apply TLS configuration if provided
        let endpoint = if let Some(tls_config) = self.tls_config {
            endpoint.tls_config(tls_config)?
        } else {
            endpoint
        };

        let channel = endpoint
            .connect()
            .await
            .map_err(RollkitClientError::Transport)?;

        Ok(RollkitClient {
            channel,
            endpoint: self.endpoint,
        })
    }
}
