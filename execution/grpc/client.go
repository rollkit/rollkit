package grpc

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/rollkit/rollkit/core/execution"
	pb "github.com/rollkit/rollkit/types/pb/rollkit/v1"
	"github.com/rollkit/rollkit/types/pb/rollkit/v1/v1connect"
)

// Ensure Client implements the execution.Executor interface
var _ execution.Executor = (*Client)(nil)

// Client is a gRPC client that implements the execution.Executor interface.
// It communicates with a remote execution service via gRPC using Connect-RPC.
type Client struct {
	client v1connect.ExecutorServiceClient
}

// NewClient creates a new gRPC execution client.
//
// Parameters:
// - url: The URL of the gRPC server (e.g., "http://localhost:50051")
// - opts: Optional Connect client options for configuring the connection
//
// Returns:
// - *Client: The initialized gRPC client
func NewClient(url string, opts ...connect.ClientOption) *Client {
	return &Client{
		client: v1connect.NewExecutorServiceClient(
			http.DefaultClient,
			url,
			opts...,
		),
	}
}

// InitChain initializes a new blockchain instance with genesis parameters.
//
// This method sends an InitChain request to the remote execution service and
// returns the initial state root and maximum bytes allowed for transactions.
func (c *Client) InitChain(ctx context.Context, genesisTime time.Time, initialHeight uint64, chainID string) (stateRoot []byte, maxBytes uint64, err error) {
	req := connect.NewRequest(&pb.InitChainRequest{
		GenesisTime:   timestamppb.New(genesisTime),
		InitialHeight: initialHeight,
		ChainId:       chainID,
	})

	resp, err := c.client.InitChain(ctx, req)
	if err != nil {
		return nil, 0, fmt.Errorf("grpc client: failed to init chain: %w", err)
	}

	return resp.Msg.StateRoot, resp.Msg.MaxBytes, nil
}

// GetTxs fetches available transactions from the execution layer's mempool.
//
// This method retrieves transactions that are ready to be included in a block.
// The execution service may perform validation and filtering before returning transactions.
func (c *Client) GetTxs(ctx context.Context) ([][]byte, error) {
	req := connect.NewRequest(&pb.GetTxsRequest{})

	resp, err := c.client.GetTxs(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("grpc client: failed to get txs: %w", err)
	}

	return resp.Msg.Txs, nil
}

// ExecuteTxs processes transactions to produce a new block state.
//
// This method sends transactions to the execution service for processing and
// returns the updated state root after execution. The execution service ensures
// deterministic execution and validates the state transition.
func (c *Client) ExecuteTxs(ctx context.Context, txs [][]byte, blockHeight uint64, timestamp time.Time, prevStateRoot []byte) (updatedStateRoot []byte, maxBytes uint64, err error) {
	req := connect.NewRequest(&pb.ExecuteTxsRequest{
		Txs:           txs,
		BlockHeight:   blockHeight,
		Timestamp:     timestamppb.New(timestamp),
		PrevStateRoot: prevStateRoot,
	})

	resp, err := c.client.ExecuteTxs(ctx, req)
	if err != nil {
		return nil, 0, fmt.Errorf("grpc client: failed to execute txs: %w", err)
	}

	return resp.Msg.UpdatedStateRoot, resp.Msg.MaxBytes, nil
}

// SetFinal marks a block as finalized at the specified height.
//
// This method notifies the execution service that a block has been finalized,
// allowing it to perform cleanup operations and commit the state permanently.
func (c *Client) SetFinal(ctx context.Context, blockHeight uint64) error {
	req := connect.NewRequest(&pb.SetFinalRequest{
		BlockHeight: blockHeight,
	})

	_, err := c.client.SetFinal(ctx, req)
	if err != nil {
		return fmt.Errorf("grpc client: failed to set final: %w", err)
	}

	return nil
}