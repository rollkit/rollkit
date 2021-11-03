package mockserv

import (
	"context"
	"os"

	grpcda "github.com/celestiaorg/optimint/da/grpc"
	"github.com/celestiaorg/optimint/da/mock"
	"github.com/celestiaorg/optimint/store"
	"github.com/celestiaorg/optimint/types"
	"github.com/celestiaorg/optimint/types/pb/dalc"
	tmlog "github.com/tendermint/tendermint/libs/log"
	"google.golang.org/grpc"
)

func GetServer(kv store.KVStore, conf grpcda.Config) *grpc.Server {
	logger := tmlog.NewTMLogger(os.Stdout)

	srv := grpc.NewServer()
	mockImpl := &mockImpl{}
	err := mockImpl.mock.Init(nil, kv, logger)
	if err != nil {
		logger.Error("failed to initialize mock DALC", "error", err)
		panic(err)
	}
	dalc.RegisterDALCServiceServer(srv, mockImpl)
	return srv
}

type mockImpl struct {
	mock mock.MockDataAvailabilityLayerClient
}

func (m *mockImpl) SubmitBlock(_ context.Context, request *dalc.SubmitBlockRequest) (*dalc.SubmitBlockResponse, error) {
	var b types.Block
	err := b.FromProto(request.Block)
	if err != nil {
		return nil, err
	}
	resp := m.mock.SubmitBlock(&b)
	return &dalc.SubmitBlockResponse{
		Result: &dalc.DAResponse{
			Code:    dalc.StatusCode(resp.Code),
			Message: resp.Message,
		},
	}, nil
}

func (m *mockImpl) CheckBlockAvailability(_ context.Context, request *dalc.CheckBlockAvailabilityRequest) (*dalc.CheckBlockAvailabilityResponse, error) {
	var h types.Header
	err := h.FromProto(request.Header)
	if err != nil {
		return nil, err
	}
	resp := m.mock.CheckBlockAvailability(&h)
	return &dalc.CheckBlockAvailabilityResponse{
		Result: &dalc.DAResponse{
			Code:    dalc.StatusCode(resp.Code),
			Message: resp.Message,
		},
		DataAvailable: resp.DataAvailable,
	}, nil
}

func (m *mockImpl) RetrieveBlock(context context.Context, request *dalc.RetrieveBlockRequest) (*dalc.RetrieveBlockResponse, error) {
	resp := m.mock.RetrieveBlock(request.Height)
	return &dalc.RetrieveBlockResponse{
		Result: &dalc.DAResponse{
			Code:    dalc.StatusCode(resp.Code),
			Message: resp.Message,
		},
		Block: resp.Block.ToProto(),
	}, nil
}
