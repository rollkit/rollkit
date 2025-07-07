use crate::{client::RollkitClient, error::Result};
use rollkit_types::v1::{
    signer_service_client::SignerServiceClient, GetPublicRequest, GetPublicResponse, SignRequest,
    SignResponse,
};
use tonic::Request;

pub struct SignerClient {
    inner: SignerServiceClient<tonic::transport::Channel>,
}

impl SignerClient {
    /// Create a new SignerClient from a RollkitClient
    pub fn new(client: &RollkitClient) -> Self {
        let inner = SignerServiceClient::new(client.channel().clone());
        Self { inner }
    }

    /// Sign a message
    pub async fn sign(&self, message: Vec<u8>) -> Result<Vec<u8>> {
        let request = Request::new(SignRequest { message });
        let response = self.inner.clone().sign(request).await?;

        Ok(response.into_inner().signature)
    }

    /// Get the public key
    pub async fn get_public_key(&self) -> Result<Vec<u8>> {
        let request = Request::new(GetPublicRequest {});
        let response = self.inner.clone().get_public(request).await?;

        Ok(response.into_inner().public_key)
    }

    /// Get the full sign response
    pub async fn sign_full(&self, message: Vec<u8>) -> Result<SignResponse> {
        let request = Request::new(SignRequest { message });
        let response = self.inner.clone().sign(request).await?;

        Ok(response.into_inner())
    }

    /// Get the full public key response
    pub async fn get_public_full(&self) -> Result<GetPublicResponse> {
        let request = Request::new(GetPublicRequest {});
        let response = self.inner.clone().get_public(request).await?;

        Ok(response.into_inner())
    }
}
