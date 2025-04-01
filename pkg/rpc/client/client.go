package client

import (
	"context"
	"net/http"

	"connectrpc.com/connect"

	pb "github.com/rollkit/rollkit/types/pb/rollkit/v1"
	rpc "github.com/rollkit/rollkit/types/pb/rollkit/v1/v1connect"
)

// Client is the client for the StoreService
type Client struct {
	client rpc.StoreServiceClient
}

// NewStoreClient creates a new StoreClient
func NewClient(baseURL string) *Client {
	httpClient := http.DefaultClient
	client := rpc.NewStoreServiceClient(
		httpClient,
		baseURL,
		connect.WithGRPC(),
	)

	return &Client{
		client: client,
	}
}

// GetBlockByHeight returns a block by height
func (c *Client) GetBlockByHeight(ctx context.Context, height uint64) (*pb.Block, error) {
	req := connect.NewRequest(&pb.GetBlockRequest{
		Identifier: &pb.GetBlockRequest_Height{
			Height: height,
		},
	})

	resp, err := c.client.GetBlock(ctx, req)
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

	resp, err := c.client.GetBlock(ctx, req)
	if err != nil {
		return nil, err
	}

	return resp.Msg.Block, nil
}

// GetState returns the current state
func (c *Client) GetState(ctx context.Context) (*pb.State, error) {
	req := connect.NewRequest(&pb.GetStateRequest{})
	resp, err := c.client.GetState(ctx, req)
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

	resp, err := c.client.GetMetadata(ctx, req)
	if err != nil {
		return nil, err
	}

	return resp.Msg.Value, nil
}
