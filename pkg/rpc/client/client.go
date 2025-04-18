package client

import (
	"context"
	"net/http"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/emptypb"

	pb "github.com/rollkit/rollkit/types/pb/rollkit/v1"
	rpc "github.com/rollkit/rollkit/types/pb/rollkit/v1/v1connect"
)

// Client is the client for the StoreService
type Client struct {
	storeClient rpc.StoreServiceClient
	p2pClient   rpc.P2PServiceClient
}

// NewStoreClient creates a new StoreClient
func NewClient(baseURL string) *Client {
	httpClient := http.DefaultClient
	storeClient := rpc.NewStoreServiceClient(
		httpClient,
		baseURL,
		connect.WithGRPC(),
	)

	p2pClient := rpc.NewP2PServiceClient(
		httpClient,
		baseURL,
		connect.WithGRPC(),
	)

	return &Client{
		storeClient: storeClient,
		p2pClient:   p2pClient,
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
