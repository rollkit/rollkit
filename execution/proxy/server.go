package proxy

import (
	"context"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/emptypb"

	coreexec "github.com/rollkit/rollkit/core/execution"
	pb "github.com/rollkit/rollkit/types/pb/rollkit/v1"
)

// Server implements the ExecutionService defined in the protobuf file.
type Server struct {
	exec coreexec.Executor
}

// NewServer creates a new Execution RPC server wrapping the given executor.
func NewServer(exec coreexec.Executor) *Server {
	return &Server{exec: exec}
}

// InitChain delegates to the wrapped executor.
func (s *Server) InitChain(ctx context.Context, req *connect.Request[pb.InitChainRequest]) (*connect.Response[pb.InitChainResponse], error) {
	genesisTime := req.Msg.GenesisTime.AsTime()
	stateRoot, maxBytes, err := s.exec.InitChain(ctx, genesisTime, req.Msg.InitialHeight, req.Msg.ChainId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&pb.InitChainResponse{StateRoot: stateRoot, MaxBytes: maxBytes}), nil
}

// GetTxs returns available transactions from the executor.
func (s *Server) GetTxs(ctx context.Context, _ *connect.Request[emptypb.Empty]) (*connect.Response[pb.GetTxsResponse], error) {
	txs, err := s.exec.GetTxs(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&pb.GetTxsResponse{Txs: txs}), nil
}

// ExecuteTxs runs a batch of transactions via the executor.
func (s *Server) ExecuteTxs(ctx context.Context, req *connect.Request[pb.ExecuteTxsRequest]) (*connect.Response[pb.ExecuteTxsResponse], error) {
	ts := req.Msg.Timestamp.AsTime()
	updated, maxBytes, err := s.exec.ExecuteTxs(ctx, req.Msg.Txs, req.Msg.BlockHeight, ts, req.Msg.PrevStateRoot)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&pb.ExecuteTxsResponse{UpdatedStateRoot: updated, MaxBytes: maxBytes}), nil
}

// SetFinal marks a block as finalized using the executor.
func (s *Server) SetFinal(ctx context.Context, req *connect.Request[pb.SetFinalRequest]) (*connect.Response[emptypb.Empty], error) {
	if err := s.exec.SetFinal(ctx, req.Msg.BlockHeight); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&emptypb.Empty{}), nil
}
