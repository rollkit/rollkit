package grpc

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"

	"github.com/evstack/ev-node/core/execution"
	pb "github.com/evstack/ev-node/types/pb/evnode/v1"
)

// Server is a gRPC server that wraps an execution.Executor implementation.
// It handles the conversion between gRPC types and internal types.
type Server struct {
	executor execution.Executor
}

// NewServer creates a new gRPC server that wraps the given executor.
//
// Parameters:
// - executor: The underlying execution implementation to wrap
//
// Returns:
// - *Server: The initialized gRPC server
func NewServer(executor execution.Executor) *Server {
	return &Server{
		executor: executor,
	}
}

// InitChain handles the InitChain RPC request.
//
// It initializes the blockchain with the given genesis parameters by delegating
// to the underlying executor implementation.
func (s *Server) InitChain(
	ctx context.Context,
	req *connect.Request[pb.InitChainRequest],
) (*connect.Response[pb.InitChainResponse], error) {
	if req.Msg.GenesisTime == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("genesis_time is required"))
	}

	if req.Msg.InitialHeight == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("initial_height must be > 0"))
	}

	if req.Msg.ChainId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("chain_id is required"))
	}

	stateRoot, maxBytes, err := s.executor.InitChain(
		ctx,
		req.Msg.GenesisTime.AsTime(),
		req.Msg.InitialHeight,
		req.Msg.ChainId,
	)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to init chain: %w", err))
	}

	return connect.NewResponse(&pb.InitChainResponse{
		StateRoot: stateRoot,
		MaxBytes:  maxBytes,
	}), nil
}

// GetTxs handles the GetTxs RPC request.
//
// It fetches available transactions from the execution layer's mempool.
func (s *Server) GetTxs(
	ctx context.Context,
	req *connect.Request[pb.GetTxsRequest],
) (*connect.Response[pb.GetTxsResponse], error) {
	txs, err := s.executor.GetTxs(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get txs: %w", err))
	}

	return connect.NewResponse(&pb.GetTxsResponse{
		Txs: txs,
	}), nil
}

// ExecuteTxs handles the ExecuteTxs RPC request.
//
// It processes transactions to produce a new block state by delegating to
// the underlying executor implementation.
func (s *Server) ExecuteTxs(
	ctx context.Context,
	req *connect.Request[pb.ExecuteTxsRequest],
) (*connect.Response[pb.ExecuteTxsResponse], error) {
	if req.Msg.BlockHeight == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("block_height must be > 0"))
	}

	if req.Msg.Timestamp == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("timestamp is required"))
	}

	if len(req.Msg.PrevStateRoot) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("prev_state_root is required"))
	}

	updatedStateRoot, maxBytes, err := s.executor.ExecuteTxs(
		ctx,
		req.Msg.Txs,
		req.Msg.BlockHeight,
		req.Msg.Timestamp.AsTime(),
		req.Msg.PrevStateRoot,
	)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to execute txs: %w", err))
	}

	return connect.NewResponse(&pb.ExecuteTxsResponse{
		UpdatedStateRoot: updatedStateRoot,
		MaxBytes:         maxBytes,
	}), nil
}

// SetFinal handles the SetFinal RPC request.
//
// It marks a block as finalized at the specified height.
func (s *Server) SetFinal(
	ctx context.Context,
	req *connect.Request[pb.SetFinalRequest],
) (*connect.Response[pb.SetFinalResponse], error) {
	if req.Msg.BlockHeight == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("block_height must be > 0"))
	}

	err := s.executor.SetFinal(ctx, req.Msg.BlockHeight)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to set final: %w", err))
	}

	return connect.NewResponse(&pb.SetFinalResponse{}), nil
}
