package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/rollkit/rollkit/pkg/store"
	"github.com/rollkit/rollkit/types"
	pb "github.com/rollkit/rollkit/types/pb/rollkit/v1"
	rpc "github.com/rollkit/rollkit/types/pb/rollkit/v1/v1connect"
)

// StoreServer implements the StoreService defined in the proto file
type StoreServer struct {
	store store.Store
}

// NewStoreServer creates a new StoreServer instance
func NewStoreServer(store store.Store) *StoreServer {
	return &StoreServer{
		store: store,
	}
}

// GetBlock implements the GetBlock RPC method
func (s *StoreServer) GetBlock(
	ctx context.Context,
	req *connect.Request[pb.GetBlockRequest],
) (*connect.Response[pb.GetBlockResponse], error) {
	var header *types.SignedHeader
	var data *types.Data
	var err error

	switch identifier := req.Msg.Identifier.(type) {
	case *pb.GetBlockRequest_Height:
		header, data, err = s.store.GetBlockData(ctx, identifier.Height)
	case *pb.GetBlockRequest_Hash:
		hash := types.Hash(identifier.Hash)
		header, data, err = s.store.GetBlockByHash(ctx, hash)
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid identifier type"))
	}

	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	// Convert types to protobuf types
	pbHeader, err := header.ToProto()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	pbData := data.ToProto()

	return connect.NewResponse(&pb.GetBlockResponse{
		Block: &pb.Block{
			Header: pbHeader,
			Data:   pbData,
		},
	}), nil
}

// GetState implements the GetState RPC method
func (s *StoreServer) GetState(
	ctx context.Context,
	req *connect.Request[pb.GetStateRequest],
) (*connect.Response[pb.GetStateResponse], error) {
	state, err := s.store.GetState(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	// Convert state to protobuf type
	pbState := &pb.State{
		AppHash:         state.AppHash,
		LastBlockHeight: state.LastBlockHeight,
		LastBlockTime:   timestamppb.New(state.LastBlockTime),
		DaHeight:        state.DAHeight,
		LastResultsHash: state.LastResultsHash,
		ChainId:         state.ChainID,
		Version: &pb.Version{
			Block: state.Version.Block,
			App:   state.Version.App,
		},
		InitialHeight: state.InitialHeight,
	}

	return connect.NewResponse(&pb.GetStateResponse{
		State: pbState,
	}), nil
}

// GetMetadata implements the GetMetadata RPC method
func (s *StoreServer) GetMetadata(
	ctx context.Context,
	req *connect.Request[pb.GetMetadataRequest],
) (*connect.Response[pb.GetMetadataResponse], error) {
	value, err := s.store.GetMetadata(ctx, req.Msg.Key)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	return connect.NewResponse(&pb.GetMetadataResponse{
		Value: value,
	}), nil
}

// NewStoreServiceHandler creates a new HTTP handler for the StoreService
func NewStoreServiceHandler(store store.Store) (http.Handler, error) {
	storeServer := NewStoreServer(store)

	mux := http.NewServeMux()

	// Register the StoreService
	path, handler := rpc.NewStoreServiceHandler(storeServer)
	mux.Handle(path, handler)

	// Use h2c to support HTTP/2 without TLS
	return h2c.NewHandler(mux, &http2.Server{
		IdleTimeout:          120 * time.Second, // Close idle connections after 2 minutes
		MaxReadFrameSize:     1 << 24,           // 16MB max frame size
		MaxConcurrentStreams: 100,               // Limit concurrent streams
		ReadIdleTimeout:      30 * time.Second,  // Timeout for reading frames
		PingTimeout:          15 * time.Second,  // Timeout for ping frames
	}), nil
}
