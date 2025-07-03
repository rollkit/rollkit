use crate::{client::RollkitClient, error::Result};
use rollkit_types::v1::{
    p2p_service_client::P2pServiceClient, GetNetInfoResponse, GetPeerInfoResponse, NetInfo,
    PeerInfo,
};
use tonic::Request;

pub struct P2PClient {
    inner: P2pServiceClient<tonic::transport::Channel>,
}

impl P2PClient {
    /// Create a new P2PClient from a RollkitClient
    pub fn new(client: &RollkitClient) -> Self {
        let inner = P2pServiceClient::new(client.channel().clone());
        Self { inner }
    }

    /// Get information about connected peers
    pub async fn get_peer_info(&mut self) -> Result<Vec<PeerInfo>> {
        let request = Request::new(());
        let response = self.inner.get_peer_info(request).await?;

        Ok(response.into_inner().peers)
    }

    /// Get network information
    pub async fn get_net_info(&mut self) -> Result<NetInfo> {
        let request = Request::new(());
        let response = self.inner.get_net_info(request).await?;

        Ok(response.into_inner().net_info.unwrap_or_default())
    }

    /// Get the full peer info response
    pub async fn get_peer_info_response(&mut self) -> Result<GetPeerInfoResponse> {
        let request = Request::new(());
        let response = self.inner.get_peer_info(request).await?;

        Ok(response.into_inner())
    }

    /// Get the full network info response
    pub async fn get_net_info_response(&mut self) -> Result<GetNetInfoResponse> {
        let request = Request::new(());
        let response = self.inner.get_net_info(request).await?;

        Ok(response.into_inner())
    }

    /// Get the number of connected peers
    pub async fn peer_count(&mut self) -> Result<usize> {
        let peers = self.get_peer_info().await?;
        Ok(peers.len())
    }
}
