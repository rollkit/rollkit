use ev_client::{Client, ClientError};

#[tokio::test]
async fn test_client_creation() {
    // Test creating a client with an invalid endpoint - will fail during connection
    let result = Client::connect("http://localhost:12345").await;
    assert!(result.is_err());

    match result.unwrap_err() {
        ClientError::Transport(_) => {}
        e => panic!("Expected Transport error, got: {e:?}"),
    }
}

#[tokio::test]
async fn test_client_with_config() {
    use std::time::Duration;

    // Test creating a client with custom configuration
    let result = Client::connect_with_config("http://localhost:50051", |endpoint| {
        endpoint
            .timeout(Duration::from_secs(5))
            .connect_timeout(Duration::from_secs(2))
    })
    .await;

    // This will fail to connect since no server is running, but it should be a transport error
    assert!(result.is_err());
    match result.unwrap_err() {
        ClientError::Transport(_) => {}
        _ => panic!("Expected Transport error"),
    }
}
