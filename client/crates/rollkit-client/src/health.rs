use crate::{client::RollkitClient, error::Result};
use rollkit_types::v1::{
    health_service_client::HealthServiceClient, GetHealthResponse, HealthStatus,
};
use tonic::Request;

pub struct HealthClient {
    inner: HealthServiceClient<tonic::transport::Channel>,
}

impl HealthClient {
    /// Create a new HealthClient from a RollkitClient
    pub fn new(client: &RollkitClient) -> Self {
        let inner = HealthServiceClient::new(client.channel().clone());
        Self { inner }
    }

    /// Check if the node is alive and get its health status
    pub async fn livez(&mut self) -> Result<HealthStatus> {
        let request = Request::new(());
        let response = self.inner.livez(request).await?;

        Ok(response.into_inner().status())
    }

    /// Get the full health response
    pub async fn get_health(&mut self) -> Result<GetHealthResponse> {
        let request = Request::new(());
        let response = self.inner.livez(request).await?;

        Ok(response.into_inner())
    }

    /// Check if the node is healthy (status is PASS)
    pub async fn is_healthy(&mut self) -> Result<bool> {
        let status = self.livez().await?;
        Ok(status == HealthStatus::Pass)
    }
}
