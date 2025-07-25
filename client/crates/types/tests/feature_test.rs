#[test]
fn test_message_types_available() {
    // These types should always be available
    let _block = ev_types::v1::Block::default();
    let _header = ev_types::v1::Header::default();
    let _state = ev_types::v1::State::default();
}

#[cfg(feature = "grpc")]
#[test]
fn test_grpc_types_available() {
    // These should only be available with the grpc feature
    use ev_types::v1::health_service_client::HealthServiceClient;
    use ev_types::v1::p2p_service_client::P2pServiceClient;
    use ev_types::v1::signer_service_client::SignerServiceClient;
    use ev_types::v1::store_service_client::StoreServiceClient;

    // Just verify the types exist
    let _ = std::any::type_name::<HealthServiceClient<tonic::transport::Channel>>();
    let _ = std::any::type_name::<P2pServiceClient<tonic::transport::Channel>>();
    let _ = std::any::type_name::<SignerServiceClient<tonic::transport::Channel>>();
    let _ = std::any::type_name::<StoreServiceClient<tonic::transport::Channel>>();
}
