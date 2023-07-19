package mockserv

import (
	"context"

	cmlog "github.com/cometbft/cometbft/libs/log"
	ds "github.com/ipfs/go-datastore"
	"google.golang.org/grpc"

	grpcda "github.com/rollkit/rollkit/da/grpc"
	"github.com/rollkit/rollkit/da/mock"
	"github.com/rollkit/rollkit/types"
	"github.com/rollkit/rollkit/types/pb/dalc"
	"github.com/rollkit/rollkit/types/pb/rollkit"
)

// GetServer creates and returns gRPC server instance.
func GetServer(kv ds.Datastore, conf grpcda.Config, mockConfig []byte, logger cmlog.Logger) *grpc.Server {
	srv := grpc.NewServer()
	mockImpl := &mockImpl{}
	err := mockImpl.mock.Init([8]byte{}, mockConfig, kv, logger)
	if err != nil {
		logger.Error("failed to initialize mock DALC", "error", err)
		panic(err)
	}
	err = mockImpl.mock.Start()
	if err != nil {
		logger.Error("failed to start mock DALC", "error", err)
		panic(err)
	}
	dalc.RegisterDALCServiceServer(srv, mockImpl)
	return srv
}

type mockImpl struct {
	mock mock.DataAvailabilityLayerClient
}

func (m *mockImpl) SubmitBlocks(ctx context.Context, request *dalc.SubmitBlocksRequest) (*dalc.SubmitBlocksResponse, error) {
	blocks := make([]*types.Block, len(request.Blocks))
	for i := range request.Blocks {
		var b types.Block
		err := b.FromProto(request.Blocks[i])
		if err != nil {
			return nil, err
		}
		blocks[i] = &b
	}

	resp := m.mock.SubmitBlocks(ctx, blocks)
	return &dalc.SubmitBlocksResponse{
		Result: &dalc.DAResponse{
			Code:     dalc.StatusCode(resp.Code),
			Message:  resp.Message,
			DAHeight: resp.DAHeight,
		},
	}, nil
}

func (m *mockImpl) RetrieveBlocks(ctx context.Context, request *dalc.RetrieveBlocksRequest) (*dalc.RetrieveBlocksResponse, error) {
	resp := m.mock.RetrieveBlocks(ctx, request.DAHeight)
	blocks := make([]*rollkit.Block, len(resp.Blocks))
	for i := range resp.Blocks {
		bp, err := resp.Blocks[i].ToProto()
		if err != nil {
			return nil, err
		}
		blocks[i] = bp
	}
	return &dalc.RetrieveBlocksResponse{
		Result: &dalc.DAResponse{
			Code:    dalc.StatusCode(resp.Code),
			Message: resp.Message,
		},
		Blocks: blocks,
	}, nil
}
