package mockserv

import (
	"context"

	ds "github.com/ipfs/go-datastore"
	tmlog "github.com/tendermint/tendermint/libs/log"
	"google.golang.org/grpc"

	grpcda "github.com/rollkit/rollkit/da/grpc"
	"github.com/rollkit/rollkit/da/mock"
	"github.com/rollkit/rollkit/types"
	"github.com/rollkit/rollkit/types/pb/dalc"
	"github.com/rollkit/rollkit/types/pb/rollkit"
)

// GetServer creates and returns gRPC server instance.
func GetServer(kv ds.Datastore, conf grpcda.Config, mockConfig []byte, logger tmlog.Logger) *grpc.Server {
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

func (m *mockImpl) SubmitBlock(ctx context.Context, request *dalc.SubmitBlockRequest) (*dalc.SubmitBlockResponse, error) {
	var b types.Block
	err := b.FromProto(request.Block)
	if err != nil {
		return nil, err
	}
	resp := m.mock.SubmitBlock(ctx, &b)
	return &dalc.SubmitBlockResponse{
		Result: &dalc.DAResponse{
			Code:     dalc.StatusCode(resp.Code),
			Message:  resp.Message,
			DAHeight: resp.DAHeight,
		},
	}, nil
}

func (m *mockImpl) CheckBlockAvailability(ctx context.Context, request *dalc.CheckBlockAvailabilityRequest) (*dalc.CheckBlockAvailabilityResponse, error) {
	resp := m.mock.CheckBlockAvailability(ctx, request.DAHeight)
	return &dalc.CheckBlockAvailabilityResponse{
		Result: &dalc.DAResponse{
			Code:    dalc.StatusCode(resp.Code),
			Message: resp.Message,
		},
		DataAvailable: resp.DataAvailable,
	}, nil
}

func (m *mockImpl) RetrieveBlocks(ctx context.Context, request *dalc.RetrieveBlocksRequest) (*dalc.RetrieveBlocksResponse, error) {
	resp := m.mock.RetrieveBlocks(ctx, request.DAHeight)
	blocks := make([]*rollkit.Block, len(resp.Blocks))
	for i := range resp.Blocks {
		blocks[i] = resp.Blocks[i].ToProto()
	}
	return &dalc.RetrieveBlocksResponse{
		Result: &dalc.DAResponse{
			Code:    dalc.StatusCode(resp.Code),
			Message: resp.Message,
		},
		Blocks: blocks,
	}, nil
}
