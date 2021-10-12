package main

import (
	grpcda "github.com/celestiaorg/optimint/da/grpc"
	"github.com/celestiaorg/optimint/da/mock"
	"github.com/celestiaorg/optimint/store"
	"github.com/celestiaorg/optimint/types"
	"github.com/celestiaorg/optimint/types/pb/dalc"
	tmlog "github.com/tendermint/tendermint/libs/log"
	context "golang.org/x/net/context"
	"google.golang.org/grpc"
	"net"
	"os"
	"strconv"
)

type MockImpl struct {
	mock mock.MockDataAvailabilityLayerClient
}

func (m MockImpl) SubmitBlock(_ context.Context, request *dalc.SubmitBlockRequest) (*dalc.SubmitBlockResponse, error) {
	var b types.Block
	err := b.FromProto(request.Block)
	if err != nil {
		return nil, err
	}
	resp := m.mock.SubmitBlock(&b)
	return &dalc.SubmitBlockResponse{
		Result: &dalc.DAResponse{
			Code: dalc.StatusCode(resp.Code),
			Message: resp.Message,
		},
	}, nil
}


func (m MockImpl) CheckBlockAvailability(_ context.Context, request *dalc.CheckBlockAvailabilityRequest) (*dalc.CheckBlockAvailabilityResponse, error) {
	var h types.Header
	err := h.FromProto(request.Header)
	if err != nil {
		return nil, err
	}
	resp := m.mock.CheckBlockAvailability(&h)
	return &dalc.CheckBlockAvailabilityResponse{
		Result: &dalc.DAResponse{
			Code: dalc.StatusCode(resp.Code),
			Message: resp.Message,
		},
		DataAvailable: resp.DataAvailable,
	}, nil
}

func (m MockImpl) RetrieveBlock(context context.Context, request *dalc.RetrieveBlockRequest) (*dalc.RetrieveBlockResponse, error) {
	resp := m.mock.RetrieveBlock(request.Height)
	return &dalc.RetrieveBlockResponse{
		Result: &dalc.DAResponse{
			Code: dalc.StatusCode(resp.Code),
			Message: resp.Message,
		},
		Block: resp.Block.ToProto(),
	}, nil
}

func main() {
	var conf grpcda.Config
	conf = grpcda.DefaultConfig

	logger := tmlog.NewTMLogger(os.Stdout)

	lis, err := net.Listen("tcp", conf.Host + ":" + strconv.Itoa(conf.Port))
	if err != nil {
		logger.Error("failed to create listener", "error", err)
		panic(err)
	}

	srv := grpc.NewServer()
	mockImpl := MockImpl{}
	kv := store.NewKVStore(".", "db", "optimint")
	err = mockImpl.mock.Init(nil, kv, logger)
	if err != nil {
		logger.Error("failed to initialize mock DALC", "error", err)
		panic(err)
	}
	dalc.RegisterDataAvailabilityLayerClientServiceServer(srv, mockImpl)
	err = srv.Serve(lis)
	if err != nil {
		logger.Error("error while serving", "error", err)
	}
}
