use crate::{client::RollkitClient, error::Result};
use rollkit_types::v1::{
    get_block_request::Identifier, store_service_client::StoreServiceClient, Block,
    GetBlockRequest, GetBlockResponse, GetMetadataRequest, GetMetadataResponse, GetStateResponse,
    State,
};
use tonic::Request;

pub struct StoreClient {
    inner: StoreServiceClient<tonic::transport::Channel>,
}

impl StoreClient {
    /// Create a new StoreClient from a RollkitClient
    pub fn new(client: &RollkitClient) -> Self {
        let inner = StoreServiceClient::new(client.channel().clone());
        Self { inner }
    }

    /// Get a block by height
    pub async fn get_block_by_height(&self, height: u64) -> Result<Option<Block>> {
        let request = Request::new(GetBlockRequest {
            identifier: Some(Identifier::Height(height)),
        });
        let response = self.inner.clone().get_block(request).await?;

        Ok(response.into_inner().block)
    }

    /// Get a block by hash
    pub async fn get_block_by_hash(&self, hash: Vec<u8>) -> Result<Option<Block>> {
        let request = Request::new(GetBlockRequest {
            identifier: Some(Identifier::Hash(hash)),
        });
        let response = self.inner.clone().get_block(request).await?;

        Ok(response.into_inner().block)
    }

    /// Get the current state
    pub async fn get_state(&self) -> Result<Option<State>> {
        let request = Request::new(());
        let response = self.inner.clone().get_state(request).await?;

        Ok(response.into_inner().state)
    }

    /// Get metadata by key
    pub async fn get_metadata(&self, key: String) -> Result<Vec<u8>> {
        let request = Request::new(GetMetadataRequest { key });
        let response = self.inner.clone().get_metadata(request).await?;

        Ok(response.into_inner().value)
    }

    /// Get the full block response by height
    pub async fn get_block_full_by_height(&self, height: u64) -> Result<GetBlockResponse> {
        let request = Request::new(GetBlockRequest {
            identifier: Some(Identifier::Height(height)),
        });
        let response = self.inner.clone().get_block(request).await?;

        Ok(response.into_inner())
    }

    /// Get the full block response by hash
    pub async fn get_block_full_by_hash(&self, hash: Vec<u8>) -> Result<GetBlockResponse> {
        let request = Request::new(GetBlockRequest {
            identifier: Some(Identifier::Hash(hash)),
        });
        let response = self.inner.clone().get_block(request).await?;

        Ok(response.into_inner())
    }

    /// Get the full state response
    pub async fn get_state_full(&self) -> Result<GetStateResponse> {
        let request = Request::new(());
        let response = self.inner.clone().get_state(request).await?;

        Ok(response.into_inner())
    }

    /// Get the full metadata response
    pub async fn get_metadata_full(&self, key: String) -> Result<GetMetadataResponse> {
        let request = Request::new(GetMetadataRequest { key });
        let response = self.inner.clone().get_metadata(request).await?;

        Ok(response.into_inner())
    }
}
