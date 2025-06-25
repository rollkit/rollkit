package client

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/emptypb"

	pb "github.com/rollkit/rollkit/types/pb/rollkit/v1"
	rpc "github.com/rollkit/rollkit/types/pb/rollkit/v1/v1connect"
)

// MetadataEntry represents a metadata key-value pair with description
type MetadataEntry struct {
	Key         string
	Value       []byte
	Description string
}

// MetadataEndpointResponse represents the JSON response from the metadata endpoint
type MetadataEndpointResponse struct {
	Metadata []MetadataEntryJSON `json:"metadata"`
}

// MetadataEntryJSON represents a single metadata entry in JSON format
type MetadataEntryJSON struct {
	Key         string `json:"key"`
	Value       string `json:"value"` // hex encoded
	Description string `json:"description"`
}

// Client is the client for StoreService, P2PService, and HealthService
type Client struct {
	storeClient  rpc.StoreServiceClient
	p2pClient    rpc.P2PServiceClient
	healthClient rpc.HealthServiceClient
	baseURL      string
}

// NewClient creates a new RPC client
func NewClient(baseURL string) *Client {
	httpClient := http.DefaultClient
	storeClient := rpc.NewStoreServiceClient(httpClient, baseURL, connect.WithGRPC())
	p2pClient := rpc.NewP2PServiceClient(httpClient, baseURL, connect.WithGRPC())
	healthClient := rpc.NewHealthServiceClient(httpClient, baseURL, connect.WithGRPC())

	return &Client{
		storeClient:  storeClient,
		p2pClient:    p2pClient,
		healthClient: healthClient,
		baseURL:      baseURL,
	}
}

// GetBlockByHeight returns a block by height
func (c *Client) GetBlockByHeight(ctx context.Context, height uint64) (*pb.Block, error) {
	req := connect.NewRequest(&pb.GetBlockRequest{
		Identifier: &pb.GetBlockRequest_Height{
			Height: height,
		},
	})

	resp, err := c.storeClient.GetBlock(ctx, req)
	if err != nil {
		return nil, err
	}

	return resp.Msg.Block, nil
}

// GetBlockByHash returns a block by hash
func (c *Client) GetBlockByHash(ctx context.Context, hash []byte) (*pb.Block, error) {
	req := connect.NewRequest(&pb.GetBlockRequest{
		Identifier: &pb.GetBlockRequest_Hash{
			Hash: hash,
		},
	})

	resp, err := c.storeClient.GetBlock(ctx, req)
	if err != nil {
		return nil, err
	}

	return resp.Msg.Block, nil
}

// GetState returns the current state
func (c *Client) GetState(ctx context.Context) (*pb.State, error) {
	req := connect.NewRequest(&emptypb.Empty{})
	resp, err := c.storeClient.GetState(ctx, req)
	if err != nil {
		return nil, err
	}

	return resp.Msg.State, nil
}

// GetMetadata returns metadata for a specific key
func (c *Client) GetMetadata(ctx context.Context, key string) ([]byte, error) {
	req := connect.NewRequest(&pb.GetMetadataRequest{
		Key: key,
	})

	resp, err := c.storeClient.GetMetadata(ctx, req)
	if err != nil {
		return nil, err
	}

	return resp.Msg.Value, nil
}

// GetPeerInfo returns information about the connected peers
func (c *Client) GetPeerInfo(ctx context.Context) ([]*pb.PeerInfo, error) {
	req := connect.NewRequest(&emptypb.Empty{})
	resp, err := c.p2pClient.GetPeerInfo(ctx, req)
	if err != nil {
		return nil, err
	}

	return resp.Msg.Peers, nil
}

// GetNetInfo returns information about the network
func (c *Client) GetNetInfo(ctx context.Context) (*pb.NetInfo, error) {
	req := connect.NewRequest(&emptypb.Empty{})
	resp, err := c.p2pClient.GetNetInfo(ctx, req)
	if err != nil {
		return nil, err
	}

	return resp.Msg.NetInfo, nil
}

// GetHealth calls the HealthService.Livez endpoint and returns the HealthStatus
func (c *Client) GetHealth(ctx context.Context) (pb.HealthStatus, error) {
	req := connect.NewRequest(&emptypb.Empty{})
	resp, err := c.healthClient.Livez(ctx, req)
	if err != nil {
		return pb.HealthStatus_UNKNOWN, err
	}
	return resp.Msg.Status, nil
}

// GetAllMetadata returns all available metadata from the server via HTTP endpoint
func (c *Client) GetAllMetadata(ctx context.Context) ([]MetadataEntry, error) {
	// Use HTTP client to call the /metadata endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/metadata", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	var response MetadataEndpointResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Convert from JSON format to our format
	entries := make([]MetadataEntry, len(response.Metadata))
	for i, jsonEntry := range response.Metadata {
		// Decode hex-encoded value
		value, err := hex.DecodeString(jsonEntry.Value)
		if err != nil {
			return nil, fmt.Errorf("failed to decode hex value for key %s: %w", jsonEntry.Key, err)
		}
		
		entries[i] = MetadataEntry{
			Key:         jsonEntry.Key,
			Value:       value,
			Description: jsonEntry.Description,
		}
	}

	return entries, nil
}
